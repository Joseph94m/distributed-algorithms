package election_test

import (
	"testing"

	"github.com/go-zookeeper/zk"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
)

var (
	Namespace  = "/election"
	Zookeepers = []string{"127.0.0.1:2181"}
)

func TestTask(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Election Suite")
}

var _ = BeforeSuite(func() {
	format.MaxLength = 0
})

func deleteZNodeRecursively(conn *zk.Conn, path string) error {
	children, _, err := conn.Children(path)
	if err != nil {
		return err
	}
	for _, child := range children {
		childPath := path + "/" + child
		err = deleteZNodeRecursively(conn, childPath)
		if err != nil {
			return err
		}
	}
	err = conn.Delete(path, -1)
	if err != nil {
		return err
	}
	return nil
}
