// Package messagesmodule assembles conversation message candidate components.
// Route registration stays outside this package so message detail payloads can
// be verified before Python traffic is cut over.
package messagesmodule

import (
	"database/sql"
	"errors"
	"time"

	"wework-go/internal/archivemedia"
	"wework-go/internal/auth"
	"wework-go/internal/config"
	"wework-go/internal/infra/messagestore"
	"wework-go/internal/infra/sessionblacklist"
	"wework-go/internal/messages"
	"wework-go/internal/messageshttp"
)

// ErrStoreRequired means route-ready assembly was requested without a store.
var ErrStoreRequired = errors.New("conversation messages store is required")

// ErrBlacklistStoreRequired means route-ready assembly cannot verify revocation.
var ErrBlacklistStoreRequired = errors.New("conversation messages blacklist store is required")

// Options contains dependencies needed by the message detail candidate module.
type Options struct {
	Config                config.Config
	DB                    *sql.DB
	DBDialect             string
	Store                 messages.Store
	Blacklist             auth.Blacklist
	MediaURLBuilder       messagestore.MediaURLBuilder
	Now                   func() time.Time
	RequireBlacklistStore bool
}

// Module groups the unmounted message service and HTTP adapter.
type Module struct {
	Service             *messages.Service
	Handler             messageshttp.Handler
	StoreRepository     *messagestore.Repository
	BlacklistRepository *sessionblacklist.Repository
}

// New wires JWT guard, message store, and HTTP glue.
func New(options Options) (Module, error) {
	verifier, err := auth.NewVerifier(options.Config.SessionJWTSecret, options.Config.SessionJWTIssuer)
	if err != nil {
		return Module{}, err
	}
	if options.Now != nil {
		verifier.Now = options.Now
	}

	var blacklistRepository *sessionblacklist.Repository
	blacklist := options.Blacklist
	if blacklist == nil && options.DB != nil {
		dialect := options.DBDialect
		if dialect == "" {
			dialect = sessionblacklist.DialectMySQL
		}
		blacklistRepository = sessionblacklist.NewSQLRepository(options.DB, dialect)
		blacklistRepository.Now = options.Now
		blacklist = blacklistRepository
	}
	if blacklist == nil && options.RequireBlacklistStore {
		return Module{}, ErrBlacklistStoreRequired
	}
	verifier.Blacklist = blacklist

	store := options.Store
	var storeRepository *messagestore.Repository
	if store == nil && options.DB != nil {
		storeRepository = messagestore.NewSQLRepository(options.DB)
		if options.MediaURLBuilder != nil {
			storeRepository.MediaURLBuilder = options.MediaURLBuilder
		} else {
			storeRepository.MediaURLBuilder = archivemedia.AccessURLBuilder{
				BaseURL:               options.Config.ArchiveMediaBaseURL,
				ObjectPublicBaseURL:   options.Config.ArchiveMediaObjectPublicBaseURL,
				PreferDirectObjectURL: options.Config.ArchiveMediaDirectObjectURL,
				SigningKey:            options.Config.ArchiveMediaSigningKey,
				TokenTTL:              time.Duration(options.Config.ArchiveMediaTokenTTLSeconds) * time.Second,
				Now:                   options.Now,
			}
		}
		store = storeRepository
	}
	if store == nil {
		return Module{}, ErrStoreRequired
	}

	service := &messages.Service{Store: store}
	guard := auth.Guard{Verifier: verifier}
	return Module{
		Service:             service,
		Handler:             messageshttp.New(guard, service),
		StoreRepository:     storeRepository,
		BlacklistRepository: blacklistRepository,
	}, nil
}
