package wsgateway

import (
	"context"
	"time"
)

// BrokerFeed streams raw Redis broker payloads.
type BrokerFeed interface {
	Messages() <-chan []byte
	Close() error
}

// Listener connects a broker feed to the local websocket hub.
type Listener struct {
	Hub         *Hub
	Feed        BrokerFeed
	LocalOrigin string
}

// Start consumes broker messages until ctx is cancelled or the feed closes.
func (listener Listener) Start(ctx context.Context) func() error {
	if listener.Hub == nil || listener.Feed == nil {
		return func() error { return nil }
	}
	if ctx == nil {
		ctx = context.Background()
	}
	listenCtx, cancel := context.WithCancel(ctx)
	messages := listener.Feed.Messages()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-listenCtx.Done():
				return
			case payload, ok := <-messages:
				if !ok {
					return
				}
				_, _ = listener.Hub.DeliverBrokerPayload(listenCtx, payload, listener.LocalOrigin)
			}
		}
	}()
	return func() error {
		cancel()
		err := listener.Feed.Close()
		select {
		case <-done:
		case <-time.After(time.Second):
		}
		return err
	}
}
