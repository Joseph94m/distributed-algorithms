package main

import (
	"fmt"
	"time"

	"github.com/go-zookeeper/zk"
)

var (
	leaseRenewal      = time.Second * 2
	connectionRetry   = time.Second * 5
	connectionTimeout = time.Second * 5
)

func register() {
	dataS, err := getHostname()
	if err != nil {
		return
	}
	data := []byte(dataS)
	retryTicker := time.NewTicker(connectionRetry)
	for range retryTicker.C {
		// Connect to the ZooKeeper ensemble
		conn, _, err := zk.Connect([]string{"zk1:2181", "zk2:2181", "zk3:2181"}, connectionTimeout)
		if err != nil {
			fmt.Println(err)
			continue
		}

		// Create an ephemeral node to represent the server
		path := "/myapp/instances/" + dataS
		_, err = conn.Create(path, data, zk.FlagEphemeral, zk.WorldACL(zk.PermAll))
		if err != nil {
			if err == zk.ErrNodeExists {
				fmt.Println("Another instance of the server is running")
				// take appropriate action
				return
			}
			fmt.Println(err)
			continue
		}

		fmt.Println("Server registered with ZooKeeper at path and data", path, dataS)

		// Refresh the node's data to keep the session alive
		leaseTicker := time.NewTicker(leaseRenewal)
		for range leaseTicker.C {
			_, err = conn.Set(path, data, -1)
			if err != nil {
				fmt.Println(err)
				if err == zk.ErrNoNode {
					fmt.Println("Node does not exist, re-registering the server")
					conn.Close()
					break
				}
				if err == zk.ErrConnectionClosed {
					fmt.Println("Lost connection to the ensemble, re-connecting")
					conn.Close()
					break
				}
				// handle other errors
			}
		}

		// Monitor the session for expiry, timeout or network failures
		event := conn.State()
		if event == zk.StateExpired || event == zk.StateDisconnected || event == zk.StateAuthFailed {
			fmt.Println("Session expired, closed or authentication failed, re-registering the server")
			conn.Close()
			continue
		}
	}
}
