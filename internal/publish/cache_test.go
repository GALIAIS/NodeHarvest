package publish

import (
	"testing"
	"time"

	"github.com/GALIAIS/NodeHarvest/internal/model"
)

func TestCacheUpdateReplacesFilesAndRejectsFutureMetadata(t *testing.T) {
	dir := t.TempDir()
	cache := NewCache(dir)
	cache.Update([]*model.Node{{
		Protocol: model.ProtoVLESS, Server: "a.example", Port: 443,
		RawURI: "vless://a@a.example:443",
	}}, 1, "test-policy")
	cache.Update([]*model.Node{{
		Protocol: model.ProtoVLESS, Server: "b.example", Port: 443,
		RawURI: "vless://b@b.example:443",
	}}, 1, "test-policy")

	loaded := NewCache(dir).Get()
	if loaded == nil || loaded.Count != 1 || loaded.Raw != "vless://b@b.example:443#node-1\n" ||
		!loaded.MatchesPolicy("test-policy") || loaded.MatchesPolicy("other-policy") {
		t.Fatalf("loaded cache=%+v", loaded)
	}
	future := &Blob{UpdatedAt: time.Now().Add(time.Hour).Format(time.RFC3339)}
	if future.Fresh(24 * time.Hour) {
		t.Fatal("future cache metadata was accepted")
	}
}
