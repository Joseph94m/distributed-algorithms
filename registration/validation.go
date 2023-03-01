package registration

import (
	"fmt"
)

func (l *RegisterWithZookeeper) validateConfig() error {
	if len(l.Zookeepers) == 0 {
		return fmt.Errorf("no zookeepers provided")
	}
	if l.ZkNamespace == "" {
		return fmt.Errorf("no zookeeper namespace provided")
	}
	if l.ZkTimeout == 0 {
		return fmt.Errorf("no zookeeper timeout provided")
	}
	if l.Data == nil {
		return fmt.Errorf("no data provided")
	}
	return nil
}
