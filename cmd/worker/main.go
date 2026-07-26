package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/GALIAIS/NodeHarvest/internal/config"
	"github.com/GALIAIS/NodeHarvest/internal/db"
	"github.com/GALIAIS/NodeHarvest/internal/hotcache"
	"github.com/GALIAIS/NodeHarvest/internal/metrics"
	"github.com/GALIAIS/NodeHarvest/internal/objectstore"
	"github.com/GALIAIS/NodeHarvest/internal/observability"
	"github.com/GALIAIS/NodeHarvest/internal/queue"
	"github.com/GALIAIS/NodeHarvest/internal/service"
	"github.com/GALIAIS/NodeHarvest/internal/store"
)

func main() {
	var cfgPath, workerID, logLevel string
	var workers int
	flag.StringVar(&cfgPath, "config", "", "config.yaml path")
	flag.StringVar(&workerID, "id", "", "worker identity")
	flag.IntVar(&workers, "workers", 1, "concurrent task workers")
	flag.StringVar(&logLevel, "log", "info", "log level")
	flag.Parse()
	setupLogger(logLevel)

	cfg, err := config.Load(config.ResolveConfigPath(cfgPath))
	if err != nil {
		fatal("load config", err)
	}
	otelCtx, otelCancel := context.WithTimeout(context.Background(),
		time.Duration(cfg.Observability.ExportTimeoutSec)*time.Second)
	shutdownTracing, err := observability.Setup(otelCtx, cfg.Observability, "worker")
	otelCancel()
	if err != nil {
		fatal("tracing setup", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(),
			time.Duration(cfg.Observability.ExportTimeoutSec)*time.Second)
		defer cancel()
		if err := shutdownTracing(ctx); err != nil {
			slog.Error("tracing shutdown", "err", err)
		}
	}()
	if !cfg.Database.Enabled {
		fatal("worker configuration", fmt.Errorf("database.enabled must be true"))
	}
	database, err := db.OpenDatabase(db.DatabaseOptions{
		Driver:       cfg.Database.Driver,
		DSN:          cfg.Database.DSN,
		MaxOpenConns: cfg.Database.MaxOpenConns,
		MaxIdleConns: cfg.Database.MaxIdleConns,
		JobDays:      cfg.Database.JobRetentionDays,
		AuditDays:    cfg.Database.AuditRetentionDays,
		MetricDays:   cfg.Database.MetricRetentionDays,
	})
	if err != nil {
		fatal("database open", err)
	}
	defer database.Close()

	var hot *hotcache.Client
	if cfg.Redis.Enabled {
		openCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		hot, err = hotcache.Open(openCtx, cfg.Redis.URL, cfg.Redis.Prefix,
			time.Duration(cfg.Redis.CacheTTLSec)*time.Second,
			time.Duration(cfg.Redis.LockTTLSec)*time.Second)
		cancel()
		if err != nil {
			fatal("redis open", err)
		}
		defer hot.Close()
	}
	artifactCtx, artifactCancel := context.WithTimeout(context.Background(), 10*time.Second)
	artifacts, err := objectstore.Open(artifactCtx, cfg.ObjectStore)
	artifactCancel()
	if err != nil {
		fatal("object store open", err)
	}
	cfg.Queue.Enabled = false
	cfg.Queue.EmbeddedWorkers = workers
	svc := service.NewWithOptions(cfg, store.NewMemory(), service.Options{
		DB: database, Metrics: metrics.New(), Hot: hot, Artifacts: artifacts,
	})
	manager, err := queue.New(database, func(ctx context.Context, task *db.QueuedTask) error {
		_, runErr := svc.RunQueuedTask(ctx, task)
		return runErr
	}, queue.Options{
		Workers:   workers,
		WorkerID:  workerID,
		Lease:     time.Duration(cfg.Queue.LeaseSec) * time.Second,
		Poll:      time.Duration(cfg.Queue.PollMS) * time.Millisecond,
		RetryBase: time.Duration(cfg.Queue.RetryBaseSec) * time.Second,
	})
	if err != nil {
		fatal("queue", err)
	}
	manager.Start()
	slog.Info("worker started", "workers", workers, "database", cfg.Database.Driver)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	<-ctx.Done()
	stop()
	manager.Stop()
	svc.CancelJob()
	waitCtx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Server.ShutdownTimeoutSec)*time.Second)
	defer cancel()
	if err := svc.WaitForJob(waitCtx); err != nil {
		slog.Error("wait for jobs", "err", err)
	}
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
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lv})))
}

func fatal(message string, err error) {
	slog.Error(message, "err", err)
	os.Exit(1)
}
