package db

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/GALIAIS/NodeHarvest/internal/model"
)

func TestSourceStateAndJobCursor(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if err := s.RecordSourceFetch(SourceState{
		Name: "source", URL: "https://example.test/sub", Priority: 90, LatencyMS: 12,
	}, true); err != nil {
		t.Fatal(err)
	}
	states, err := s.ListSourceStates()
	if err != nil || len(states) != 1 || states[0].SuccessCount != 1 {
		t.Fatalf("states=%+v err=%v", states, err)
	}

	base := time.Now().Add(-time.Hour)
	for i, id := range []string{"a", "b", "c"} {
		at := base.Add(time.Duration(i) * time.Minute)
		if err := s.SaveJob(&model.Job{
			ID: id, Type: "fetch", Status: model.JobCompleted, CreatedAt: at, UpdatedAt: at,
		}); err != nil {
			t.Fatal(err)
		}
	}
	first, err := s.ListJobsPage(2, "")
	if err != nil || len(first) != 2 {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	second, err := s.ListJobsPage(2, first[1].ID)
	if err != nil || len(second) != 1 {
		t.Fatalf("second=%+v err=%v", second, err)
	}
}

func TestDurableNodesMetricsAndQueue(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "enterprise.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	node := &model.Node{
		ID: "node-1", Fingerprint: "fingerprint", Protocol: model.ProtoVLESS,
		Server: "203.0.113.1", Port: 443, Alive: true, Score: 90,
		FirstSeenAt: time.Now().Add(-time.Hour), LastSeenAt: time.Now(), TestedAt: time.Now(),
	}
	if err := s.ReplaceNodes([]*model.Node{node}); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.LoadNodes(10)
	if err != nil || len(loaded) != 1 || loaded[0].ID != node.ID {
		t.Fatalf("loaded=%+v err=%v", loaded, err)
	}
	for _, latency := range []time.Duration{10, 20, 100, 200} {
		node.Latency = latency * time.Millisecond
		node.TestedAt = time.Now()
		if err := s.RecordNodeMetrics([]*model.Node{node}); err != nil {
			t.Fatal(err)
		}
	}
	daily, err := s.DailyNodeMetrics(node.ID, 1)
	if err != nil || len(daily) != 1 || daily[0].P50LatencyMS != 20 || daily[0].P95LatencyMS != 200 {
		t.Fatalf("daily=%+v err=%v", daily, err)
	}

	low := &QueuedTask{ID: "low", Type: "fetch", Priority: 1, MaxAttempts: 2}
	high := &QueuedTask{ID: "high", Type: "quality", Priority: 100, MaxAttempts: 2}
	if err := s.EnqueueTask(low, 2); err != nil {
		t.Fatal(err)
	}
	if err := s.EnqueueTask(high, 2); err != nil {
		t.Fatal(err)
	}
	if err := s.EnqueueTask(&QueuedTask{ID: "overflow", Type: "ai"}, 2); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("queue full error=%v", err)
	}
	task, err := s.LeaseTask(context.Background(), "worker-a", time.Minute)
	if err != nil || task.ID != "high" {
		t.Fatalf("leased=%+v err=%v", task, err)
	}
	if err := s.CompleteTask(task.ID, "worker-a"); err != nil {
		t.Fatal(err)
	}
	task, err = s.LeaseTask(context.Background(), "worker-a", 10*time.Millisecond)
	if err != nil || task.ID != "low" {
		t.Fatalf("leased=%+v err=%v", task, err)
	}
	time.Sleep(20 * time.Millisecond)
	task, err = s.LeaseTask(context.Background(), "worker-b", time.Minute)
	if err != nil || task.ID != "low" || task.Attempts != 2 {
		t.Fatalf("re-leased=%+v err=%v", task, err)
	}
}

func TestTokenUsageAndAuditRange(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "management.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	token := &Token{
		ID: "token-1", Name: "production", TokenHash: "hash", TokenPrefix: "prefix",
		Enabled: true, TenantID: "acme", DailyQuota: 10, CreatedAt: time.Now().Format(time.RFC3339),
	}
	if err := s.InsertToken(token); err != nil {
		t.Fatal(err)
	}
	if _, allowed, err := s.ConsumeTokenQuota(token.ID, token.DailyQuota); err != nil || !allowed {
		t.Fatalf("consume allowed=%t err=%v", allowed, err)
	}
	if err := s.AddTokenBytes(token.ID, 1234); err != nil {
		t.Fatal(err)
	}
	tokens, err := s.ListTokensTenant("acme")
	if err != nil || len(tokens) != 1 || tokens[0].RequestsToday != 1 || tokens[0].BytesToday != 1234 {
		t.Fatalf("tokens=%+v err=%v", tokens, err)
	}

	for _, entry := range []struct {
		at     string
		action string
	}{
		{"2026-07-24T10:00:00+08:00", "before"},
		{"2026-07-25T10:00:00+08:00", "inside"},
		{"2026-07-26T10:00:00+08:00", "after"},
	} {
		if _, err := s.exec(`INSERT INTO audit_logs(at,actor,action,detail) VALUES(?,?,?,?)`,
			entry.at, "acme:admin", entry.action, ""); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := s.ListAuditTenantRange(50, "acme",
		"2026-07-25T00:00:00+08:00", "2026-07-25T23:59:59+08:00")
	if err != nil || len(entries) != 1 || entries[0]["action"] != "inside" {
		t.Fatalf("audit=%+v err=%v", entries, err)
	}
}

func TestTokenQueriesHandleLegacyNullableColumns(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "legacy-token.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	createdAt := time.Now().Format(time.RFC3339)
	_, err = s.exec(`INSERT INTO tokens(id,name,token_hash,token_prefix,enabled,max_rps,allow_countries,expires_at,created_at,last_used_at,note,tenant_id,allow_protocols,daily_quota)
 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		"legacy-token", "legacy", "legacy-hash", "legacy-prefix", 1, nil, nil, nil,
		createdAt, nil, nil, "acme", nil, 0)
	if err != nil {
		t.Fatal(err)
	}

	list, err := s.ListTokensTenant("acme")
	if err != nil || len(list) != 1 {
		t.Fatalf("list=%+v err=%v", list, err)
	}
	if list[0].AllowCountries != nil || list[0].AllowProtocols != nil || list[0].Note != "" || list[0].MaxRPS != 0 {
		t.Fatalf("unexpected legacy token=%+v", list[0])
	}

	if _, err := s.FindTokenByHash("legacy-hash"); err != nil {
		t.Fatalf("find by hash: %v", err)
	}
	byPrefix, err := s.FindTokensByPrefix("legacy-prefix")
	if err != nil || len(byPrefix) != 1 {
		t.Fatalf("find by prefix=%+v err=%v", byPrefix, err)
	}
}
