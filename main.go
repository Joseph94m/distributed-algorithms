package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/go-zookeeper/zk"
)

const (
	zkPath    = "/leader/consensus"
	zkTimeout = time.Second * 5
	// if the lease is not renewed in this time the leader is considered failed
	leaderLease   = time.Second * 5
	electionSleep = time.Second * 2
)

var (
	conn *zk.Conn
	data []byte
)

func main() {
	var err error
	dataS, err := getNodeHostName()
	if err != nil {
		return
	}
	data = []byte(dataS)
	conn, _, err = zk.Connect([]string{"zk1:2181", "zk2:2181", "zk3:2181"}, zkTimeout)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer conn.Close()
	mainCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go monitorConnection(mainCtx, cancel)
	leaderElection(mainCtx)
}

func monitorConnection(mainCtx context.Context, cancel context.CancelFunc) {
	defer cancel()
	var err error
	ticker := time.NewTicker(time.Second * 2)
	for {
		select {
		case <-mainCtx.Done():
			return
		case <-ticker.C:
			if conn.State() != zk.StateConnected {
				fmt.Println("Connection lost, re-establishing connection")
				for {
					conn, _, err = zk.Connect([]string{"zk1:2181", "zk2:2181", "zk3:2181"}, zkTimeout)
					if err == nil {
						break
					}
					time.Sleep(time.Second * 5)
				}
			}
		}
	}
}

func leaderElection(mainCtx context.Context) {
	var ctx context.Context
	var cancel context.CancelFunc
	ticker := time.NewTicker(electionSleep)
	for {
		select {
		case <-mainCtx.Done():
			if cancel != nil {
				cancel()
			}
			return
		case <-ticker.C:
			err := tryBecomeLeader()
			if err == nil {
				//create context for leader lease renewal
				fmt.Println("I acquired leadership")
				ctx, cancel = context.WithCancel(context.Background())
				go leaderLeaseRenewal(ctx, cancel)
				startLeaderWork(ctx, cancel)
				cancel()
			} else if err == zk.ErrNodeExists {
				fmt.Println("Leader already exists. Could be me or someone else")
			}
		}
	}
}

func getNodeHostName() (string, error) {
	return os.Hostname()
}

func tryBecomeLeader() error {
	exists, _, err := conn.Exists(zkPath)
	if err != nil {
		return err
	}
	if exists {
		return zk.ErrNodeExists
	}
	// add unique identifier to node's data
	_, err = conn.Create(zkPath, []byte(data), zk.FlagEphemeral, zk.WorldACL(zk.PermAll))
	if err != nil {
		return err
	}
	return nil
}

func leaderLeaseRenewal(ctx context.Context, cancel context.CancelFunc) {
	defer cancel()
	ticker := time.NewTicker(leaderLease / 3)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			//renew the lease
			_, err := conn.Set(zkPath, data, -1)
			if err != nil {
				fmt.Println(err)
				return
			}
		}
	}
}

func startLeaderWork(ctx context.Context, cancel context.CancelFunc) {
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return
		default:
			leader, _, err := conn.Exists(zkPath)
			if err != nil {
				fmt.Println(err)
				continue
			}
			if !leader {
				fmt.Println("I am no longer the leader")
				return
			}
			//Perform leader tasks
			fmt.Println("I am working")
			time.Sleep(time.Second)
		}
	}
}
