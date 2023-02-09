package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-zookeeper/zk"
	"github.com/rs/zerolog"
)

type LeaderElection struct {
	ZkPath    string
	ZkTimeout time.Duration
	// if the lease is not renewed in this time the leader is considered failed, i.e. the znode fades
	LeaderLease   time.Duration
	ElectionSleep time.Duration
	conn          *zk.Conn
	Data          []byte
	Zookeepers    []string
	Log           zerolog.Logger
}

// Elect starts the leader election process based on the configuration provided in the struct
func (l *LeaderElection) StartElectionLoop() {
	var err error
	l.Log.Info().Msg("Starting leader election")
	//set Data from the node hostname + random string if not provided
	if l.Data != nil {
		dataS, err := getUniqueIdentifier()
		l.Log.Info().Msgf("Node hostname: %s", dataS)
		if err != nil {
			return
		}
		l.Data = []byte(dataS)
	}
	l.Log.Info().Msg("Connecting to zookeeper")

	l.conn, _, err = zk.Connect(l.Zookeepers, l.ZkTimeout)
	if err != nil {
		l.Log.Info().Err(err).Msg("Failed connecting to zookeeper")
		return
	}
	defer l.conn.Close()
	mainCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	l.Log.Info().Msg("Starting leader election")
	l.leaderElection(mainCtx)

	//handle signals to shutdown
	done := make(chan bool, 1)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigs
		l.Log.Info().Msgf("Shutting down. Received signal %s", sig)
		cancel()
		done <- true
	}()
	<-done
}

func (l *LeaderElection) leaderElection(mainCtx context.Context) {
	var ctx context.Context
	var cancel context.CancelFunc
	var err error
	ticker := time.NewTicker(l.ElectionSleep)
	for {
		select {
		case <-mainCtx.Done():
			if cancel != nil {
				cancel()
			}
			return
		case <-ticker.C:
			l.Log.Info().Msg("Trying to become leader")
			err = l.tryBecomeLeader()
			if err == nil {
				//create context for leader lease renewal
				l.Log.Info().Msg("I acquired leadership")
				ctx, cancel = context.WithCancel(context.Background())
				go l.leaderLeaseRenewal(ctx, cancel)
				l.startLeaderWork(ctx, cancel)
			} else if err == zk.ErrNodeExists {
				l.Log.Info().Msgf("Leader already exists %s", err.Error())
			}
		}
	}
}

func (l *LeaderElection) tryBecomeLeader() error {
	var path string
	var err error
	path, err = l.conn.Create(l.ZkPath, l.Data, zk.FlagEphemeral, zk.WorldACL(zk.PermAll))
	if err != nil {
		l.Log.Info().Err(err).Msgf("Failed to create node path %s at: %s with data %s", l.ZkPath, path, string(l.Data))
		dat, _, err2 := l.conn.Get(l.ZkPath)
		if err2 != nil {
			l.Log.Info().Err(err2).Msgf("Failed to get node path %s at: %s", l.ZkPath, path)
			return err2
		}
		l.Log.Info().Msgf("Node path %s at: %s has data %s", l.ZkPath, path, string(dat))
		if string(dat) == string(l.Data) {
			l.Log.Info().Msgf("Node was my meta data")
			return nil
		}
		return err
	}
	l.Log.Info().Msgf("Created node path %s at: %s with data %s", l.ZkPath, path, string(l.Data))
	return nil
}

func (l *LeaderElection) leaderLeaseRenewal(ctx context.Context, cancel context.CancelFunc) {
	defer cancel()
	ticker := time.NewTicker(l.LeaderLease / 3)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			//renew the lease
			_, err := l.conn.Set(l.ZkPath, l.Data, -1)
			if err != nil {
				l.Log.Info().Err(err).Msg("Failed to renew leader lease")
				return
			}
			l.Log.Info().Msgf("Renewed leader lease at path: %s with data %s", l.ZkPath, string(l.Data))
		}
	}
}

func (l *LeaderElection) startLeaderWork(ctx context.Context, cancel context.CancelFunc) {
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return
		default:
			leader, _, err := l.conn.Exists(l.ZkPath)
			if err != nil {
				l.Log.Info().Err(err).Msgf("Failed to check if I am the leader at path %s", l.ZkPath)
				return
			}
			if !leader {
				l.Log.Info().Msg("I am no longer the leader")
				return
			}
			//Perform leader tasks
			l.Log.Info().Msg("I am working")
			time.Sleep(5 * time.Second)
		}
	}
}
