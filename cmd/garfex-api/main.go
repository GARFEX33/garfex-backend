package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/GARFEX33/garfex-api/internal/httpapi"
	garfex "github.com/GARFEX33/garfex-costos-unitarios"
)

const (
	defaultListenAddr = "127.0.0.1:8080"
	lifecycleTimeout  = 10 * time.Second
)

type config struct {
	dsn        string
	listenAddr string
}

type application interface {
	Close()
	ResourceReader() httpapi.CatalogReader
	SupplierWriter() httpapi.SupplierWriter
	ResourceWriter() httpapi.ResourceWriter
}

type coreApplication struct{ *garfex.Application }

func (a coreApplication) ResourceReader() httpapi.CatalogReader {
	return a.Application.ResourceReader
}

func (a coreApplication) SupplierWriter() httpapi.SupplierWriter {
	return a.Application.SupplierWriter
}

func (a coreApplication) ResourceWriter() httpapi.ResourceWriter {
	return a.Application.ResourceWriter
}

type server interface {
	Serve(net.Listener) error
	Shutdown(context.Context) error
	Close() error
}

type dependencies struct {
	open      func(context.Context, garfex.Config) (application, error)
	listen    func(string, string) (net.Listener, error)
	newServer func(string, http.Handler) server
}

func main() {
	os.Exit(runMain(os.Getenv, productionDependencies()))
}

func runMain(getenv func(string) string, deps dependencies) int {
	config, err := loadConfig(getenv)
	if err == nil {
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		err = run(ctx, config, deps)
	}
	if err != nil {
		log.Print("garfex-api: startup failed")
		return 1
	}
	return 0
}

func loadConfig(getenv func(string) string) (config, error) {
	dsn := getenv("GARFEX_API_DSN")
	if strings.TrimSpace(dsn) == "" {
		return config{}, errors.New("invalid API configuration")
	}
	addr := getenv("GARFEX_API_LISTEN_ADDR")
	if addr == "" {
		addr = defaultListenAddr
	}
	if !validLocalAddress(addr) {
		return config{}, errors.New("invalid API configuration")
	}
	return config{dsn: dsn, listenAddr: addr}, nil
}

func validLocalAddress(addr string) bool {
	host, port, err := net.SplitHostPort(addr)
	if err != nil || (host != "127.0.0.1" && host != "::1") || port == "" {
		return false
	}
	for _, char := range port {
		if char < '0' || char > '9' {
			return false
		}
	}
	number, err := strconv.Atoi(port)
	return err == nil && number > 0 && number <= 65535
}

func run(parent context.Context, config config, deps dependencies) error {
	openCtx, cancelOpen := context.WithTimeout(parent, lifecycleTimeout)
	app, err := deps.open(openCtx, garfex.Config{DSN: config.dsn})
	cancelOpen()
	if err != nil {
		return errors.New("API startup failed")
	}
	defer app.Close()

	listener, err := deps.listen("tcp", config.listenAddr)
	if err != nil {
		return errors.New("API startup failed")
	}
	server := deps.newServer(config.listenAddr, httpapiRouter(app.ResourceReader(), app.SupplierWriter(), app.ResourceWriter()))
	served := make(chan error, 1)
	go func() { served <- server.Serve(listener) }()

	select {
	case serveErr := <-served:
		shutdownErr := drainServer(server)
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			return errors.New("API server failed")
		}
		if shutdownErr != nil {
			return errors.New("API server shutdown failed")
		}
		return nil
	case <-parent.Done():
		if err := drainServer(server); err != nil {
			return errors.New("API server shutdown failed")
		}
		return nil
	}
}

func drainServer(server server) error {
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), lifecycleTimeout)
	defer cancelShutdown()
	if err := server.Shutdown(shutdownCtx); err != nil {
		_ = server.Close()
		return err
	}
	return nil
}

func productionDependencies() dependencies {
	return dependencies{
		open: func(ctx context.Context, config garfex.Config) (application, error) {
			app, err := garfex.Open(ctx, config)
			if err != nil {
				return nil, err
			}
			return coreApplication{Application: app}, nil
		},
		listen: net.Listen,
		newServer: func(addr string, handler http.Handler) server {
			return newHTTPServer(addr, handler)
		},
	}
}

func httpapiRouter(reader httpapi.CatalogReader, supplierWriter httpapi.SupplierWriter, resourceWriter httpapi.ResourceWriter) http.Handler {
	return httpapi.NewRouter(reader, supplierWriter, resourceWriter)
}

func newHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}
