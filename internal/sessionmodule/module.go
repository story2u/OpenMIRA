// Package sessionmodule assembles the Go session components for phase two.
// It keeps route registration outside this package so /api/v1/session/me can
// be wired, tested, and golden-compared before Python traffic is cut over.
package sessionmodule

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"wework-go/internal/auth"
	"wework-go/internal/config"
	"wework-go/internal/infra/sessionblacklist"
	"wework-go/internal/infra/sessionprofile"
	"wework-go/internal/infra/workbenchauditlogs"
	"wework-go/internal/session"
	"wework-go/internal/sessionhttp"
	"wework-go/internal/workbench"
)

// ErrProfileStoreRequired means route-ready assembly was requested without DB.
var ErrProfileStoreRequired = errors.New("session profile store is required")

// ErrBlacklistStoreRequired means route-ready assembly cannot verify revocation.
var ErrBlacklistStoreRequired = errors.New("session blacklist store is required")

// Options contains the external dependencies needed by the session module.
type Options struct {
	Config                config.Config
	DB                    *sql.DB
	DBDialect             string
	Blacklist             auth.Blacklist
	Revoker               auth.Revoker
	LoginLimiter          session.LoginAttemptLimiter
	AuditLogs             session.AuditLogWriter
	Now                   func() time.Time
	LastSeenThrottle      time.Duration
	RequireProfileStore   bool
	RequireBlacklistStore bool
}

// Module groups the unmounted session service and HTTP adapter.
type Module struct {
	Service             *session.Service
	Handler             sessionhttp.Handler
	ProfileRepository   *sessionprofile.Repository
	BlacklistRepository *sessionblacklist.Repository
	AuditLogRepository  *workbenchauditlogs.Repository
}

// New wires JWT verification, optional cs_users profile access, and HTTP glue.
func New(options Options) (Module, error) {
	verifier, err := auth.NewVerifier(options.Config.SessionJWTSecret, options.Config.SessionJWTIssuer)
	if err != nil {
		return Module{}, err
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
	if options.Now != nil {
		verifier.Now = options.Now
	}
	revoker := options.Revoker
	if revoker == nil && blacklistRepository != nil {
		revoker = blacklistRepository
	}
	if revoker == nil {
		if candidate, ok := blacklist.(auth.Revoker); ok {
			revoker = candidate
		}
	}

	service := &session.Service{
		Verifier:          verifier,
		Revoker:           revoker,
		LastSeenThrottle:  options.LastSeenThrottle,
		PasswordlessLogin: options.Config.AllowPasswordlessLogin,
		LoginLimiter:      options.LoginLimiter,
		AuditLogs:         options.AuditLogs,
		AdminCredentials: session.AdminCredentials{
			Username: options.Config.AdminUsername,
			Password: options.Config.AdminPassword,
		},
	}
	if service.LoginLimiter == nil {
		service.LoginLimiter = session.NewLoginRateLimiter(session.LoginRateLimiterOptions{
			Window:      secondsDuration(options.Config.AuthRateLimitWindowSec),
			MaxAttempts: options.Config.AuthRateLimitMaxAttempts,
			Burst:       options.Config.AuthRateLimitBurst,
			BurstWindow: secondsDuration(options.Config.AuthRateLimitBurstWindowSec),
			Now:         options.Now,
		})
	}

	var profileRepository *sessionprofile.Repository
	var auditLogRepository *workbenchauditlogs.Repository
	if options.DB != nil {
		dialect := options.DBDialect
		if dialect == "" {
			dialect = sessionprofile.DialectMySQL
		}
		profileRepository = sessionprofile.NewSQLRepository(options.DB, dialect)
		profileRepository.Now = options.Now
		service.Profiles = profileRepository
		service.Users = profileRepository
		service.LastSeen = profileRepository
		if service.AuditLogs == nil {
			auditLogRepository = workbenchauditlogs.NewSQLRepository(options.DB, dialect)
			service.AuditLogs = sessionAuditLogWriter{repository: auditLogRepository}
		}
	} else if options.RequireProfileStore {
		return Module{}, ErrProfileStoreRequired
	}

	return Module{
		Service:             service,
		Handler:             sessionhttp.New(service),
		ProfileRepository:   profileRepository,
		BlacklistRepository: blacklistRepository,
		AuditLogRepository:  auditLogRepository,
	}, nil
}

type sessionAuditLogWriter struct {
	repository *workbenchauditlogs.Repository
}

func (writer sessionAuditLogWriter) AddAuditLog(ctx context.Context, entry session.AuditLogEntry) error {
	if writer.repository == nil {
		return nil
	}
	_, err := writer.repository.AddAuditLog(ctx, workbench.AuditLogEntry{
		Operator:   entry.Operator,
		ActionType: entry.ActionType,
		Detail:     entry.Detail,
		IP:         entry.IP,
	})
	return err
}

func secondsDuration(seconds float64) time.Duration {
	if seconds <= 0 {
		return 0
	}
	return time.Duration(seconds * float64(time.Second))
}
