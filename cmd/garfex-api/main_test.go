package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GARFEX33/garfex-api/internal/httpapi"
	garfex "github.com/GARFEX33/garfex-costos-unitarios"
)

func TestLoadConfig(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  map[string]string
		want config
		ok   bool
	}{
		{
			name: "default loopback",
			env:  map[string]string{"GARFEX_API_DSN": "postgres://app:secret@localhost/db"},
			want: config{"postgres://app:secret@localhost/db", "127.0.0.1:8080"},
			ok:   true,
		},
		{
			name: "IPv6 loopback",
			env:  map[string]string{"GARFEX_API_DSN": "dsn", "GARFEX_API_LISTEN_ADDR": "[::1]:9090"},
			want: config{"dsn", "[::1]:9090"},
			ok:   true,
		},
		{name: "blank DSN", env: map[string]string{"GARFEX_API_DSN": " \t "}},
		{name: "wildcard", env: map[string]string{"GARFEX_API_DSN": "dsn", "GARFEX_API_LISTEN_ADDR": ":8080"}},
		{name: "hostname", env: map[string]string{"GARFEX_API_DSN": "dsn", "GARFEX_API_LISTEN_ADDR": "localhost:8080"}},
		{name: "other IP", env: map[string]string{"GARFEX_API_DSN": "dsn", "GARFEX_API_LISTEN_ADDR": "127.0.0.2:8080"}},
		{name: "zero port", env: map[string]string{"GARFEX_API_DSN": "dsn", "GARFEX_API_LISTEN_ADDR": "127.0.0.1:0"}},
		{name: "non-numeric port", env: map[string]string{"GARFEX_API_DSN": "dsn", "GARFEX_API_LISTEN_ADDR": "127.0.0.1:http"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := loadConfig(func(key string) string { return tc.env[key] })
			if (err == nil) != tc.ok {
				t.Fatalf("error = %v, want success=%v", err, tc.ok)
			}
			if err == nil && got != tc.want {
				t.Fatalf("config = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestNewHTTPServerUsesRouterAndTimeouts(t *testing.T) {
	s := newHTTPServer("127.0.0.1:8080", httpapiRouter(nil, nil, nil, nil, nil))
	if s.Addr != "127.0.0.1:8080" || s.ReadHeaderTimeout == 0 || s.ReadTimeout == 0 || s.WriteTimeout == 0 || s.IdleTimeout == 0 {
		t.Fatalf("server not configured: %#v", s)
	}
	r := httptest.NewRecorder()
	s.Handler.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if r.Code != http.StatusOK {
		t.Fatalf("health status = %d", r.Code)
	}
}

func TestRunPassesExactDSNWithDeadlineAndDrainsBeforeCoreClose(t *testing.T) {
	events := []string{}
	app := &fakeApp{events: &events}
	var got garfex.Config
	err := run(context.Background(), config{"postgres://u:secret@db/app", "127.0.0.1:8080"}, dependencies{
		open: func(ctx context.Context, c garfex.Config) (application, error) {
			got = c
			deadline, ok := ctx.Deadline()
			if !ok || time.Until(deadline) > 10*time.Second || time.Until(deadline) < 9*time.Second {
				t.Fatal("missing ten-second deadline")
			}
			return app, nil
		},
		listen: func(string, string) (net.Listener, error) { return nil, nil },
		newServer: func(string, http.Handler) server {
			return &fakeServer{serveErr: errors.New("postgres://u:secret@db/app"), events: &events}
		},
	})
	if got.DSN != "postgres://u:secret@db/app" || app.closes != 1 || strings.Join(events, ",") != "shutdown,app" {
		t.Fatalf("DSN = %q, closes = %d, events = %v", got.DSN, app.closes, events)
	}
	assertSafeError(t, err)
}

func TestRunShutdownTimeoutClosesServerBeforeApplication(t *testing.T) {
	events := []string{}
	app := &fakeApp{events: &events}
	serveDone := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := run(ctx, config{"dsn", "127.0.0.1:8080"}, dependencies{
		open:   func(context.Context, garfex.Config) (application, error) { return app, nil },
		listen: func(string, string) (net.Listener, error) { return nil, nil },
		newServer: func(string, http.Handler) server {
			return &fakeServer{
				serveDone:   serveDone,
				shutdownErr: context.DeadlineExceeded,
				events:      &events,
				onShutdown: func(ctx context.Context) {
					deadline, ok := ctx.Deadline()
					if !ok || time.Until(deadline) > 10*time.Second || time.Until(deadline) < 9*time.Second {
						t.Fatal("missing ten-second shutdown deadline")
					}
				},
			}
		},
	})
	if strings.Join(events, ",") != "shutdown,close,app" || app.closes != 1 {
		t.Fatalf("cleanup order = %v, closes = %d", events, app.closes)
	}
	assertSafeError(t, err)
}

func TestRunMainReturnsFailureStatusAfterCleanup(t *testing.T) {
	events := []string{}
	status := runMain(func(key string) string {
		if key == "GARFEX_API_DSN" {
			return "dsn"
		}
		return ""
	}, dependencies{
		open:   func(context.Context, garfex.Config) (application, error) { return &fakeApp{events: &events}, nil },
		listen: func(string, string) (net.Listener, error) { return nil, nil },
		newServer: func(string, http.Handler) server {
			return &fakeServer{serveErr: errors.New("password=secret"), events: &events}
		},
	})
	if status != 1 || strings.Join(events, ",") != "shutdown,app" {
		t.Fatalf("status = %d, cleanup = %v", status, events)
	}
}

func TestRunSanitizesOpenFailure(t *testing.T) {
	err := run(context.Background(), config{"dsn", "127.0.0.1:8080"}, dependencies{
		open: func(context.Context, garfex.Config) (application, error) { return nil, errors.New("password=secret") },
	})
	assertSafeError(t, err)
}

func assertSafeError(t *testing.T, err error) {
	t.Helper()
	if err == nil || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "postgres") {
		t.Fatalf("unsafe error: %v", err)
	}
}

type fakeApp struct {
	closes int
	events *[]string
}

func (a *fakeApp) ResourceReader() httpapi.CatalogReader  { return nil }
func (a *fakeApp) SupplierWriter() httpapi.SupplierWriter { return nil }
func (a *fakeApp) ResourceWriter() httpapi.ResourceWriter { return nil }
func (a *fakeApp) SupplierReader() httpapi.SupplierReader { return nil }
func (a *fakeApp) Resources() httpapi.ResourceReader      { return nil }

func (a *fakeApp) Close() {
	a.closes++
	if a.events != nil {
		*a.events = append(*a.events, "app")
	}
}

type fakeServer struct {
	serveErr, shutdownErr error
	serveDone             chan struct{}
	events                *[]string
	onShutdown            func(context.Context)
}

func (s *fakeServer) Serve(net.Listener) error {
	if s.serveDone != nil {
		<-s.serveDone
	}
	return s.serveErr
}
func (s *fakeServer) Shutdown(ctx context.Context) error {
	*s.events = append(*s.events, "shutdown")
	if s.onShutdown != nil {
		s.onShutdown(ctx)
	}
	if s.serveDone != nil {
		close(s.serveDone)
	}
	return s.shutdownErr
}
func (s *fakeServer) Close() error {
	*s.events = append(*s.events, "close")
	return nil
}
