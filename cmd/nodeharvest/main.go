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

	"github.com/GALIAIS/NodeHarvest/internal/config"
	"github.com/GALIAIS/NodeHarvest/internal/model"
	"github.com/GALIAIS/NodeHarvest/internal/pipeline"
)

func main() {
	var (
		cfgPath     string
		concurrency int
		maxLatency  int
		maxNodes    int
		skipTest    bool
		logLevel    string
	)
	flag.StringVar(&cfgPath, "config", "", "path to config.yaml")
	flag.IntVar(&concurrency, "c", 0, "test concurrency (override config)")
	flag.IntVar(&maxLatency, "max-latency", 0, "max latency ms (override config)")
	flag.IntVar(&maxNodes, "max-nodes", 0, "max exported nodes (override config)")
	flag.BoolVar(&skipTest, "skip-test", false, "only fetch/parse/clean/export, skip connectivity test")
	flag.StringVar(&logLevel, "log", "", "log level: debug|info|warn|error")
	flag.Parse()

	cfgPath = config.ResolveConfigPath(cfgPath)
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	if concurrency > 0 {
		cfg.App.Concurrency = concurrency
	}
	if maxLatency > 0 {
		cfg.Filter.MaxLatencyMS = maxLatency
	}
	if maxNodes > 0 {
		cfg.Filter.MaxNodes = maxNodes
	}
	if logLevel != "" {
		cfg.Logging.Level = logLevel
	}

	setupLogger(cfg.Logging.Level)
	slog.Info("NodeHarvest starting", "config", cfgPath, "sources", len(cfg.EnabledSources()))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var res *model.Result
	if skipTest {
		res, err = pipeline.RunFetchOnly(ctx, cfg)
	} else {
		res, err = pipeline.Run(ctx, cfg)
	}
	if err != nil {
		slog.Error("failed", "err", err)
		if res != nil {
			printSummary(res)
		}
		os.Exit(1)
	}
	printSummary(res)
}

func printSummary(res *model.Result) {
	fmt.Println()
	fmt.Println("========== NodeHarvest summary ==========")
	fmt.Printf("sources ok : %d\n", res.FetchedSources)
	fmt.Printf("parsed     : %d\n", res.ParsedCount)
	fmt.Printf("unique     : %d\n", res.UniqueCount)
	fmt.Printf("alive      : %d\n", res.AliveCount)
	fmt.Printf("exported   : %d\n", res.ExportedCount)
	fmt.Printf("duration   : %s\n", res.Duration)
	fmt.Println("=========================================")
	if res.ExportedCount > 0 {
		fmt.Println("output files are under the configured export dir (default: output/)")
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
	h := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: lv})
	slog.SetDefault(slog.New(h))
}
