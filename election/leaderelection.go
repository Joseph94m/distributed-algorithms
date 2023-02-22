package election

import (
	"context"
	"sort"
	"strings"
	"time"

	backoff "github.com/cenkalti/backoff/v4"
	"github.com/go-zookeeper/zk"
	"github.com/rs/zerolog"
)

// LeaderElection is the struct that will be used to configure the leader election
// It is exported so that the calling program can configure it
// Only
type LeaderElection struct {
	// namespace is the namespace on zookeeper that will be used for the election. It must be created manually before the election can start
	ZkNamespace string
	// ZkTimeout is the timeout for the zookeeper connection and the session for the ephemeral znodes
	ZkTimeout time.Duration
	// Zookeepers is a list of zookeeper servers that will be used for the election
	Zookeepers []string
	// Log is logger that will be used. It is zerolog, it is exported to allow for configuration
	// if you don't provide a logger, it will use the default logger
	Log *zerolog.Logger
	// IsLeader is a boolean that indicates if the node is the leader or not. This should be evaluated regularly by the calling program to determine what to do
	IsLeader bool
	// Backoff is the backoff strategy that will be used to retry the election
	// The default value is an exponential backoff with a max elapsed time of 0 so it will retry forever
	// Only the default value is tested
	Backoff backoff.BackOff
	// conn is the zookeeper connection and is shared across the functions
	conn *zk.Conn
	// connectionWatcher is the channel that will be used to watch for connection events
	connectionWatcher <-chan zk.Event
	// currentZnodeName is the name of the znode that the current node is using
	currentZnodeName string
	// leaderWatcher is the channel that will be used to watch for predecessor node events
	watchPredecessor <-chan zk.Event
}

// Elect starts the leader election process based on the configuration provided in the struct
func (l *LeaderElection) StartElectionLoop(ctx context.Context) error {
	var err error
	// perform validation on the configuration
	err = l.validateConfig()
	if err != nil {
		return err
	}
	l.defaultConfig()
	l.Log.Info().Msg("Starting leader election loop")
	// establsih connection to zookeeper, if it fails, the function will return
	// if the connection is lost at any point after a succesful connection, it will infinitely try to reconnect
	l.Log.Info().Msg("Connecting to zookeeper")
	l.conn, l.connectionWatcher, err = zk.Connect(l.Zookeepers, l.ZkTimeout)
	if err != nil {
		l.Log.Info().Err(err).Msg("Failed connecting to zookeeper")
		return err
	}
	// check namespace exists
	l.Log.Info().Msg("Checking namespace exists")
	exists, _, err := l.conn.Exists(l.ZkNamespace)
	if err != nil {
		l.Log.Info().Err(err).Msg("Failed to check if namespace exists")
		return err
	}
	if !exists {
		l.Log.Info().Msg("Namespace does not exist. You must create it")
		return err
	}
	// start the leader election routine
	go func(l *LeaderElection, ctx context.Context) {
		// defer closing the connection
		defer l.conn.Close()
		var nextBackOff time.Duration
		var lastretryTime time.Time
		// start the election loop
		for {
			// reset the backoff if we haven't failed in a while
			if time.Since(lastretryTime) > 10*time.Minute {
				l.Backoff.Reset()
			}
			nextBackOff = l.Backoff.NextBackOff()
			lastretryTime = time.Now()
			l.Log.Info().Msgf("Time for next attempt %s", nextBackOff)
			select {
			case <-ctx.Done():
				return
			case <-time.NewTicker(nextBackOff).C:
				l.Log.Info().Msg("Volunteering for candidate")
				err = l.candidate()
				if err != nil {
					l.Log.Info().Err(err).Msg("Failed to volunteer for candidate")
				}
				err = l.reelectLeader()
				if err != nil {
					l.Log.Info().Err(err).Msg("Failed to re-elect leader")
				}
				err = l.processEvents()
				if err != nil {
					l.Log.Info().Err(err).Msg("Failed to process events")
				}
			}
		}
	}(l, ctx)
	return nil
}

// candidate is the function that will volunteer the current node as a candidate for leader by creating an ephemeral znode with a sequential suffix
func (l *LeaderElection) candidate() error {
	l.Log.Info().Msg("Starting leader election")
	l.conn.Create(l.ZkNamespace, []byte{}, 0, zk.WorldACL(zk.PermAll))
	znodePrefix := l.ZkNamespace + "/c_"
	znodeFullPath, err := l.conn.Create(znodePrefix, []byte{}, zk.FlagEphemeral+zk.FlagSequence, zk.WorldACL(zk.PermAll))
	if err != nil {
		l.Log.Info().Err(err).Msg("Failed to create znode")
		return err
	}
	l.currentZnodeName = strings.Replace(znodeFullPath, l.ZkNamespace+"/", "", 1)
	return nil
}

// reelectLeader is the function that will re-elect the leader if the current node is not the leader
// it will get the list of children and sort them, then check if the current node is the smallest
// if it is, it will set the IsLeader flag to true, otherwise it will set it to false
// it will also set the watchPredecessor channel to watch the predecessor node
// if the predecessor node is deleted, it will trigger an event that will be processed in the processEvents function
func (l *LeaderElection) reelectLeader() error {
	l.Log.Info().Msg("Re-electing leader")
	var predecessorName string
	var children []string
	var err error
	var exists bool
	l.watchPredecessor = nil
	for !exists {
		children, _, err = l.conn.Children(l.ZkNamespace)
		if err != nil {
			l.Log.Info().Err(err).Msg("Failed to get children")
			return err
		}
		sort.Strings(children)
		//the smallest child should be the leader
		smallestChild := children[0]
		if smallestChild == l.currentZnodeName {
			l.Log.Info().Msg("I am the leader")
			l.IsLeader = true
			return nil
		} else {
			l.Log.Info().Msg("I am not the leader")
			l.IsLeader = false
			for _, child := range children {
				if child == l.currentZnodeName {
					break
				}
				//get the child before the current node
				predecessorName = child
			}
			l.Log.Info().Msgf("Predecessor is %s", predecessorName)
			// we need to watch the predecessor node because when it is deleted, we need to re-elect the leader
			exists, _, l.watchPredecessor, err = l.conn.ExistsW(l.ZkNamespace + "/" + predecessorName)
			if err != nil {
				l.Log.Info().Err(err).Msg("Failed to get predecessor")
				return err
			}
			//this means that the predecessor has been deleted between the time we got the children and the time we tried to watch it
			if !exists {
				l.Log.Info().Msg("Predecessor does not exist")
				return nil
			}
		}
	}
	return nil
}

// processEvents is the function that will process events from the watchPredecessor channel and the connectionWatcher channel
// if the event comes from the watchPredecessor channel, it will check if the predecessor has been deleted
// if it has, it will re-elect the leader by calling the reelectLeader function
// if the event comes from the connectionWatcher channel, it will check if the connection has been lost
// if it has, it will exit the processEvents function and the leader election will be restarted with backoff retries
func (l *LeaderElection) processEvents() error {
	l.Log.Info().Msg("Processing events")
	for {
		select {
		case event := <-l.watchPredecessor:
			l.Log.Info().Msgf("Received event from predecessor %v", event)
			//TODO: check if the event for node deletion is actually received and if it
			// is received check that it is only received for that node and not for all nodes
			// also verifiy StateExpired
			if event.Type == zk.EventNodeDeleted || event.State == zk.StateDisconnected || event.State == zk.StateExpired {
				l.Log.Info().Msg("Predecessor deleted")
				err := l.reelectLeader()
				if err != nil {
					l.Log.Info().Err(err).Msg("Failed to re-elect leader")
					return err
				}
			}
		case event := <-l.connectionWatcher:
			l.Log.Info().Msgf("Received event from connection watcher %v", event)
			//TODO: check if the event for node deletion is actually received and if it
			// is received check that it is only received for that node and not for all nodes
			if event.State == zk.StateDisconnected || event.Type == zk.EventNodeDeleted {
				l.Log.Info().Msg("Disconnected")
				return nil
			}
		}
	}
}
