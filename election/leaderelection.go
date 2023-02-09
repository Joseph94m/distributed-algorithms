package election

import (
	"context"
	"sync"
	"time"

	"github.com/go-zookeeper/zk"
	"github.com/rs/zerolog"
	"gitlab.mobile-intra.com/cloud-ops/distributed-algorithms/utils"
)

type LeaderElection struct {
	// ZkPath is the path to the znode in zookeeper that will be created and where the data will be stored
	// The presence of the znode indicates that some node is the leader
	// The data stored in the znode is the identifier of the node that is the leader
	// A leader knows it is the leader if it can read the znode and the data in the znode is its own identifier
	// or if it can Set the znode in the leaderLeaseRenewal function
	ZkPath string
	// ZkTimeout is the session timeout for the zookeeper connection.
	// The provided session timeout sets the amount of time for which
	// a session is considered valid after losing connection to a server. Within
	// the session timeout it's possible to reestablish a connection to a different
	// server and keep the same session. This is means any ephemeral nodes and
	// watches are maintained.
	ZkTimeout time.Duration
	// LeaderLease is the time for which the leader will hold the leadership.
	// On the zookeeper this sets the time for which the znode will be present
	// If the leader is not able to renew the lease in this time, the znode will be deleted
	// and the leader will lose the leadership, the first node that will be able to create the znode will become the new leader
	LeaderLease time.Duration
	// ElectionSleep is the time for which the node will sleep between each leadership claim attempt
	ElectionSleep time.Duration
	// Data is the data that will be stored in the znode
	// If not provided, the data will be the hostname of the node + a random string
	Data []byte
	// Zookeepers is the list of zookeeper servers
	Zookeepers []string
	// Log is the logger that will be used, it is exported to allow for configuration
	Log zerolog.Logger
	// IsLeader is a boolean that indicates if the node is the leader or not. This should be evaluated regularly by the calling program to determine what to do
	IsLeader bool
	// conn is the zookeeper connection and is shared across the functions
	conn          *zk.Conn
	isLeaderMutex *sync.Mutex
}

// Elect starts the leader election process based on the configuration provided in the struct
func (l *LeaderElection) StartElectionLoop() error {
	var err error
	// perform validation on the configuration
	err = l.validateConfig()
	if err != nil {
		return err
	}
	l.Log.Info().Msg("Starting leader election loop")
	// set Data from the node hostname + random string if not provided
	if l.Data == nil || len(l.Data) == 0 {
		dataS, err := utils.GetUniqueIdentifier()
		l.Log.Info().Msgf("Node hostname: %s", dataS)
		if err != nil {
			return err
		}
		l.Data = []byte(dataS)
	}
	// establsih connection to zookeeper, if it fails, the function will return
	// if the connection is lost at any point after a succesful connection, it will infinitely try to reconnect
	l.Log.Info().Msg("Connecting to zookeeper")
	l.conn, _, err = zk.Connect(l.Zookeepers, l.ZkTimeout)
	if err != nil {
		l.Log.Info().Err(err).Msg("Failed connecting to zookeeper")
		return err
	}
	// defer closing the connection
	defer l.conn.Close()
	// start the leader election loop
	l.isLeaderMutex = &sync.Mutex{}
	l.Log.Info().Msg("Starting leader election")
	l.leaderElection()
	return nil
}

// leaderElection is the main loop that will try to become the leader
// every ElectionSleep time it will try to create the znode
// if it succeeds, it will start the leaderLeaseRenewal function
func (l *LeaderElection) leaderElection() {
	var ctx context.Context
	var cancel context.CancelFunc
	var err error
	var justClaimedLeadership bool
	ticker := time.NewTicker(l.ElectionSleep)
	for range ticker.C {
		l.Log.Info().Msg("Trying to become leader")
		justClaimedLeadership, err = l.tryBecomeLeader()
		if err == nil {
			//create context for leader lease renewal if we just claimed leadership or if the context was nil or cancelled
			if justClaimedLeadership || ctx == nil || ctx.Err() != nil {
				l.Log.Info().Msg("I just acquired leadership")
				ctx, cancel = context.WithCancel(context.Background())
				go l.leaderLeaseRenewal(ctx, cancel)
			} else {
				l.Log.Info().Msg("I am already the leader")
			}
			// lock access to the leader variable with a mutex
			l.isLeaderMutex.Lock()
			l.IsLeader = true
			l.isLeaderMutex.Unlock()
		} else {
			if cancel != nil {
				cancel()
			}
			if err == zk.ErrNodeExists {
				l.Log.Info().Msgf("Leader already exists and it's another node %s", err.Error())
			} else {
				l.Log.Info().Msgf("Error trying to become leader %s", err.Error())
			}
		}
	}
	// if the ticker is stopped, cancel the context
	// TODO if the function is stopped, the context should be cancelled
	if cancel != nil {
		cancel()
	}
}

// tryBecomeLeader tries to create the znode, returns a boolean and an error
// the boolean indicates whether the node became the leader or not during this specific attempt.
// If the node was already the leader, it will return false and nil
// If the node just became the leader, it will return true and nil
// If the node failed to become the leader, it will return false and an error
// --------------------
// As for the workflow of the function, it will try to create the znode, if it succeeds, that means the node is the new leader
// If it fails, it will check if the node already exists, if it does, it will fetch the data from the znode and check if it's its data
// if it is, it means this node is still the leader, if it's not, it means another node is the leader
func (l *LeaderElection) tryBecomeLeader() (bool, error) {
	var path string
	var err error
	//TODO add configurable ACL
	//TODO check what happens when path returned is different from the one provided
	path, err = l.conn.Create(l.ZkPath, l.Data, zk.FlagEphemeral, zk.WorldACL(zk.PermAll))
	if err != nil {
		// if the node already exists, it means that a leader already exists, must check if it's the same node
		l.Log.Info().Err(err).Msgf("Failed to create node path %s at: %s with data %s", l.ZkPath, path, string(l.Data))
		dat, _, err2 := l.conn.Get(l.ZkPath)
		if err2 != nil {
			l.Log.Info().Err(err2).Msgf("Failed to get node path %s at: %s", l.ZkPath, path)
			return false, err2
		}
		l.Log.Info().Msgf("Node path %s at: %s has data %s", l.ZkPath, path, string(dat))
		// if the data is the same, it means that this node is the leader
		if string(dat) == string(l.Data) {
			l.Log.Info().Msgf("Node was my metadata. I am the leader")
			return false, nil
		}
		return false, err
	}
	// if the node was created, it means that this node is the leader
	l.Log.Info().Msgf("Created node path %s at: %s with data %s", l.ZkPath, path, string(l.Data))
	return true, nil
}

// leaderLeaseRenewal is a function that will renew the leader lease every LeaderLease/3
// if it fails to renew the lease, it will cancel the context it received from leaderElection and the node will no longer be the leader
// cancelling the context will stop the leaderLeaseRenewal function
// if for any reason the function returns, it will cancel the context, this is not currently useful but might be in the future if we want to add
// more functions to the leaderElection loop
func (l *LeaderElection) leaderLeaseRenewal(ctx context.Context, cancel context.CancelFunc) {
	defer cancel()
	ticker := time.NewTicker(l.LeaderLease / 3)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// get the data from the znode
			dat, _, err := l.conn.Get(l.ZkPath)
			if err != nil {
				l.Log.Info().Err(err).Msg("Failed to get leader lease")
				return
			}
			// if the data is not the same, it means that another node is the leader
			if string(dat) != string(l.Data) {
				l.Log.Info().Msgf("Leader lease data is not the same as mine: %s != %s", string(dat), string(l.Data))
				return
			}
			//renew the lease
			_, err = l.conn.Set(l.ZkPath, l.Data, -1)
			if err != nil {
				l.Log.Info().Err(err).Msg("Failed to renew leader lease")
				return
			}
			l.Log.Info().Msgf("Renewed leader lease at path: %s with data %s", l.ZkPath, string(l.Data))
		}
	}
}
