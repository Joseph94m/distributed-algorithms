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
	// Cancel is the cancel function for the context that will be used to cancel the election loop. You can safely use it to cancel the loop
	Cancel context.CancelFunc
	// Ctx is the context that will be used to cancel the election loop
	Ctx context.Context
	// conn is the zookeeper connection and is shared across the functions
	conn *zk.Conn
	// connectionWatcher is the channel that will be used to watch for connection events
	connectionWatcher <-chan zk.Event
	// currentZnodeName is the name of the znode that the current node is using
	currentZnodeName string
	// leaderWatcher is the channel that will be used to watch for predecessor node events
	watchPredecessor <-chan zk.Event
}

// preElection is a function that will be called before the election loop starts
// it will initialize default configs, validate the configs, and connect to zookeeper
// it will set the conn and connectionWatcher fields
// it will return an error if any of the steps fail
func (l *LeaderElection) preElection() error {
	var err error
	// perform validation on the configuration
	l.defaultConfig()
	err = l.validateConfig()
	if err != nil {
		return err
	}
	// establsih connection to zookeeper, if it fails, the function will return
	// if the connection is lost at any point after a succesful connection, it will infinitely try to reconnect
	l.Log.Info().Msg("Connecting to zookeeper")
	if l.conn != nil {
		l.conn.Close()
		l.connectionWatcher = nil
	}
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
	return nil
}

// StartElectionLoopWithoutFailureReties starts the leader election process based on the configuration provided in the struct
// if any function fails, it will return an error and will not attempt to reelect new leader.
// Note that this does not mean that the loss of leadership will cause the loop to stop.
// It will continue to run normally and attempt
// This is useful if you want to handle the failure yourself
// If you want the election to retry, use StartElectionLoopWithFailureRetries
func (l *LeaderElection) StartElectionLoopWithoutFailureReties() error {
	err := l.preElection()
	if err != nil {
		return err
	}
	// start the leader election routine
	go func(l *LeaderElection) {
		// defer closing the connection
		defer l.conn.Close()
		defer l.Cancel()
		// start the election loop

		l.Log.Info().Msg("Volunteering for candidate")
		err = l.candidate()
		if err != nil {
			l.Log.Info().Err(err).Msg("Failed to volunteer for candidate")
			return
		}
		err = l.reelectLeader()
		if err != nil {
			l.Log.Info().Err(err).Msg("Failed to re-elect leader")
			return
		}
		err = l.processEvents()
		if err != nil {
			l.Log.Info().Err(err).Msg("Failed to process events")
			return
		}
		l.Log.Info().Msg("Context cancelled. Exiting")
	}(l)
	return nil
}

// StartElectionLoop starts the leader election process based on the configuration provided in the struct
func (l *LeaderElection) StartElectionLoopWithFailureRetries() error {
	var err error
	err = l.preElection()
	if err != nil {
		l.Log.Info().Err(err).Msg("Failed to preform pre-election tasks")
		return err
	}
	firstRun := true
	// start the leader election routine
	go func(l *LeaderElection) {
		// defer closing the connection
		defer l.conn.Close()
		defer l.Cancel()
		var nextBackOff time.Duration
		var lastretryTime time.Time
		// start the election loop
		l.Log.Info().Msg("Starting leader election loop")
		for {
			// reset the backoff if we haven't failed in a while
			if time.Since(lastretryTime) > 10*time.Minute {
				l.Backoff.Reset()
			}
			// wait for the next retry using the randomized exponential backoff with jitter
			nextBackOff = l.Backoff.NextBackOff()
			lastretryTime = time.Now()
			l.Log.Info().Msgf("Time for next attempt %s", nextBackOff)
			select {
			case <-l.Ctx.Done():
				l.Log.Info().Msg("Context cancelled. Exiting")
				l.IsLeader = false
				return
			case <-time.NewTicker(nextBackOff).C:
				if !firstRun {
					l.Log.Info().Msg("Retrying election")
					err = l.preElection()
					if err != nil {
						l.Log.Log().Err(err).Msg("Failed to preform pre-election tasks %s")
						continue
					}
				}
				firstRun = false
				l.Log.Info().Msg("Volunteering for candidate")
				err = l.candidate()
				if err != nil {
					l.Log.Info().Err(err).Msg("Failed to volunteer for candidate")
					continue
				}
				err = l.reelectLeader()
				if err != nil {
					l.Log.Info().Err(err).Msg("Failed to re-elect leader")
					continue
				}
				err = l.processEvents()
				if err != nil {
					l.Log.Info().Err(err).Msg("Failed to process events")
					continue
				}
			}
		}
	}(l)
	return nil
}

// candidate is the function that will volunteer the current node as a candidate for leader by creating an ephemeral znode with a sequential suffix
func (l *LeaderElection) candidate() error {
	l.Log.Info().Msg("Starting leader election")
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
			l.IsLeader = false
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
		case <-l.Ctx.Done():
			l.IsLeader = false
			return nil
		case event := <-l.watchPredecessor:
			l.Log.Info().Msgf("Received event from predecessor %v", event)
			if event.Type == zk.EventNodeDeleted {
				l.Log.Info().Msg("Predecessor deleted")
				err := l.reelectLeader()
				if err != nil {
					l.Log.Info().Err(err).Msg("Failed to re-elect leader")
					return err
				}
			}
		case event := <-l.connectionWatcher:
			l.Log.Info().Msgf("Received event from connection watcher %v", event)
			if event.State == zk.StateDisconnected {
				l.Log.Info().Msg("Disconnected")
				l.IsLeader = false
				return nil
			}
			//TODO: if I manually delete the current node's znode, it will not trigger an event in the connectionWatcher channel
			// but for some reason it will trigger an event in the watchPredecessor channel
			// check if current node's znode has been delete event
			// the following code does not yet fix the problem but we must find it
			// if event.Type == zk.EventNodeDeleted && event.Path == l.ZkNamespace+"/"+l.currentZnodeName {
			// 	l.Log.Info().Msg("Current node's znode deleted")
			// 	return nil
			// }
		}
	}
}
