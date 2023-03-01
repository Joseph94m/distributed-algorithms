package election_test

import (
	"fmt"
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
		candidateCount int
		candidates     []*election.LeaderElection
		// connectionWatchers []<-chan zk.Event
	)

	BeforeEach(func() {
		//connect to zookeeper and create the namespace
		conn, _, err = zk.Connect(Zookeepers, Timeout)
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

	Describe("One running instance and default values StartElectionLoopWithoutFailureReties", func() {
		BeforeEach(func() {
			// define the struct
			leaderElection = defaultLeaderElection()
			// start the loop with cancel context
			err = leaderElection.StartElectionLoopWithoutFailureReties()
			if err != nil {
				panic(err)
			}
			//timer To wait before checking the znode
			<-time.After(Timeout)
		})
		AfterEach(func() {
			leaderElection.Cancel()
		})
		It("running instance should become the leader", func() {
			//connect to zookeeper, get znode children and verify that size is 1
			children, _, err := conn.Children(Namespace)
			if err != nil {
				panic(err)
			}
			Expect(len(children)).To(Equal(1))
			Expect(leaderElection.IsLeader).To(BeTrue())
		})
	})

	Describe("One running instance and default values StartElectionLoopWithoutFailureRetries", func() {
		BeforeEach(func() {
			// define the struct
			leaderElection = defaultLeaderElection()
			// start the loop with cancel context
			err = leaderElection.StartElectionLoopWithFailureRetries()
			if err != nil {
				panic(err)
			}
			//timer To wait before checking the znode
			<-time.After(Timeout)
		})
		AfterEach(func() {
			leaderElection.Cancel()
		})
		It("running instance should become the leader", func() {
			//connect to zookeeper, get znode children and verify that size is 1
			children, _, err := conn.Children(Namespace)
			if err != nil {
				panic(err)
			}
			Expect(len(children)).To(Equal(1))
			Expect(leaderElection.IsLeader).To(BeTrue())
		})
	})

	Describe("Run candidate function and expect to have 1 znode", func() {
		BeforeEach(func() {
			// define the struct
			candidateCount = 1
			candidates = createCandidates(candidateCount)
			// candidate to create znode
			if len(candidates) == 1 {
				leaderElection = candidates[0]
			} else {
				panic("no candidates")
			}
			err = leaderElection.Candidate()
		})
		AfterEach(func() {
			for _, candidate := range candidates {
				candidate.CloseConn()
			}
		})
		It("must return 1 znode with c_ prefix", func() {
			Expect(err).To(BeNil())
			//connect to zookeeper, get znode children and verify that size is 1
			children, _, err := conn.Children(Namespace)
			if err != nil {
				panic(err)
			}
			Expect(len(children)).To(Equal(1))
			Expect(children[0]).To(ContainSubstring("c_00"))
		})
	})
	Describe("Run 5 candidates functions and expect to have 5 znode", func() {
		BeforeEach(func() {
			// define the struct
			candidateCount = 5
			candidates = createCandidates(candidateCount)
			if err != nil {
				panic(err)
			}
			for _, candidate := range candidates {
				err = candidate.Candidate()
				if err != nil {
					panic(err)
				}
			}
		})
		AfterEach(func() {
			for _, candidate := range candidates {
				candidate.CloseConn()
			}
		})
		It("must return 5 znodes with c_ prefix", func() {
			//connect to zookeeper, get znode children and verify that size is 5
			Expect(len(candidates)).To(Equal(candidateCount))
			children, _, err := conn.Children(Namespace)
			if err != nil {
				panic(err)
			}
			Expect(len(children)).To(Equal(candidateCount))
			for _, child := range children {
				Expect(child).To(ContainSubstring("c_00"))
			}
		})
	})

	Describe("Run 1 candidates function and follow it with a re-elect. Node should become leader", func() {
		BeforeEach(func() {
			// define the struct
			candidateCount = 1
			candidates = createCandidates(candidateCount)
			for _, candidate := range candidates {
				err = candidate.Candidate()
				if err != nil {
					panic(err)
				}
				err = candidate.ReelectLeader()
				if err != nil {
					panic(err)
				}
			}
		})
		AfterEach(func() {
			for _, candidate := range candidates {
				candidate.CloseConn()
			}
		})
		It("must return 1 znode with c_ prefix and must be the leader", func() {
			//connect to zookeeper, get znode children and verify that size is 1
			Expect(len(candidates)).To(Equal(candidateCount))
			children, _, err := conn.Children(Namespace)
			if err != nil {
				panic(err)
			}
			Expect(len(children)).To(Equal(candidateCount))
			for _, child := range children {
				Expect(child).To(ContainSubstring("c_00"))
			}
			Expect(candidates[0].IsLeader).To(BeTrue())
		})
	})

	Describe("Run 5 candidates function and follow it with a re-elect. Only one of the nodes should become the leader.", func() {
		BeforeEach(func() {
			// define the struct
			candidateCount = 5
			candidates = createCandidates(candidateCount)
			for _, candidate := range candidates {
				err = candidate.Candidate()
				if err != nil {
					panic(err)
				}
				err = candidate.ReelectLeader()
				if err != nil {
					panic(err)
				}
			}
		})
		AfterEach(func() {
			for _, candidate := range candidates {
				candidate.Cancel()
				candidate.CloseConn()
			}
		})

		It("must return 5 znode with c_00 prefix and must have one leader.", func() {
			//connect to zookeeper, get znode children and verify that size is 5
			Expect(len(candidates)).To(Equal(candidateCount))
			children, _, err := conn.Children(Namespace)
			Expect(err).To(BeNil())
			Expect(len(children)).To(Equal(candidateCount))
			followers := 0
			leaders := 0
			for _, child := range children {
				Expect(child).To(ContainSubstring("c_00"))
			}
			for _, candidate := range candidates {
				if candidate.IsLeader {
					leaders += 1
				} else {
					followers += 1
				}
			}
			Expect(leaders).To(Equal(1))
			Expect(followers).To(Equal(candidateCount - 1))
		})
	})

	Describe("Run 5 candidates function and follow it with a re-elect. Only one of the nodes should become the leader. if the leader dies, another one is reelected", func() {
		BeforeEach(func() {
			// define the struct
			candidateCount = 5
			candidates = createCandidates(candidateCount)
			for _, candidate := range candidates {
				err = candidate.Candidate()
				if err != nil {
					panic(err)
				}
				err = candidate.ReelectLeader()
				if err != nil {
					panic(err)
				}
				go candidate.ProcessEvents()
			}
		})
		AfterEach(func() {
			for _, candidate := range candidates {
				candidate.Cancel()
				candidate.CloseConn()
			}
		})

		It("must return 5 znode with c_00 prefix and must have one leader. Must re-elect a new leader when first one disconnects", func() {
			//connect to zookeeper, get znode children and verify that size is 5
			Expect(len(candidates)).To(Equal(candidateCount))
			children, _, err := conn.Children(Namespace)
			Expect(err).To(BeNil())
			Expect(len(children)).To(Equal(candidateCount))
			followers := 0
			leaders := 0
			for _, child := range children {
				Expect(child).To(ContainSubstring("c_00"))
			}
			for _, candidate := range candidates {
				if candidate.IsLeader {
					leaders += 1
				} else {
					followers += 1
				}
			}
			Expect(leaders).To(Equal(1))
			Expect(followers).To(Equal(candidateCount - 1))
			/*
				####################
				disconnect the leader
				####################
			*/
			followers = 0
			leaders = 0
			var newCandidates []*election.LeaderElection
			for i, candidate := range candidates {
				if candidate.IsLeader {
					candidate.Cancel()
					newCandidates = append(candidates[:i], candidates[i+1:]...)
					// we must close the connection to expire the session
					candidate.CloseConn()
				}
			}
			// wait for a new leader to be elected and wait for session timeout
			<-time.After(Timeout)
			// verify that there is a new leader

			children, _, err = conn.Children(Namespace)
			Expect(err).To(BeNil())
			Expect(len(children)).To(Equal(candidateCount - 1))
			for _, child := range children {
				Expect(child).To(ContainSubstring("c_00"))
			}

			for _, candidate := range newCandidates {
				if candidate.IsLeader {
					leaders += 1
				} else {
					followers += 1
				}
			}
			Expect(leaders).To(Equal(1))
			Expect(followers).To(Equal(candidateCount - 2))
		})
	})

	Describe("Run 5 candidates function and follow it with a re-elect. Only one of the nodes should become the leader. The leaders will die one by one until none is left", func() {
		BeforeEach(func() {
			// define the struct
			candidateCount = 5
			candidates = createCandidates(candidateCount)
			for _, candidate := range candidates {
				err = candidate.Candidate()
				if err != nil {
					panic(err)
				}
				err = candidate.ReelectLeader()
				if err != nil {
					panic(err)
				}
				go candidate.ProcessEvents()
			}
		})
		AfterEach(func() {
			for _, candidate := range candidates {
				candidate.Cancel()
				candidate.CloseConn()
			}
		})

		It("must return 5 znode with c_00 prefix and must have one leader. Must re-elect a new leader whenever one disconnects", func() {
			//connect to zookeeper, get znode children and verify that size is 5
			Expect(len(candidates)).To(Equal(candidateCount))
			children, _, err := conn.Children(Namespace)
			Expect(err).To(BeNil())
			Expect(len(children)).To(Equal(candidateCount))
			followers := 0
			leaders := 0
			for _, child := range children {
				Expect(child).To(ContainSubstring("c_00"))
			}
			for _, candidate := range candidates {
				if candidate.IsLeader {
					leaders += 1
				} else {
					followers += 1
				}
			}
			Expect(leaders).To(Equal(1))
			Expect(followers).To(Equal(candidateCount - 1))
			/*
				####################
				disconnect the leaders one by one
				####################
			*/
			newCandidates := candidates
			newCandidateCount := candidateCount
			for k := 0; k < candidateCount; k++ {
				followers = 0
				leaders = 0
				tmpCandidates := []*election.LeaderElection{}
				newCandidateCount -= 1
				for i, candidate := range newCandidates {
					if candidate.IsLeader {
						candidate.Cancel()
						tmpCandidates = append(newCandidates[:i], newCandidates[i+1:]...)
						// we must close the connection to expire the session
						candidate.CloseConn()
					}
				}
				newCandidates = tmpCandidates
				// wait for a new leader to be elected and wait for session timeout
				<-time.After(Timeout)
				// verify that there is a new leader

				children, _, err = conn.Children(Namespace)
				fmt.Println("length of children:", len(children))
				fmt.Printf("children: %v \n", children)
				Expect(err).To(BeNil())
				Expect(len(newCandidates)).To(Equal(newCandidateCount))
				Expect(len(children)).To(Equal(newCandidateCount))
				for _, child := range children {
					Expect(child).To(ContainSubstring("c_00"))
				}

				for _, candidate := range newCandidates {
					if candidate.IsLeader {
						leaders += 1
					} else {
						followers += 1
					}
				}
				if newCandidateCount > 0 {
					Expect(leaders).To(Equal(1))
					Expect(followers).To(Equal(newCandidateCount - 1))
				} else {
					Expect(leaders).To(Equal(0))
					Expect(followers).To(Equal(0))
				}
			}
		})
	})
})
