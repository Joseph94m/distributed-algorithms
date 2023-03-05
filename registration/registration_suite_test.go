package registration_test

import (
	"testing"
	"time"

	"github.com/go-zookeeper/zk"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
	"gitlab.mobile-intra.com/cloud-ops/distributed-algorithms/registration"
)

var (
	Namespace  = "/registration"
	Zookeepers = []string{"127.0.0.1:2181"}
	Timeout    = time.Second * 2
)

func TestTask(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Registration Suite")
}

var _ = BeforeSuite(func() {
	format.MaxLength = 0
})

func deleteZNodeRecursively(conn *zk.Conn, path string) error {
	children, _, _ := conn.Children(path)

	for _, child := range children {
		childPath := path + "/" + child
		deleteZNodeRecursively(conn, childPath)

	}
	conn.Delete(path, -1)

	return nil
}

func defaultRegistration() *registration.RegisterWithZookeeper {
	registration := &registration.RegisterWithZookeeper{
		ZkNamespace: Namespace,
		ZkTimeout:   Timeout,
		Zookeepers:  Zookeepers,
		Data:        []byte("test"),
	}

	registration.DefaultConfig()
	return registration
}

func createRegistrations(number int) []*registration.RegisterWithZookeeper {
	var candidates []*registration.RegisterWithZookeeper
	var candidate *registration.RegisterWithZookeeper
	var connection *zk.Conn
	var watcher <-chan zk.Event
	var err error
	for number > 0 {
		candidate = defaultRegistration()
		connection, watcher, err = zk.Connect(candidate.Zookeepers, candidate.ZkTimeout)
		if err != nil {
			panic(err)
		}
		candidate.SetConn(connection)
		candidate.SetWatcher(watcher)
		candidates = append(candidates, candidate)
		number -= 1
	}
	return candidates
}
