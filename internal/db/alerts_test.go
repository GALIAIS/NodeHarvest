package db

import (
	"path/filepath"
	"testing"
)

func TestAlertLifecycle(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "alerts.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.RaiseAlert("quality-drop", "critical", "quality dropped", map[string]any{"drop": 50}); err != nil {
		t.Fatal(err)
	}
	alerts, err := store.ListAlerts(true, 10)
	if err != nil || len(alerts) != 1 || alerts[0].Details["drop"].(float64) != 50 {
		t.Fatalf("alerts=%+v err=%v", alerts, err)
	}
	if err := store.AcknowledgeAlert("quality-drop", "default:operator"); err != nil {
		t.Fatal(err)
	}
	if err := store.ResolveAlert("quality-drop"); err != nil {
		t.Fatal(err)
	}
	alerts, err = store.ListAlerts(true, 10)
	if err != nil || len(alerts) != 0 {
		t.Fatalf("active alerts=%+v err=%v", alerts, err)
	}
}
