package election

import (
	"fmt"
)

func (l *LeaderElection) validateConfig() error {
	if len(l.Zookeepers) == 0 {
		return fmt.Errorf("no zookeepers provided")
	}
	if l.ZkPath == "" {
		return fmt.Errorf("no zookeeper path provided")
	}
	if l.ZkTimeout == 0 {
		return fmt.Errorf("no zookeeper timeout provided")
	}
	if l.LeaderLease == 0 {
		return fmt.Errorf("no leader lease provided")
	}
	if l.ElectionSleep == 0 {
		return fmt.Errorf("no election sleep provided")
	}
	return nil
}
