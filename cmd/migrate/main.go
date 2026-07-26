package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/GALIAIS/NodeHarvest/internal/config"
	"github.com/GALIAIS/NodeHarvest/internal/db"
)

func main() {
	var cfgPath string
	flag.StringVar(&cfgPath, "config", "", "config.yaml path")
	flag.Parse()

	cfgPath = config.ResolveConfigPath(cfgPath)
	cfg, err := config.Load(cfgPath)
	if err != nil {
		exitf("load config: %v", err)
	}
	if !cfg.Database.Enabled {
		exitf("database is disabled")
	}
	store, err := db.OpenDatabase(db.DatabaseOptions{
		Driver:       cfg.Database.Driver,
		DSN:          cfg.Database.DSN,
		MaxOpenConns: cfg.Database.MaxOpenConns,
		MaxIdleConns: cfg.Database.MaxIdleConns,
		JobDays:      cfg.Database.JobRetentionDays,
		AuditDays:    cfg.Database.AuditRetentionDays,
		MetricDays:   cfg.Database.MetricRetentionDays,
	})
	if err != nil {
		exitf("migrate database: %v", err)
	}
	defer store.Close()
	if err := store.Ping(); err != nil {
		exitf("verify database: %v", err)
	}
	slog.Info("database migration complete", "driver", cfg.Database.Driver)
}

func exitf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
