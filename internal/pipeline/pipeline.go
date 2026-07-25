package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/local/node-hunter/internal/cleaner"
	"github.com/local/node-hunter/internal/config"
	"github.com/local/node-hunter/internal/exporter"
	"github.com/local/node-hunter/internal/fetcher"
	"github.com/local/node-hunter/internal/filter"
	"github.com/local/node-hunter/internal/model"
	"github.com/local/node-hunter/internal/parser"
	"github.com/local/node-hunter/internal/tester"
)

// Run 执行完整流水线：拉取 → 解析 → 清理 → 测活 → 筛选 → 导出
func Run(ctx context.Context, cfg *config.Config) (*model.Result, error) {
	start := time.Now()
	sources := cfg.EnabledSources()
	if len(sources) == 0 {
		return nil, fmt.Errorf("no enabled sources in config")
	}

	slog.Info("start fetch", "sources", len(sources))
	f := fetcher.New(cfg.FetchTimeout(), cfg.App.UserAgent)
	docs := f.FetchAll(ctx, sources, min(16, len(sources)))

	okSources := 0
	var all []*model.Node
	rawLines := 0
	for _, d := range docs {
		if d.Err != nil {
			slog.Warn("fetch failed", "source", d.Source.Name, "err", d.Err)
			continue
		}
		okSources++
		nodes := parser.ParseContent(d.Body, d.Source.Name)
		rawLines += len(nodes)
		all = append(all, nodes...)
		slog.Info("fetched",
			"source", d.Source.Name,
			"bytes", d.Bytes,
			"nodes", len(nodes),
			"latency", d.Latency.Round(time.Millisecond).String(),
		)
	}

	parsed := len(all)
	slog.Info("parse done", "raw_nodes", parsed)

	unique := cleaner.Clean(all, cfg)
	slog.Info("clean done", "unique", len(unique))

	if len(unique) == 0 {
		return &model.Result{
			FetchedSources: okSources,
			RawCount:       rawLines,
			ParsedCount:    parsed,
			UniqueCount:    0,
			Duration:       time.Since(start).Round(time.Millisecond).String(),
		}, fmt.Errorf("no valid nodes after clean")
	}

	slog.Info("start test", "nodes", len(unique), "concurrency", cfg.App.Concurrency)
	t := tester.New(tester.Options{
		Concurrency:   cfg.App.Concurrency,
		Timeout:       cfg.TestTimeout(),
		PreferTLSDial: false,
		OnProgress: func(done, total int) {
			if done == total || done%max(1, total/10) == 0 {
				slog.Info("testing", "progress", fmt.Sprintf("%d/%d", done, total))
			}
		},
	})
	t.TestAll(ctx, unique)
	slog.Info("test done", "summary", tester.Summary(unique))

	final := filter.Apply(unique, cfg)
	stats := filter.StatsByProtocol(final)
	slog.Info("filter done", "kept", len(final), "by_protocol", stats)

	paths, err := exporter.Export(final, cfg)
	if err != nil {
		return nil, err
	}
	for _, p := range paths {
		slog.Info("exported", "file", p)
	}

	alive := 0
	for _, n := range unique {
		if n.Alive {
			alive++
		}
	}

	return &model.Result{
		FetchedSources: okSources,
		RawCount:       rawLines,
		ParsedCount:    parsed,
		UniqueCount:    len(unique),
		AliveCount:     alive,
		ExportedCount:  len(final),
		Nodes:          final,
		Duration:       time.Since(start).Round(time.Millisecond).String(),
	}, nil
}

// RunFetchOnly 仅拉取/解析/清理/导出，不做连通性测试
func RunFetchOnly(ctx context.Context, cfg *config.Config) (*model.Result, error) {
	start := time.Now()
	sources := cfg.EnabledSources()
	if len(sources) == 0 {
		return nil, fmt.Errorf("no enabled sources in config")
	}

	slog.Info("start fetch (skip-test)", "sources", len(sources))
	f := fetcher.New(cfg.FetchTimeout(), cfg.App.UserAgent)
	docs := f.FetchAll(ctx, sources, min(16, len(sources)))

	okSources := 0
	var all []*model.Node
	for _, d := range docs {
		if d.Err != nil {
			slog.Warn("fetch failed", "source", d.Source.Name, "err", d.Err)
			continue
		}
		okSources++
		nodes := parser.ParseContent(d.Body, d.Source.Name)
		all = append(all, nodes...)
		slog.Info("fetched", "source", d.Source.Name, "nodes", len(nodes))
	}

	parsed := len(all)
	unique := cleaner.Clean(all, cfg)
	slog.Info("clean done", "unique", len(unique))

	// 跳过测活时：标记为未测试，导出全部唯一节点（仍受 max_nodes 限制）
	cfgCopy := *cfg
	cfgCopy.Filter.MinSuccess = false
	final := filter.Apply(unique, &cfgCopy)

	paths, err := exporter.Export(final, cfg)
	if err != nil {
		return nil, err
	}
	for _, p := range paths {
		slog.Info("exported", "file", p)
	}

	return &model.Result{
		FetchedSources: okSources,
		RawCount:       parsed,
		ParsedCount:    parsed,
		UniqueCount:    len(unique),
		AliveCount:     0,
		ExportedCount:  len(final),
		Nodes:          final,
		Duration:       time.Since(start).Round(time.Millisecond).String(),
	}, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
