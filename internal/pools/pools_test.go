package pools

import (
	"testing"
	"time"

	"github.com/GALIAIS/NodeHarvest/internal/config"
	"github.com/GALIAIS/NodeHarvest/internal/model"
	"github.com/GALIAIS/NodeHarvest/internal/store"
)

func TestConfiguredPoolEnforcesAllDimensions(t *testing.T) {
	cfg := config.Default()
	cfg.Pools = []config.PoolConfig{{
		Key: "streaming", Name: "Streaming", MinScore: 70, MaxLatencyMS: 300,
		Countries: []string{"US"}, Protocols: []string{"vless"}, RequireVerified: true, MaxNodes: 10,
	}}
	st := store.NewMemory()
	nodes := []*model.Node{
		{Protocol: model.ProtoVLESS, Server: "ok", Port: 443, UUID: "1", Alive: true, Verified: true, Score: 90, Country: "US", Latency: 100 * time.Millisecond},
		{Protocol: model.ProtoVLESS, Server: "slow", Port: 443, UUID: "2", Alive: true, Verified: true, Score: 90, Country: "US", Latency: time.Second},
		{Protocol: model.ProtoVMess, Server: "wrong", Port: 443, UUID: "3", Alive: true, Verified: true, Score: 90, Country: "US", Latency: 100 * time.Millisecond},
	}
	if err := st.ReplaceNodes(nodes); err != nil {
		t.Fatal(err)
	}
	got := Select(st, Configured(cfg)[0])
	if len(got) != 1 || got[0].Server != "ok" {
		t.Fatalf("selected=%+v", got)
	}
}
