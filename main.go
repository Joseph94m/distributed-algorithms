package main

import (

	// "fmt"

	"time"

	"github.com/rs/zerolog/log"
)

func main() {
	leaderElection := &LeaderElection{
		ZkPath:        "/election",
		ZkTimeout:     time.Second * 5,
		LeaderLease:   time.Second * 5,
		ElectionSleep: time.Second * 8,
		Zookeepers:    []string{"127.0.0.1:2181", "127.0.0.1:12181", "127.0.0.1:22181"},
		Log:           log.Logger,
	}
	leaderElection.StartElectionLoop()
}
