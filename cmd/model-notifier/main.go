package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hutoroff/model-notifier/internal/config"
	"github.com/hutoroff/model-notifier/internal/monitor"
	"github.com/hutoroff/model-notifier/internal/state"
)

func main() {
	cfgPath := flag.String("config", "config.yaml", "config file")
	once := flag.Bool("once", false, "check all sources once, print summary and exit")
	notifyOnSeed := flag.Bool("notify-on-seed", false, "notify about items found on a source's first check")
	healthcheck := flag.Bool("healthcheck", false, "probe the running server's /healthz and exit 0/1 (for Docker HEALTHCHECK)")
	flag.Parse()
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	if *healthcheck {
		os.Exit(probe())
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fatal("config", err)
	}
	st, err := state.Load(cfg.StateFile)
	if err != nil {
		fatal("state", err)
	}
	m, err := monitor.New(cfg, st, *notifyOnSeed)
	if err != nil {
		fatal("monitor", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if *once {
		sum, ok := m.RunAll(ctx)
		fmt.Print(sum)
		if !ok {
			os.Exit(1)
		}
		return
	}

	go m.Run(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, "ok") })
	mux.HandleFunc("POST /run", func(w http.ResponseWriter, r *http.Request) {
		sum, ok := m.RunAll(r.Context())
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
		}
		fmt.Fprint(w, sum)
	})
	srv := &http.Server{Addr: cfg.Listen, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(sctx)
	}()
	slog.Info("listening", "addr", cfg.Listen)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		fatal("http", err)
	}
}

// probe GETs $HEALTHCHECK_URL (default http://127.0.0.1:8080/healthz). It deliberately
// doesn't read the config so it works without secrets; set HEALTHCHECK_URL if listen changes.
func probe() int {
	u := os.Getenv("HEALTHCHECK_URL")
	if u == "" {
		u = "http://127.0.0.1:8080/healthz"
	}
	c := &http.Client{Timeout: 5 * time.Second}
	resp, err := c.Get(u)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, resp.Status)
		return 1
	}
	return 0
}

func fatal(what string, err error) {
	slog.Error(what, "err", err)
	os.Exit(1)
}
