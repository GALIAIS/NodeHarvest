//go:build integration

package db

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestPostgresLeaseRoundTrip(t *testing.T) {
	dsn := os.Getenv("NODE_HARVEST_TEST_POSTGRES")
	if dsn == "" {
		t.Skip("NODE_HARVEST_TEST_POSTGRES is not configured")
	}
	store, err := OpenDatabase(DatabaseOptions{Driver: "postgres", DSN: dsn})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	id := "integration-" + time.Now().Format("20060102150405.000000000")
	task := &QueuedTask{ID: id, Type: "fetch", Priority: 100, MaxAttempts: 2}
	if err := store.EnqueueTask(task, 100); err != nil {
		t.Fatal(err)
	}
	leased, err := store.LeaseTask(context.Background(), "integration-worker", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if leased.ID != id || leased.Status != TaskRunning {
		t.Fatalf("leased=%+v", leased)
	}
	if err := store.CompleteTask(id, "integration-worker"); err != nil {
		t.Fatal(err)
	}
}
