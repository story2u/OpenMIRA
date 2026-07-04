// Package sqldb tests DSN translation without connecting to a real database.
// The cases mirror Python's CLOUD_DB_DSN compatibility surface for MySQL and
// historical PostgreSQL deployments.
package sqldb

import (
	"errors"
	"strings"
	"testing"
)

// TestResolveDSNTranslatesMySQLURL verifies Python mysql:// DSN compatibility.
func TestResolveDSNTranslatesMySQLURL(t *testing.T) {
	resolved, err := ResolveDSN("mysql://user:pass@db.example:3307/wework?timeout=2s")
	if err != nil {
		t.Fatalf("ResolveDSN returned error: %v", err)
	}
	if resolved.Dialect != DialectMySQL || resolved.DriverName != "mysql" {
		t.Fatalf("unexpected resolved metadata: %+v", resolved)
	}
	for _, want := range []string{
		"user:pass@tcp(db.example:3307)/wework",
		"charset=utf8mb4",
		"parseTime=true",
		"time_zone=%27%2B08%3A00%27",
		"timeout=2s",
	} {
		if !strings.Contains(resolved.DriverDSN, want) {
			t.Fatalf("driver DSN missing %q: %s", want, resolved.DriverDSN)
		}
	}
	if resolved.MaskedDSN != "mysql://***@db.example:3307/wework" {
		t.Fatalf("masked DSN = %q", resolved.MaskedDSN)
	}
}

// TestResolveDSNAcceptsMySQLPyMySQLScheme preserves Alembic-era DSNs.
func TestResolveDSNAcceptsMySQLPyMySQLScheme(t *testing.T) {
	resolved, err := ResolveDSN("mysql+pymysql://user:pass@db.example/wework")
	if err != nil {
		t.Fatalf("ResolveDSN returned error: %v", err)
	}
	if resolved.Dialect != DialectMySQL || !strings.Contains(resolved.DriverDSN, "tcp(db.example:3306)") {
		t.Fatalf("unexpected resolved DSN: %+v", resolved)
	}
}

// TestResolveDSNPreservesPostgresURL verifies the historical compatibility path.
func TestResolveDSNPreservesPostgresURL(t *testing.T) {
	dsn := "postgresql://user:pass@pg.example:5432/wework"
	resolved, err := ResolveDSN(dsn)
	if err != nil {
		t.Fatalf("ResolveDSN returned error: %v", err)
	}
	if resolved.Dialect != DialectPostgres || resolved.DriverName != "pgx" || resolved.DriverDSN != dsn {
		t.Fatalf("unexpected resolved DSN: %+v", resolved)
	}
}

// TestResolveDSNRejectsMissingOrUnsupportedInput keeps startup failures explicit.
func TestResolveDSNRejectsMissingOrUnsupportedInput(t *testing.T) {
	if _, err := ResolveDSN(""); !errors.Is(err, ErrMissingDSN) {
		t.Fatalf("empty DSN error = %v, want %v", err, ErrMissingDSN)
	}
	if _, err := ResolveDSN("sqlite:///tmp/app.db"); !errors.Is(err, ErrUnsupportedDSN) {
		t.Fatalf("sqlite DSN error = %v, want %v", err, ErrUnsupportedDSN)
	}
}

// TestPoolDefaultsMatchPythonRoles protects high-concurrency role boundaries.
func TestPoolDefaultsMatchPythonRoles(t *testing.T) {
	cases := []struct {
		role     string
		wantIdle int
		wantOpen int
	}{
		{role: "api", wantIdle: 5, wantOpen: 30},
		{role: "send-dispatcher", wantIdle: 2, wantOpen: 40},
		{role: "contact-sync-worker", wantIdle: 1, wantOpen: 20},
		{role: "maintenance-worker", wantIdle: 1, wantOpen: 10},
	}
	for _, testCase := range cases {
		t.Run(testCase.role, func(t *testing.T) {
			gotIdle, gotOpen := PoolDefaults(testCase.role)
			if gotIdle != testCase.wantIdle || gotOpen != testCase.wantOpen {
				t.Fatalf("PoolDefaults(%q) = (%d,%d), want (%d,%d)", testCase.role, gotIdle, gotOpen, testCase.wantIdle, testCase.wantOpen)
			}
		})
	}
}
