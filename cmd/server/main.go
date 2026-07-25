package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/local/node-hunter/internal/api"
	"github.com/local/node-hunter/internal/auth"
	"github.com/local/node-hunter/internal/config"
	"github.com/local/node-hunter/internal/db"
	"github.com/local/node-hunter/internal/metrics"
	"github.com/local/node-hunter/internal/scheduler"
	"github.com/local/node-hunter/internal/service"
	"github.com/local/node-hunter/internal/store"
	"github.com/local/node-hunter/internal/version"
)

func main() {
	var (
		cfgPath string
		addr    string
		dataDir string
		webDir  string
		logLvl  string
	)
	flag.StringVar(&cfgPath, "config", "", "config.yaml path")
	flag.StringVar(&addr, "addr", ":8080", "listen address")
	flag.StringVar(&dataDir, "data", "data", "data directory")
	flag.StringVar(&webDir, "web", "web/dist", "static frontend directory")
	flag.StringVar(&logLvl, "log", "info", "log level")
	flag.Parse()

	setupLogger(logLvl)

	cfgPath = config.ResolveConfigPath(cfgPath)
	cfg, err := config.Load(cfgPath)
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}
	if err := validateConfig(cfg); err != nil {
		slog.Error("invalid config", "err", err)
		os.Exit(1)
	}

	st := store.New(dataDir)

	var sqlDB *db.Store
	if cfg.SQLite.Enabled {
		sqlDB, err = db.Open(cfg.SQLite.Path)
		if err != nil {
			slog.Error("sqlite open", "err", err)
			os.Exit(1)
		}
		defer sqlDB.Close()
	}

	reg := metrics.New()
	svc := service.NewWithOptions(cfg, st, service.Options{DB: sqlDB, Metrics: reg})
	sch := scheduler.New(cfg, svc)
	sch.Start()

	am := &auth.Manager{
		DB:                sqlDB,
		MasterToken:       cfg.Publish.Token,
		AdminToken:        cfg.Security.AdminToken,
		QueryTokenAllowed: cfg.Security.AllowQueryToken,
	}
	apiServer := api.New(svc, sch, am)

	mux := http.NewServeMux()
	mux.Handle("/api/", apiServer.Handler())
	mux.Handle("/api", apiServer.Handler())
	mux.Handle("/sub/", apiServer.Handler())
	mux.Handle("/sub", apiServer.Handler())
	mux.Handle("/metrics", apiServer.Handler())

	if info, err := os.Stat(webDir); err == nil && info.IsDir() {
		fs := http.FileServer(http.Dir(webDir))
		mux.Handle("/", spaHandler(webDir, fs))
		slog.Info("serving frontend", "dir", webDir)
	} else {
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			pub := cfg.Publish.PathPrefix
			if pub == "" {
				pub = "/sub"
			}
			_, _ = fmt.Fprintf(w, `<!doctype html><meta charset="utf-8"><title>node-hunter</title>
<body style="font-family:system-ui;background:#0b1220;color:#e2e8f0;padding:2rem;line-height:1.6">
<h1>node-hunter <small style="color:#64748b">%s</small></h1>
<ul>
<li><a href="/api/health" style="color:#22d3ee">/api/health</a></li>
<li><a href="/api/ready" style="color:#22d3ee">/api/ready</a></li>
<li><a href="/api/version" style="color:#22d3ee">/api/version</a></li>
<li><a href="/metrics" style="color:#a3e635">/metrics</a></li>
<li><a href="%s" style="color:#fbbf24">%s</a> subscription</li>
<li><a href="/api/schedule" style="color:#a78bfa">/api/schedule</a></li>
</ul>
</body>`, version.Version, pub, pub)
		})
	}

	readHdr := time.Duration(cfg.Server.ReadHeaderTimeoutSec) * time.Second
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: readHdr,
	}

	go func() {
		slog.Info("node-hunter server listening",
			"addr", addr,
			"version", version.Version,
			"config", cfgPath,
			"nodes", st.Count(),
			"schedule", cfg.Schedule.Enabled,
			"publish", cfg.Publish.Enabled,
			"sqlite", cfg.SQLite.Enabled,
			"sub", cfg.Publish.PathPrefix,
		)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server failed", "err", err)
			os.Exit(1)
		}
	}()

	// graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	slog.Info("shutdown signal", "sig", sig.String())
	sch.Stop()
	svc.CancelJob()
	svc.Flush()

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Server.ShutdownTimeoutSec)*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("shutdown", "err", err)
	}
	slog.Info("bye")
}

func validateConfig(cfg *config.Config) error {
	if cfg.App.Concurrency <= 0 {
		return fmt.Errorf("app.concurrency must be > 0")
	}
	if cfg.Filter.MaxNodes < 0 {
		return fmt.Errorf("filter.max_nodes invalid")
	}
	if cfg.Publish.Enabled && cfg.Publish.PathPrefix == "" {
		return fmt.Errorf("publish.path_prefix empty")
	}
	return nil
}

func spaHandler(webDir string, fs http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api") || strings.HasPrefix(r.URL.Path, "/sub") || r.URL.Path == "/metrics" {
			http.NotFound(w, r)
			return
		}
		p := filepath.Join(webDir, filepath.Clean("/"+r.URL.Path))
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			fs.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(webDir, "index.html"))
	})
}

func setupLogger(level string) {
	var lv slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lv = slog.LevelDebug
	case "warn":
		lv = slog.LevelWarn
	case "error":
		lv = slog.LevelError
	default:
		lv = slog.LevelInfo
	}
	// prefer JSON in production if NODE_HUNTER_LOG_JSON=1
	if os.Getenv("NODE_HUNTER_LOG_JSON") == "1" {
		slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lv})))
		return
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: lv})))
}
