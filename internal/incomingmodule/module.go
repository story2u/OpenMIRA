// Package incomingmodule assembles incoming message write components.
package incomingmodule

import (
	"database/sql"
	"errors"
	"time"

	"wework-go/internal/incomingwrite"
	"wework-go/internal/infra/incomingmessagestore"
	"wework-go/internal/infra/outboxstore"
)

// ErrMessageStoreRequired means incoming writes were assembled without a message store.
var ErrMessageStoreRequired = errors.New("incoming message store is required")

// ErrOutboxStoreRequired means incoming writes were assembled without an outbox store.
var ErrOutboxStoreRequired = errors.New("incoming outbox store is required")

// Options contains dependencies needed by the incoming write module.
type Options struct {
	DB                  *sql.DB
	DBDialect           string
	MessageStore        incomingwrite.MessageStore
	Outbox              incomingwrite.OutboxEnqueuer
	OutboxAfterEnqueue  outboxstore.AfterEnqueueFunc
	CustomerReplies     incomingwrite.CustomerReplyMarker
	NextMessageID       func() int64
	RequireMessageStore bool
	RequireOutboxStore  bool
}

// Module groups the incoming write service and optional SQL repositories.
type Module struct {
	Service           incomingwrite.Service
	MessageRepository *incomingmessagestore.Repository
	OutboxRepository  *outboxstore.Repository
}

// New wires SQL-backed or injected incoming write dependencies.
func New(options Options) (Module, error) {
	nextMessageID := options.NextMessageID
	if nextMessageID == nil {
		nextMessageID = defaultNextMessageID
	}

	messageStore := options.MessageStore
	var messageRepository *incomingmessagestore.Repository
	if messageStore == nil && options.DB != nil {
		dialect := options.DBDialect
		if dialect == "" {
			dialect = incomingmessagestore.DialectMySQL
		}
		messageRepository = incomingmessagestore.NewSQLRepository(options.DB, dialect)
		messageRepository.NextMessageID = nextMessageID
		messageStore = messageRepository
	}
	if messageStore == nil && options.RequireMessageStore {
		return Module{}, ErrMessageStoreRequired
	}
	if messageStore == nil {
		return Module{}, ErrMessageStoreRequired
	}

	outbox := options.Outbox
	var outboxRepository *outboxstore.Repository
	if outbox == nil && options.DB != nil {
		dialect := options.DBDialect
		if dialect == "" {
			dialect = outboxstore.DialectMySQL
		}
		outboxRepository = outboxstore.NewSQLRepository(options.DB, dialect)
		outboxRepository.AfterEnqueue = options.OutboxAfterEnqueue
		outbox = outboxRepository
	}
	if outbox == nil && options.RequireOutboxStore {
		return Module{}, ErrOutboxStoreRequired
	}
	if outbox == nil {
		return Module{}, ErrOutboxStoreRequired
	}

	return Module{
		Service: incomingwrite.Service{
			Chat: incomingwrite.StoreChatIngestor{
				Store:         messageStore,
				NextMessageID: nextMessageID,
			},
			Outbox:          outbox,
			CustomerReplies: options.CustomerReplies,
		},
		MessageRepository: messageRepository,
		OutboxRepository:  outboxRepository,
	}, nil
}

func defaultNextMessageID() int64 {
	return time.Now().UTC().UnixNano() / int64(time.Millisecond)
}
