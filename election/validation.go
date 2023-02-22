package election

import (
	"fmt"
)

func (l *LeaderElection) validateConfig() error {
	if len(l.Zookeepers) == 0 {
		return fmt.Errorf("no zookeepers provided")
	}
	if l.ZkNamespace == "" {
		return fmt.Errorf("no zookeeper namespace provided")
	}
	if l.ZkTimeout == 0 {
		return fmt.Errorf("no zookeeper timeout provided")
	}
	return nil
}
