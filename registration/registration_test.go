package registration_test

import (
	"fmt"
	"time"

	"github.com/go-zookeeper/zk"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gitlab.mobile-intra.com/cloud-ops/distributed-algorithms/registration"
)

var _ = Describe("Registration", func() {
	var (
		reg           *registration.RegisterWithZookeeper
		conn          *zk.Conn
		err           error
		servicesCount int
		services      []*registration.RegisterWithZookeeper
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
		reg = nil
		err = deleteZNodeRecursively(conn, Namespace)
		conn.Close()
		if err != nil {
			panic(err)
		}
	})

	Describe("One running instance and default values RegisterWithoutFailureRetries", func() {
		BeforeEach(func() {
			// define the struct
			reg = defaultRegistration()
			// start the loop with cancel context
			err = reg.RegisterWithoutFailureRetries()
			if err != nil {
				panic(err)
			}
			//timer To wait before checking the znode
			<-time.After(Timeout)
		})
		AfterEach(func() {
			reg.Cancel()
		})
		It("running instance should be registered", func() {
			//connect to zookeeper, get znode children and verify that size is 1
			children, _, err := conn.Children(Namespace)
			if err != nil {
				panic(err)
			}
			Expect(len(children)).To(Equal(1))
			Expect(reg.IsRegistered).To(BeTrue())
		})
	})

	Describe("One running instance and default values RegisterWithFailureRetries", func() {
		BeforeEach(func() {
			// define the struct
			reg = defaultRegistration()
			// start the loop with cancel context
			err = reg.RegisterWithFailureRetries()
			if err != nil {
				panic(err)
			}
			//timer To wait before checking the znode
			<-time.After(Timeout)
		})
		AfterEach(func() {
			reg.Cancel()
		})
		It("running instance should be registered", func() {
			//connect to zookeeper, get znode children and verify that size is 1
			children, _, err := conn.Children(Namespace)
			if err != nil {
				panic(err)
			}
			Expect(len(children)).To(Equal(1))
			Expect(reg.IsRegistered).To(BeTrue())
		})
	})

	Describe("Run createZnode function and expect to have 1 znode", func() {
		BeforeEach(func() {
			// define the struct
			servicesCount = 1
			services = createRegistrations(servicesCount)
			// service to create znode
			if len(services) == 1 {
				reg = services[0]
			} else {
				panic("no services")
			}
			err = reg.CreateZNode()
		})
		AfterEach(func() {
			for _, service := range services {
				service.CloseConn()
			}
		})
		It("must return 1 znode with s_ prefix", func() {
			Expect(err).To(BeNil())
			//connect to zookeeper, get znode children and verify that size is 1
			children, _, err := conn.Children(Namespace)
			if err != nil {
				panic(err)
			}
			Expect(len(children)).To(Equal(1))
			Expect(children[0]).To(ContainSubstring("s_00"))
		})
	})
	Describe("Run createZnode function and expect to have 5 znode", func() {
		BeforeEach(func() {
			// define the struct
			servicesCount = 5
			services = createRegistrations(servicesCount)
			if err != nil {
				panic(err)
			}
			for _, reg := range services {
				err = reg.CreateZNode()
				if err != nil {
					panic(err)
				}
			}
		})
		AfterEach(func() {
			for _, service := range services {
				service.CloseConn()
			}
		})
		It("must return 5 znode with s_ prefix", func() {
			Expect(len(services)).To(Equal(servicesCount))
			children, _, err := conn.Children(Namespace)
			if err != nil {
				panic(err)
			}
			Expect(len(children)).To(Equal(servicesCount))
			for _, child := range children {
				Expect(child).To(ContainSubstring("s_00"))
			}
		})
	})

	Describe("Run 5 createZNodes and follow it with process events. if one dies, it should be removed from the list", func() {
		BeforeEach(func() {
			// define the struct
			servicesCount = 5
			services = createRegistrations(servicesCount)
			for _, service := range services {
				err = service.CreateZNode()
				if err != nil {
					panic(err)
				}
				go service.ProcessEvents()
			}
		})
		AfterEach(func() {
			for _, service := range services {
				service.Cancel()
				service.CloseConn()
			}
		})

		It("must return 5 znode with s_00 prefix and must have one leader. Must re-elect a new leader when first one disconnects", func() {
			//connect to zookeeper, get znode children and verify that size is 5
			Expect(len(services)).To(Equal(servicesCount))
			children, _, err := conn.Children(Namespace)
			Expect(err).To(BeNil())
			Expect(len(children)).To(Equal(servicesCount))
			registered := 0
			for _, child := range children {
				Expect(child).To(ContainSubstring("s_00"))
			}
			for _, service := range services {
				if service.IsRegistered {
					registered += 1
				}
			}
			Expect(registered).To(Equal(servicesCount))
			/*
				####################
				disconnect a node
				####################
			*/
			registered = 0
			var newServices []*registration.RegisterWithZookeeper
			services[0].Cancel()
			newServices = append(newServices, services[1:]...)
			// we must close the connection to expire the session
			services[0].CloseConn()
			// wait for a new leader to be elected and wait for session timeout
			<-time.After(Timeout)
			// verify that there is a new leader

			children, _, err = conn.Children(Namespace)
			Expect(err).To(BeNil())
			Expect(len(children)).To(Equal(servicesCount - 1))
			for _, child := range children {
				Expect(child).To(ContainSubstring("s_00"))
			}
			for _, service := range newServices {
				if service.IsRegistered {
					registered += 1
				}
			}
			Expect(registered).To(Equal(servicesCount - 1))
		})
	})

	Describe("Run 5 createZNodes and follow it with process events. they should disconnect one by one", func() {
		BeforeEach(func() {
			// define the struct
			servicesCount = 5
			services = createRegistrations(servicesCount)
			for _, service := range services {
				err = service.CreateZNode()
				if err != nil {
					panic(err)
				}
				go service.ProcessEvents()
			}
		})
		AfterEach(func() {
			for _, service := range services {
				service.Cancel()
				service.CloseConn()
			}
		})

		FIt("must return 5 znode with s_00 prefix and must have one leader. Must re-elect a new leader when first one disconnects", func() {
			//connect to zookeeper, get znode children and verify that size is 5
			Expect(len(services)).To(Equal(servicesCount))
			children, _, err := conn.Children(Namespace)
			Expect(err).To(BeNil())
			Expect(len(children)).To(Equal(servicesCount))
			registered := 0
			unregistered := 0
			for _, child := range children {
				Expect(child).To(ContainSubstring("s_00"))
			}
			for _, service := range services {
				if service.IsRegistered {
					registered += 1
				} else {
					unregistered += 1
				}
			}
			Expect(registered).To(Equal(servicesCount))
			Expect(unregistered).To(Equal(0))
			/*
				####################
				disconnect nodes one by one
				####################
			*/
			newServices := services
			newServicesCount := servicesCount
			for k := 0; k < servicesCount; k++ {
				registered = 0
				unregistered = 0
				newServicesCount -= 1
				newServices[k].Cancel()
				newServices = append(services[:k], services[k+1:]...)
				// we must close the connection to expire the session
				newServices[k].CloseConn()
				// wait for a new leader to be elected and wait for session timeout
				<-time.After(Timeout)
				// verify that there is a new leader

				children, _, err = conn.Children(Namespace)
				fmt.Println("length of children:", len(children))
				fmt.Printf("children: %v \n", children)
				Expect(err).To(BeNil())
				Expect(len(newServices)).To(Equal(newServicesCount))
				Expect(len(children)).To(Equal(newServicesCount))
				for _, child := range children {
					Expect(child).To(ContainSubstring("s_00"))
				}

				for _, candidate := range newServices {
					if candidate.IsRegistered {
						registered += 1
					} else {
						fmt.Println("unregistered:", candidate)
						unregistered += 1
					}
				}
				time.Sleep(Timeout * 2)
				if newServicesCount > 0 {
					Expect(unregistered).To(Equal(0))
					Expect(registered).To(Equal(newServicesCount))

				} else {
					Expect(unregistered).To(Equal(0))
					Expect(registered).To(Equal(0))
				}
			}
		})
	})
})
