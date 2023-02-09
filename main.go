package main

import (

	// "fmt"

	"time"

	"github.com/rs/zerolog/log"
	"gitlab.mobile-intra.com/cloud-ops/distributed-algorithms/election"
)

func main() {
	leaderElection := &election.LeaderElection{
		ZkPath:        "/election",
		ZkTimeout:     time.Second * 5,
		LeaderLease:   time.Second * 5,
		ElectionSleep: time.Second * 10,
		Zookeepers:    []string{"127.0.0.1:2181", "127.0.0.1:12181", "127.0.0.1:22181"},
		Log:           log.Logger,
	}
	go leaderElection.StartElectionLoop()
	ticker := time.NewTicker(time.Second * 5)
	for range ticker.C {
		log.Info().Bool("isLeader", leaderElection.IsLeader).Msg("Is leader")
	}
}
