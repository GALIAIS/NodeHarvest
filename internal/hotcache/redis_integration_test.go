//go:build integration

package hotcache

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/GALIAIS/NodeHarvest/internal/publish"
)

func TestRedisSnapshotAndOwnedLock(t *testing.T) {
	rawURL := os.Getenv("NODE_HARVEST_TEST_REDIS")
	if rawURL == "" {
		t.Skip("NODE_HARVEST_TEST_REDIS is not configured")
	}
	ctx := context.Background()
	client, err := Open(ctx, rawURL, "nodeharvest-integration", time.Minute, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	blob := &publish.Blob{Count: 1, Raw: "vless://example", UpdatedAt: time.Now().Format(time.RFC3339)}
	if err := client.SetSnapshot(ctx, blob, nil); err != nil {
		t.Fatal(err)
	}
	loaded, err := client.GetBlob(ctx)
	if err != nil || loaded.Raw != blob.Raw {
		t.Fatalf("blob=%+v err=%v", loaded, err)
	}
	lock, acquired, err := client.Acquire(ctx, "publish")
	if err != nil || !acquired {
		t.Fatalf("lock acquired=%v err=%v", acquired, err)
	}
	if _, acquired, err := client.Acquire(ctx, "publish"); err != nil || acquired {
		t.Fatalf("second lock acquired=%v err=%v", acquired, err)
	}
	if err := lock.Release(ctx); err != nil {
		t.Fatal(err)
	}
}
