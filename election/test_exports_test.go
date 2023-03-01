package election

import "github.com/go-zookeeper/zk"

func (l *LeaderElection) Candidate() error {
	return l.candidate()
}

func (l *LeaderElection) ReelectLeader() error {
	return l.reelectLeader()
}
func (l *LeaderElection) ProcessEvents() error {
	return l.processEvents()
}

func (l *LeaderElection) SetConn(conn *zk.Conn) {
	l.conn = conn
}

func (l *LeaderElection) DefaultConfig() {
	l.defaultConfig()
}
