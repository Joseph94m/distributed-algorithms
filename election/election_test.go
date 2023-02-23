package election_test

import (
	"context"
	"time"

	"github.com/go-zookeeper/zk"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gitlab.mobile-intra.com/cloud-ops/distributed-algorithms/election"
)

var _ = Describe("Election", func() {
	var (
		leaderElection *election.LeaderElection
		conn           *zk.Conn
		err            error
	)

	BeforeEach(func() {
		//connect to zookeeper and create the namespace
		conn, _, err = zk.Connect(Zookeepers, time.Second*5)
		if err != nil {
			panic(err)
		}
		conn.Delete(Namespace, -1)
		_, err = conn.Create(Namespace, []byte{}, 0, zk.WorldACL(zk.PermAll))
		if err != nil {
			panic(err)
		}
	})
	AfterEach(func() {
		leaderElection = nil
		err = deleteZNodeRecursively(conn, Namespace)
		conn.Close()
		if err != nil {
			panic(err)
		}
	})
	Describe("One running instance and default values", func() {
		BeforeEach(func() {
			// start the instance
			leaderElection = &election.LeaderElection{
				ZkNamespace: Namespace,
				ZkTimeout:   time.Second * 5,
				Zookeepers:  Zookeepers,
			}
			ctx, cancel := context.WithCancel(context.Background())
			leaderElection.StartElectionLoop(ctx)
			//timer to cancel the context after 10 seconds
			<-time.After(time.Second * 10)
			cancel()
		})
		It("That one running instance should become the leader", func() {
			//connect to zookeeper, get znode children and verify that size is 1
			children, _, err := conn.Children(Namespace)
			if err != nil {
				panic(err)
			}
			Expect(len(children)).To(Equal(1))
			Expect(leaderElection.IsLeader).To(BeTrue())
		})
	})
})
