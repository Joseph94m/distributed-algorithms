package election

import (
	backoff "github.com/cenkalti/backoff/v4"
	"github.com/rs/zerolog/log"
)

func (l *LeaderElection) defaultConfig() {
	if l.Backoff == nil {
		bckoff := backoff.NewExponentialBackOff()
		// set the max elapsed time to 0 so it will retry forever
		bckoff.MaxElapsedTime = 0
		l.Backoff = bckoff
	}
	if l.Log == nil {
		l.Log = &log.Logger
	}
}
