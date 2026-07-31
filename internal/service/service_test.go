package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/GALIAIS/NodeHarvest/internal/config"
	"github.com/GALIAIS/NodeHarvest/internal/db"
	"github.com/GALIAIS/NodeHarvest/internal/model"
	"github.com/GALIAIS/NodeHarvest/internal/publish"
	"github.com/GALIAIS/NodeHarvest/internal/store"
)

func TestWaitForJobWaitsForCompletion(t *testing.T) {
	cfg := config.Default()
	cfg.Geo.Enabled = false
	cfg.Publish.PreRender = false
	cfg.Export.Dir = t.TempDir()
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc := New(cfg, st)
	job, err := svc.StartQuality(nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := svc.WaitForJob(ctx); err != nil {
		t.Fatal(err)
	}
	got, ok := st.GetJob(job.ID)
	if !ok || got.Status != model.JobFailed {
		t.Fatalf("job=%+v", got)
	}
}

func TestQualityAnomaliesCompareDurableBaseline(t *testing.T) {
	cfg := config.Default()
	cfg.Geo.Enabled = false
	cfg.Publish.PreRender = false
	cfg.Export.Dir = t.TempDir()
	database, err := db.Open(filepath.Join(t.TempDir(), "anomaly.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	ended := time.Now().Add(-time.Minute)
	previous := &model.Job{
		ID: "previous", Type: "quality", Status: model.JobCompleted,
		Stats: map[string]any{"high_quality": 100}, CreatedAt: ended, UpdatedAt: ended, EndedAt: &ended,
	}
	if err := database.SaveJob(previous); err != nil {
		t.Fatal(err)
	}
	svc := NewWithOptions(cfg, store.NewMemory(), Options{DB: database})
	current := &model.Job{ID: "current", Type: "quality"}
	svc.detectQualityAnomalies(current, 20, 10, 20, map[string]int{"US": 20})
	alerts, err := database.ListAlerts(true, 10)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{}
	for _, alert := range alerts {
		kinds[alert.Kind] = true
	}
	if !kinds["high-quality-drop"] || !kinds["country-dominance"] {
		t.Fatalf("alerts=%+v", alerts)
	}
}

func TestRaiseAlertSendsSignedWebhook(t *testing.T) {
	const secret = "webhook-test-secret"
	var received bool
	webhook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write(payload)
		want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
		if r.Header.Get("X-NodeHarvest-Signature") != want {
			t.Errorf("signature=%q want=%q", r.Header.Get("X-NodeHarvest-Signature"), want)
		}
		received = true
		w.WriteHeader(http.StatusNoContent)
	}))
	defer webhook.Close()

	cfg := config.Default()
	cfg.Export.Dir = t.TempDir()
	cfg.Governance.AlertWebhookURL = webhook.URL
	cfg.Governance.AlertWebhookSecret = secret
	database, err := db.Open(filepath.Join(t.TempDir(), "webhook.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	svc := NewWithOptions(cfg, store.NewMemory(), Options{DB: database})
	if err := svc.raiseAlert("test-alert", "warning", "test message", map[string]any{"value": 1}); err != nil {
		t.Fatal(err)
	}
	if !received {
		t.Fatal("webhook was not received")
	}
}

func TestVerifiedFreshnessControlsRetestAndPublish(t *testing.T) {
	cfg := config.Default()
	cfg.Geo.Enabled = false
	cfg.Publish.PreRender = false
	cfg.Export.Dir = t.TempDir()
	cfg.Publish.MaxNodes = 10
	cfg.Dial.VerifiedTTLHours = 6
	now := time.Now()
	fresh := &model.Node{
		Protocol: model.ProtoVLESS, Name: "fresh", Server: "fresh.example", Port: 443, UUID: "fresh",
		Alive: true, Verified: true, Score: 90, LastSeenAt: now,
		Dial: &model.DialResult{OK: true, Engine: "both", TestedAt: now, Checks: []*model.DialResult{
			{OK: true, Engine: "sing-box", TestedAt: now}, {OK: true, Engine: "mihomo", TestedAt: now},
		}},
	}
	stale := &model.Node{
		Protocol: model.ProtoVLESS, Name: "stale", Server: "stale.example", Port: 443, UUID: "stale",
		Alive: true, Verified: true, Score: 90, LastSeenAt: now,
		Dial: &model.DialResult{OK: true, Engine: "both", TestedAt: now.Add(-7 * time.Hour), Checks: []*model.DialResult{
			{OK: true, Engine: "sing-box", TestedAt: now.Add(-7 * time.Hour)},
			{OK: true, Engine: "mihomo", TestedAt: now.Add(-7 * time.Hour)},
		}},
	}
	legacy := &model.Node{
		Protocol: model.ProtoVLESS, Name: "legacy", Server: "legacy.example", Port: 443, UUID: "legacy",
		Alive: true, Verified: true, Score: 90, LastSeenAt: now,
		Dial: &model.DialResult{OK: true, Engine: "sing-box", TestedAt: now},
	}
	st := store.NewMemory()
	if err := st.ReplaceNodes([]*model.Node{fresh, stale, legacy}); err != nil {
		t.Fatal(err)
	}
	svc := New(cfg, st)
	candidates := svc.pickDialCandidates(0, true, true)
	if len(candidates) != 2 {
		t.Fatalf("retest candidates=%+v", candidates)
	}
	retest := map[string]bool{}
	for _, node := range candidates {
		retest[node.Server] = true
	}
	if !retest["stale.example"] || !retest["legacy.example"] || retest["fresh.example"] {
		t.Fatalf("wrong retest set=%+v", retest)
	}
	published := svc.SelectPublishNodes()
	if len(published) != 1 || published[0].Server != "fresh.example" {
		t.Fatalf("published=%+v", published)
	}
}

func TestServiceRejectsLegacyPublishCache(t *testing.T) {
	cfg := config.Default()
	cfg.Geo.Enabled = false
	cfg.Publish.PreRender = false
	cfg.Export.Dir = t.TempDir()
	publish.NewCache(cfg.Export.Dir).Update([]*model.Node{{
		Protocol: model.ProtoVLESS, Server: "legacy.example", Port: 443,
		RawURI: "vless://legacy@legacy.example:443",
	}}, 1, "legacy-policy")

	svc := New(cfg, store.NewMemory())
	if blob := svc.PublishCache().Get(); blob != nil {
		t.Fatalf("legacy publish cache was accepted: %+v", blob)
	}
}

func TestPublishPolicyIgnoresSecretsButTracksDialRules(t *testing.T) {
	first := config.Default()
	second := config.Default()
	first.Auth.SessionSecret = "first-secret"
	second.Auth.SessionSecret = "second-secret"
	if (&Service{cfg: first}).publishPolicy() != (&Service{cfg: second}).publishPolicy() {
		t.Fatal("unrelated secrets changed the shared publish cache policy")
	}
	second.Dial.Engine = "mihomo"
	if (&Service{cfg: first}).publishPolicy() == (&Service{cfg: second}).publishPolicy() {
		t.Fatal("dial engine change did not invalidate the publish cache")
	}
}

func TestMihomoIsAuthoritativeInBothMode(t *testing.T) {
	now := time.Now()
	result := combineDialChecks([]*model.DialResult{
		{OK: false, Engine: "sing-box", Error: "unsupported transport", TestedAt: now},
		{OK: true, Engine: "mihomo", LatencyMS: 80, TestedAt: now.Add(time.Second)},
	})
	if result == nil || !result.OK || result.Engine != "both" || len(result.Checks) != 2 || result.LatencyMS != 80 {
		t.Fatalf("combined=%+v", result)
	}
}
