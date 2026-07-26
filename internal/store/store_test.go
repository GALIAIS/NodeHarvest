package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/GALIAIS/NodeHarvest/internal/model"
)

func TestReplaceNodesPreservesObservationsAndFreshness(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	seen := time.Now().Add(-time.Hour).Round(time.Second)
	old := &model.Node{
		Protocol: model.ProtoVLESS, Server: "example.com", Port: 443, UUID: "id",
		Network: "tcp", TLS: true, Alive: true, Score: 88, Grade: "A",
		TestedAt: seen, LastSeenAt: seen,
	}
	if err := s.ReplaceNodes([]*model.Node{old}); err != nil {
		t.Fatal(err)
	}

	fresh := &model.Node{
		Protocol: model.ProtoVLESS, Server: "example.com", Port: 443, UUID: "id",
		Network: "tcp", TLS: true, RawURI: "vless://id@example.com:443",
		LastSeenAt: time.Now(),
	}
	if err := s.ReplaceNodes([]*model.Node{fresh}); err != nil {
		t.Fatal(err)
	}
	got := s.AllNodes()[0]
	if !got.Alive || got.Score != 88 || got.TestedAt.IsZero() {
		t.Fatalf("observations lost: %+v", got)
	}
	if !got.LastSeenAt.After(seen) {
		t.Fatalf("freshness not updated: %s", got.LastSeenAt)
	}
}

func TestListNodesReturnsIndependentValues(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceNodes([]*model.Node{{
		Protocol: model.ProtoVLESS, Server: "example.com", Port: 443, UUID: "id",
		Extra:    map[string]string{"pbk": "original"},
		Source:   "primary",
		Sources:  []string{"primary", "secondary"},
		Quality:  &model.Quality{Notes: []string{"original"}},
		AIAccess: map[string]*model.AIProbeResult{"ai": {OK: true}},
	}}); err != nil {
		t.Fatal(err)
	}

	got := s.AllNodes()[0]
	got.Extra["pbk"] = "changed"
	got.Quality.Notes[0] = "changed"
	got.AIAccess["ai"].OK = false

	stored := s.AllNodes()[0]
	if stored.Extra["pbk"] != "original" || stored.Quality.Notes[0] != "original" || !stored.AIAccess["ai"].OK {
		t.Fatalf("store value was mutated through a list result: %+v", stored)
	}
	if len(s.ListNodes(NodeFilter{Source: "secondary"})) != 1 {
		t.Fatal("source provenance filter did not match")
	}
}

func TestNewRejectsCorruptSnapshot(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "snapshot.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := New(dir); err == nil {
		t.Fatal("corrupt snapshot was silently ignored")
	}
}
