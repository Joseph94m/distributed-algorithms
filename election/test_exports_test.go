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

func (l *LeaderElection) CloseConn() {
	l.conn.Close()
}

func (l *LeaderElection) DefaultConfig() {
	l.defaultConfig()
}

func (l *LeaderElection) SetWatcher(watcher <-chan zk.Event) {
	l.connectionWatcher = watcher
}
