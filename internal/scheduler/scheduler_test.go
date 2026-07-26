package scheduler

import (
	"testing"

	"github.com/GALIAIS/NodeHarvest/internal/config"
	"github.com/GALIAIS/NodeHarvest/internal/model"
	"github.com/GALIAIS/NodeHarvest/internal/service"
	"github.com/GALIAIS/NodeHarvest/internal/store"
)

func TestStatusReadsCurrentJobStateFromStore(t *testing.T) {
	cfg := config.Default()
	cfg.Geo.Enabled = false
	cfg.Publish.PreRender = false
	cfg.Export.Dir = t.TempDir()
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc := service.New(cfg, st)
	scheduler := New(cfg, svc)
	scheduler.lastJobID = "job"
	st.SaveJob(&model.Job{ID: "job", Type: "fetch", Status: model.JobCompleted, Progress: 100})

	status := scheduler.Status()
	job, ok := status["last_job"].(map[string]any)
	if !ok || job["status"] != model.JobCompleted {
		t.Fatalf("last_job=%v", status["last_job"])
	}
}

func TestStopWaitsForSchedulerLoop(t *testing.T) {
	cfg := config.Default()
	cfg.Geo.Enabled = false
	cfg.Publish.PreRender = false
	cfg.Export.Dir = t.TempDir()
	cfg.Schedule.Enabled = true
	cfg.Schedule.IntervalMin = 60
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	scheduler := New(cfg, service.New(cfg, st))
	scheduler.Start()
	scheduler.Stop()
	if scheduler.Status()["running"] != false {
		t.Fatal("scheduler still running after Stop")
	}
}
