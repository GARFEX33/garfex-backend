package garfex

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestOpenRejectsInvalidDSN(t *testing.T) {
	tests := []struct{ name, dsn string }{
		{name: "empty", dsn: ""},
		{name: "whitespace", dsn: " \t\n"},
		{name: "malformed", dsn: "://invalid"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			app, err := Open(context.Background(), Config{DSN: test.dsn})
			if app != nil {
				t.Fatal("Open() application = non-nil, want nil")
			}
			if err == nil {
				t.Fatal("Open() error = nil, want error")
			}
			if got := err.Error(); got != "garfex: invalid database configuration" {
				t.Fatalf("Open() error = %q, want safe configuration error", got)
			}
		})
	}
}

func TestOpenPreservesCanceledContextWithoutDSNLeakage(t *testing.T) {
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	deadlineCtx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	cancel()

	tests := []struct {
		name  string
		ctx   context.Context
		cause error
	}{
		{name: "already canceled", ctx: canceledCtx, cause: context.Canceled},
		{name: "expired deadline", ctx: deadlineCtx, cause: context.DeadlineExceeded},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			app, err := Open(test.ctx, Config{DSN: "postgres://user:secret-password@example.invalid/database"})
			if app != nil {
				t.Fatal("Open() application = non-nil, want nil")
			}
			if !errors.Is(err, test.cause) {
				t.Fatalf("Open() error = %v, want errors.Is(_, %v)", err, test.cause)
			}
			if strings.Contains(err.Error(), "secret-password") {
				t.Fatalf("Open() error leaked DSN password: %q", err)
			}
		})
	}
}

func TestApplicationCloseIsIdempotent(t *testing.T) {
	var nilApp *Application
	nilApp.Close()
	nilApp.Close()

	zeroApp := &Application{}
	zeroApp.Close()
	zeroApp.Close()

	config, err := pgxpool.ParseConfig("postgres://user:password@example.invalid/database")
	if err != nil {
		t.Fatalf("parse pool configuration: %v", err)
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	app := &Application{pool: pool}
	app.Close()
	app.Close()
}
