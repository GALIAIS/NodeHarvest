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
