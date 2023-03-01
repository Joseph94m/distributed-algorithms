package registration

import (
	"context"

	backoff "github.com/cenkalti/backoff/v4"
	"github.com/rs/zerolog/log"
)

// DefaultConfig initialized some recommended default values for RegisterWithZookeeper
// You shouldn't need to call this yourself as its called in RegisterLoop
// backoff is set to exponential backoff with randonmess and infinite retry
// log is set to default zero log logger
func (l *RegisterWithZookeeper) defaultConfig() {
	if l.Backoff == nil {
		bckoff := backoff.NewExponentialBackOff()
		// set the max elapsed time to 0 so it will retry forever
		bckoff.MaxElapsedTime = 0
		l.Backoff = bckoff
	}
	if l.Log == nil {
		l.Log = &log.Logger
	}
	l.Ctx, l.Cancel = context.WithCancel(context.Background())
}
