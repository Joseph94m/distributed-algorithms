package registration

import "github.com/go-zookeeper/zk"

func (l *RegisterWithZookeeper) CreateZNode() error {
	return l.createZNode()
}

func (l *RegisterWithZookeeper) ProcessEvents() error {
	return l.processEvents()
}

func (l *RegisterWithZookeeper) SetConn(conn *zk.Conn) {
	l.conn = conn
}

func (l *RegisterWithZookeeper) CloseConn() {
	l.conn.Close()
}

func (l *RegisterWithZookeeper) DefaultConfig() {
	l.defaultConfig()
}

func (l *RegisterWithZookeeper) SetWatcher(watcher <-chan zk.Event) {
	l.connectionWatcher = watcher
}
