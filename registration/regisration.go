package registration

import (
	"context"
	"fmt"
	"time"

	backoff "github.com/cenkalti/backoff/v4"
	"github.com/go-zookeeper/zk"
	"github.com/rs/zerolog"
)

// RegisterWithZookeeper registers the server with ZooKeeper
type RegisterWithZookeeper struct {
	ZkNamespace       string
	ZkTimeout         time.Duration
	Zookeepers        []string
	Data              []byte
	Ctx               context.Context
	Cancel            context.CancelFunc
	connectionWatcher <-chan zk.Event
	conn              *zk.Conn
	Log               *zerolog.Logger
	Backoff           backoff.BackOff
	IsRegistered      bool
}

// preElection is a function that will be called before the election loop starts
// it will initialize default configs, validate the configs, and connect to zookeeper
// it will set the conn and connectionWatcher fields
// it will return an error if any of the steps fail
func (r *RegisterWithZookeeper) preRegistration() error {
	var err error
	// init basic configs
	r.defaultConfig()
	// validate configs
	err = r.validateConfig()
	if err != nil {
		return err
	}
	// establish connection to zookeeper, if it fails, the function will return
	// if the connection is lost at any point after a succesful connection, it will infinitely try to reconnect or until the connection is closed
	r.Log.Info().Msg("Connecting to zookeeper")
	if r.conn != nil {
		r.conn.Close()
		r.connectionWatcher = nil
	}
	r.conn, r.connectionWatcher, err = zk.Connect(r.Zookeepers, r.ZkTimeout)
	if err != nil {
		r.Log.Info().Err(err).Msg("Failed connecting to zookeeper")
		return err
	}

	// check namespace exists
	r.Log.Info().Msg("Checking namespace exists")
	exists, _, err := r.conn.Exists(r.ZkNamespace)
	if err != nil {
		r.Log.Info().Err(err).Msg("Failed to check if namespace exists")
		return err
	}
	if !exists {
		r.Log.Info().Msg("Namespace does not exist. You must create it")
		return err
	}
	return nil
}

// RegisterWithoutFailureRetries starts the registration process based on the configuration provided in the struct
// if any function fails, it will return an error and will not attempt to re-register
// This is useful if you want to handle the failure yourself
// If you want the election to retry, use RegisterWithFailureRetries
func (r *RegisterWithZookeeper) RegisterWithoutFailureRetries() error {
	var err error
	err = r.preRegistration()
	if err != nil {
		return err
	}
	go func(r *RegisterWithZookeeper) {
		defer r.Cancel()
		defer r.conn.Close()
		// start the registration
		err = r.createZNode()
		if err != nil {
			r.Log.Error().Err(err).Msg("Error creating znode")
			return
		}
		err = r.processEvents()
		if err != nil {
			r.Log.Error().Err(err).Msg("Error processing events")
			return
		}
		r.Log.Info().Msg("Context cancelled. Exiting")
	}(r)
	return nil
}

// RegisterWithFailureRetries will call preRegistration and then register with zookeeper
// for as long as the session holds, or until the context is cancelled or the connection is closed, it will remain registered
// if the connection is lost, or if any other error occurs, it will retry with a backoff indefinitely
func (r *RegisterWithZookeeper) RegisterWithFailureRetries() error {
	var err error
	err = r.preRegistration()
	if err != nil {
		return err
	}
	firstRun := true
	go func(r *RegisterWithZookeeper) {
		defer r.Cancel()
		defer r.conn.Close()
		var nextBackOff time.Duration
		var lastretryTime time.Time
		// start the registration loop
		for {
			if time.Since(lastretryTime) > 10*time.Minute {
				r.Backoff.Reset()
			}
			nextBackOff = r.Backoff.NextBackOff()
			lastretryTime = time.Now()
			r.Log.Info().Msgf("Time for next attempt %s", nextBackOff)
			select {
			case <-r.Ctx.Done():
				r.Log.Info().Msg("Context cancelled. Exiting")
				r.IsRegistered = false
				return
			case <-time.NewTicker(nextBackOff).C:
				if !firstRun {
					r.Log.Info().Msg("Retrying election")
					err = r.preRegistration()
					if err != nil {
						r.Log.Log().Err(err).Msg("Failed to preform pre-registration tasks")
						continue
					}
				}
				firstRun = false
				err = r.createZNode()
				if err != nil {
					r.Log.Error().Err(err).Msg("Error creating znode")
					continue
				}
				err = r.processEvents()
				if err != nil {
					r.Log.Error().Err(err).Msg("Error processing events")
					continue
				}
			}
		}
	}(r)
	return nil
}

func (r *RegisterWithZookeeper) createZNode() error {
	// Create an ephemeral node to represent the server
	znodePrefix := fmt.Sprintf("%s/s_", r.ZkNamespace)
	_, err := r.conn.Create(znodePrefix, r.Data, zk.FlagEphemeral+zk.FlagSequence, zk.WorldACL(zk.PermAll))
	if err != nil {
		r.Log.Info().Err(err).Msg("Failed to create znode")
		r.IsRegistered = false
		return err
	}
	r.IsRegistered = true
	return nil
}
func (r *RegisterWithZookeeper) processEvents() error {
	for {
		select {
		case <-r.Ctx.Done():
			r.Log.Info().Msg("Context cancelled. Exiting events loop")
			r.IsRegistered = false
			return nil
		case event := <-r.connectionWatcher:
			if event.State == zk.StateDisconnected {
				r.Log.Info().Msg("Disconnected from ZooKeeper")
				r.IsRegistered = false
				return nil
			}
		}
	}
}
