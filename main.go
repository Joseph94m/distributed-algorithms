package main

import (

	// "fmt"

	"time"

	"github.com/go-zookeeper/zk"
	"github.com/rs/zerolog/log"
	"gitlab.mobile-intra.com/cloud-ops/distributed-algorithms/election"
)

func main() {
	leaderElection := &election.LeaderElection{
		ZkNamespace: "/election",
		ZkTimeout:   time.Second * 5,
		Zookeepers:  []string{"127.0.0.1:2181", "127.0.0.1:12181", "127.0.0.1:22181"},
		// Log:         &log.Logger,
	}
	//connect and create namespace
	conn, _, err := zk.Connect(leaderElection.Zookeepers, leaderElection.ZkTimeout)
	if err != nil {
		panic(err)
	}
	defer conn.Close()
	_, err = conn.Create(leaderElection.ZkNamespace, []byte{}, 0, zk.WorldACL(zk.PermAll))
	if err != nil && err != zk.ErrNodeExists {
		panic(err)
	}
	leaderElection.StartElectionLoopWithFailureRetries()
	ticker := time.NewTicker(time.Second * 5)
	tickerCancel := time.NewTicker(time.Second * 20)
	go func() {
		<-tickerCancel.C
		leaderElection.Cancel()
	}()
	for {
		select {
		case <-leaderElection.Ctx.Done():
			log.Info().Msg("Done")
			return
		case <-ticker.C:
			log.Info().Bool("isLeader", leaderElection.IsLeader).Msg("Is leader")
		}
	}
}
