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

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/GALIAIS/NodeHarvest/internal/api"
	"github.com/GALIAIS/NodeHarvest/internal/auth"
	"github.com/GALIAIS/NodeHarvest/internal/config"
	"github.com/GALIAIS/NodeHarvest/internal/db"
	"github.com/GALIAIS/NodeHarvest/internal/hotcache"
	"github.com/GALIAIS/NodeHarvest/internal/metrics"
	"github.com/GALIAIS/NodeHarvest/internal/objectstore"
	"github.com/GALIAIS/NodeHarvest/internal/observability"
	"github.com/GALIAIS/NodeHarvest/internal/queue"
	"github.com/GALIAIS/NodeHarvest/internal/scheduler"
	"github.com/GALIAIS/NodeHarvest/internal/service"
	"github.com/GALIAIS/NodeHarvest/internal/store"
	"github.com/GALIAIS/NodeHarvest/internal/version"
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
	otelCtx, otelCancel := context.WithTimeout(context.Background(),
		time.Duration(cfg.Observability.ExportTimeoutSec)*time.Second)
	shutdownTracing, err := observability.Setup(otelCtx, cfg.Observability, "server")
	otelCancel()
	if err != nil {
		slog.Error("tracing setup", "err", err)
		os.Exit(1)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(),
			time.Duration(cfg.Observability.ExportTimeoutSec)*time.Second)
		defer cancel()
		if err := shutdownTracing(ctx); err != nil {
			slog.Error("tracing shutdown", "err", err)
		}
	}()
	st, err := store.New(dataDir)
	if err != nil {
		slog.Error("load node store", "err", err)
		os.Exit(1)
	}

	var sqlDB *db.Store
	if cfg.Database.Enabled {
		sqlDB, err = db.OpenDatabase(db.DatabaseOptions{
			Driver:       cfg.Database.Driver,
			DSN:          cfg.Database.DSN,
			MaxOpenConns: cfg.Database.MaxOpenConns,
			MaxIdleConns: cfg.Database.MaxIdleConns,
			JobDays:      cfg.Database.JobRetentionDays,
			AuditDays:    cfg.Database.AuditRetentionDays,
			MetricDays:   cfg.Database.MetricRetentionDays,
		})
		if err != nil {
			slog.Error("database open", "driver", cfg.Database.Driver, "err", err)
			os.Exit(1)
		}
		defer sqlDB.Close()
	}

	reg := metrics.New()
	var hot *hotcache.Client
	if cfg.Redis.Enabled {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		hot, err = hotcache.Open(ctx, cfg.Redis.URL, cfg.Redis.Prefix,
			time.Duration(cfg.Redis.CacheTTLSec)*time.Second,
			time.Duration(cfg.Redis.LockTTLSec)*time.Second)
		cancel()
		if err != nil {
			slog.Error("redis open", "err", err)
			os.Exit(1)
		}
		defer hot.Close()
	}
	artifactCtx, artifactCancel := context.WithTimeout(context.Background(), 10*time.Second)
	artifacts, err := objectstore.Open(artifactCtx, cfg.ObjectStore)
	artifactCancel()
	if err != nil {
		slog.Error("object store open", "err", err)
		os.Exit(1)
	}
	svc := service.NewWithOptions(cfg, st, service.Options{
		DB: sqlDB, Metrics: reg, Hot: hot, Artifacts: artifacts,
	})
	var taskQueue *queue.Queue
	if cfg.Queue.Enabled && cfg.Queue.EmbeddedWorkers > 0 {
		taskQueue, err = queue.New(sqlDB, func(ctx context.Context, task *db.QueuedTask) error {
			_, runErr := svc.RunQueuedTask(ctx, task)
			return runErr
		}, queue.Options{
			Workers:   cfg.Queue.EmbeddedWorkers,
			Lease:     time.Duration(cfg.Queue.LeaseSec) * time.Second,
			Poll:      time.Duration(cfg.Queue.PollMS) * time.Millisecond,
			RetryBase: time.Duration(cfg.Queue.RetryBaseSec) * time.Second,
		})
		if err != nil {
			slog.Error("queue start", "err", err)
			os.Exit(1)
		}
		taskQueue.Start()
		defer taskQueue.Stop()
	}
	sch := scheduler.New(cfg, svc)
	sch.Start()

	am, err := auth.NewManager(cfg.Auth, sqlDB, cfg.Publish.Token)
	if err != nil {
		slog.Error("authentication setup", "err", err)
		os.Exit(1)
	}
	apiServer := api.New(svc, sch, am)

	mux := http.NewServeMux()
	mux.Handle("/api/", apiServer.Handler())
	mux.Handle("/api", apiServer.Handler())
	mux.Handle("/sub/", apiServer.Handler())
	mux.Handle("/sub", apiServer.Handler())
	mux.Handle("/metrics", apiServer.Handler())
	if prefix := strings.TrimRight(cfg.Publish.PathPrefix, "/"); prefix != "" &&
		prefix != "/sub" && !strings.HasPrefix(prefix, "/sub/") &&
		prefix != "/api" && !strings.HasPrefix(prefix, "/api/") {
		mux.Handle(prefix+"/", apiServer.Handler())
		mux.Handle(prefix, apiServer.Handler())
	}
	if cfg.Server.EnablePprof {
		mux.Handle("/debug/pprof/", apiServer.Handler())
	}

	if info, err := os.Stat(webDir); err == nil && info.IsDir() {
		mux.Handle("/", spaHandler(webDir))
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
			_, _ = fmt.Fprintf(w, `<!doctype html><meta charset="utf-8"><title>NodeHarvest</title>
<body style="font-family:system-ui;background:#0b1220;color:#e2e8f0;padding:2rem;line-height:1.6">
<h1>NodeHarvest <small style="color:#64748b">%s</small></h1>
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
		Handler:           otelhttp.NewHandler(mux, "nodeharvest.http"),
		ReadHeaderTimeout: readHdr,
		ReadTimeout:       time.Duration(cfg.Server.ReadTimeoutSec) * time.Second,
		WriteTimeout:      time.Duration(cfg.Server.WriteTimeoutSec) * time.Second,
		IdleTimeout:       time.Duration(cfg.Server.IdleTimeoutSec) * time.Second,
		MaxHeaderBytes:    cfg.Server.MaxHeaderBytes,
	}

	go func() {
		slog.Info("NodeHarvest server listening",
			"addr", addr,
			"version", version.Version,
			"config", cfgPath,
			"nodes", st.Count(),
			"schedule", cfg.Schedule.Enabled,
			"publish", cfg.Publish.Enabled,
			"database", cfg.Database.Driver,
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
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Server.ShutdownTimeoutSec)*time.Second)
	defer cancel()
	sch.Stop()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("shutdown", "err", err)
	}
	if taskQueue != nil {
		taskQueue.Stop()
	}
	svc.CancelJob()
	if err := svc.WaitForJob(ctx); err != nil {
		slog.Error("wait for job", "err", err)
	}
	if err := svc.Flush(); err != nil {
		slog.Error("flush node store", "err", err)
	}
	slog.Info("bye")
}

func spaHandler(webDir string) http.Handler {
	root, _ := filepath.Abs(webDir)
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api") || strings.HasPrefix(r.URL.Path, "/sub") || r.URL.Path == "/metrics" {
			http.NotFound(w, r)
			return
		}
		p := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(r.URL.Path, "/")))
		if resolved, err := filepath.EvalSymlinks(p); err == nil && withinDir(root, resolved) {
			// #nosec G703 -- the canonical path is explicitly constrained to the canonical web root.
			if info, err := os.Stat(resolved); err == nil && !info.IsDir() {
				http.ServeFile(w, r, resolved)
				return
			}
		}
		index, err := filepath.EvalSymlinks(filepath.Join(root, "index.html"))
		if err != nil || !withinDir(root, index) {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, index)
	})
}

func withinDir(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
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
	// prefer JSON in production if NODE_HARVEST_LOG_JSON=1
	if os.Getenv("NODE_HARVEST_LOG_JSON") == "1" {
		slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lv})))
		return
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: lv})))
}
