// Package sqldb opens the legacy cloud database for the Go rewrite.
// It mirrors Python's CLOUD_DB_DSN scheme support while keeping connection
// pooling and driver selection in one infrastructure boundary.
package sqldb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
)

const (
	// DialectMySQL is the production default backend.
	DialectMySQL = "mysql"
	// DialectPostgres is retained for compatibility with historical deployments.
	DialectPostgres = "postgres"
)

var (
	// ErrMissingDSN means CLOUD_DB_DSN/DATABASE_URL was not configured.
	ErrMissingDSN = errors.New("cloud database dsn is required")
	// ErrUnsupportedDSN means the DSN scheme is not mysql/postgres compatible.
	ErrUnsupportedDSN = errors.New("cloud database dsn scheme is unsupported")

	beijingLocation = time.FixedZone("Asia/Shanghai", 8*60*60)
)

// Options controls database/sql opening and pool defaults.
type Options struct {
	DSN             string
	RuntimeRole     string
	MaxIdleConns    int
	MaxOpenConns    int
	ConnMaxIdleTime time.Duration
	ConnMaxLifetime time.Duration
	PingTimeout     time.Duration
	SkipPing        bool
}

// Database is an opened SQL handle plus its compatibility metadata.
type Database struct {
	DB         *sql.DB
	Dialect    string
	DriverName string
	DriverDSN  string
	MaskedDSN  string
}

// ResolvedDSN describes how a legacy CLOUD_DB_DSN maps to database/sql.
type ResolvedDSN struct {
	Dialect    string
	DriverName string
	DriverDSN  string
	MaskedDSN  string
}

// Open creates a database/sql handle and optionally verifies connectivity.
func Open(ctx context.Context, options Options) (Database, error) {
	resolved, err := ResolveDSN(options.DSN)
	if err != nil {
		return Database{}, err
	}
	db, err := sql.Open(resolved.DriverName, resolved.DriverDSN)
	if err != nil {
		return Database{}, err
	}
	applyPoolDefaults(db, options)
	if !options.SkipPing {
		pingCtx := ctx
		if pingCtx == nil {
			pingCtx = context.Background()
		}
		var cancel context.CancelFunc
		if options.PingTimeout > 0 {
			pingCtx, cancel = context.WithTimeout(pingCtx, options.PingTimeout)
		} else {
			pingCtx, cancel = context.WithTimeout(pingCtx, 5*time.Second)
		}
		defer cancel()
		if err := db.PingContext(pingCtx); err != nil {
			_ = db.Close()
			return Database{}, err
		}
	}
	return Database{
		DB:         db,
		Dialect:    resolved.Dialect,
		DriverName: resolved.DriverName,
		DriverDSN:  resolved.DriverDSN,
		MaskedDSN:  resolved.MaskedDSN,
	}, nil
}

// ResolveDSN maps Python-compatible mysql/postgres URLs to Go driver settings.
func ResolveDSN(rawDSN string) (ResolvedDSN, error) {
	rawDSN = strings.TrimSpace(rawDSN)
	if rawDSN == "" {
		return ResolvedDSN{}, ErrMissingDSN
	}
	parsed, err := url.Parse(rawDSN)
	if err != nil {
		return ResolvedDSN{}, err
	}
	switch strings.ToLower(parsed.Scheme) {
	case "mysql", "mysql+pymysql":
		driverDSN, err := mysqlDriverDSN(parsed)
		if err != nil {
			return ResolvedDSN{}, err
		}
		return ResolvedDSN{
			Dialect:    DialectMySQL,
			DriverName: "mysql",
			DriverDSN:  driverDSN,
			MaskedDSN:  MaskDSN(rawDSN),
		}, nil
	case "postgres", "postgresql":
		return ResolvedDSN{
			Dialect:    DialectPostgres,
			DriverName: "pgx",
			DriverDSN:  rawDSN,
			MaskedDSN:  MaskDSN(rawDSN),
		}, nil
	default:
		return ResolvedDSN{}, fmt.Errorf("%w: %q", ErrUnsupportedDSN, parsed.Scheme)
	}
}

// PoolDefaults returns the Python-equivalent idle/open defaults for a role.
func PoolDefaults(runtimeRole string) (int, int) {
	role := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(runtimeRole), "-", "_"))
	switch role {
	case "outbox_worker", "send_dispatcher", "sdk_dispatcher":
		return 2, 40
	case "incoming_worker", "ingest":
		return 2, 30
	case "archive_sync_worker", "archive_media_worker", "archive_worker", "contact_sync_worker":
		return 1, 20
	case "maintenance_worker", "automation_worker":
		return 1, 10
	default:
		return 5, 30
	}
}

// MaskDSN removes userinfo while retaining scheme, host, port, and database.
func MaskDSN(rawDSN string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawDSN))
	if err != nil || parsed.Scheme == "" {
		return ""
	}
	host := parsed.Hostname()
	if parsed.Port() != "" {
		host = net.JoinHostPort(host, parsed.Port())
	}
	return fmt.Sprintf("%s://***@%s%s", parsed.Scheme, host, parsed.EscapedPath())
}

func mysqlDriverDSN(parsed *url.URL) (string, error) {
	cfg := mysql.NewConfig()
	cfg.User = parsed.User.Username()
	cfg.Passwd, _ = parsed.User.Password()
	cfg.Net = "tcp"
	host := parsed.Hostname()
	if host == "" {
		host = "127.0.0.1"
	}
	port := parsed.Port()
	if port == "" {
		port = "3306"
	}
	cfg.Addr = net.JoinHostPort(host, port)
	cfg.DBName = strings.TrimPrefix(parsed.Path, "/")
	cfg.ParseTime = true
	cfg.Loc = beijingLocation
	cfg.AllowNativePasswords = true
	cfg.CheckConnLiveness = true
	cfg.Params = map[string]string{
		"charset":   "utf8mb4",
		"time_zone": "'+08:00'",
	}
	for key, values := range parsed.Query() {
		if len(values) == 0 {
			continue
		}
		applyMySQLQueryParam(cfg, key, values[len(values)-1])
	}
	return cfg.FormatDSN(), nil
}

func applyMySQLQueryParam(cfg *mysql.Config, key string, value string) {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "parsetime":
		cfg.ParseTime = strings.EqualFold(value, "true") || value == "1"
	case "timeout":
		cfg.Timeout = parseDuration(value)
	case "readtimeout":
		cfg.ReadTimeout = parseDuration(value)
	case "writetimeout":
		cfg.WriteTimeout = parseDuration(value)
	case "loc":
		if location, err := time.LoadLocation(value); err == nil {
			cfg.Loc = location
		}
	default:
		cfg.Params[key] = value
	}
}

func applyPoolDefaults(db *sql.DB, options Options) {
	defaultIdle, defaultOpen := PoolDefaults(options.RuntimeRole)
	maxIdle := options.MaxIdleConns
	if maxIdle <= 0 {
		maxIdle = defaultIdle
	}
	maxOpen := options.MaxOpenConns
	if maxOpen <= 0 {
		maxOpen = defaultOpen
	}
	db.SetMaxIdleConns(maxIdle)
	db.SetMaxOpenConns(maxOpen)
	if options.ConnMaxIdleTime > 0 {
		db.SetConnMaxIdleTime(options.ConnMaxIdleTime)
	} else {
		db.SetConnMaxIdleTime(60 * time.Second)
	}
	if options.ConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(options.ConnMaxLifetime)
	}
}

func parseDuration(value string) time.Duration {
	duration, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil {
		return 0
	}
	return duration
}
