package hotcache

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/GALIAIS/NodeHarvest/internal/model"
	"github.com/GALIAIS/NodeHarvest/internal/publish"
)

type Client struct {
	redis    *redis.Client
	prefix   string
	cacheTTL time.Duration
	lockTTL  time.Duration
}

func Open(ctx context.Context, rawURL, prefix string, cacheTTL, lockTTL time.Duration) (*Client, error) {
	opt, err := redis.ParseURL(rawURL)
	if err != nil {
		return nil, err
	}
	rdb := redis.NewClient(opt)
	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = rdb.Close()
		return nil, err
	}
	if cacheTTL <= 0 {
		cacheTTL = 5 * time.Minute
	}
	if lockTTL <= 0 {
		lockTTL = 2 * time.Minute
	}
	prefix = strings.Trim(prefix, ": ")
	if prefix == "" {
		prefix = "nodeharvest"
	}
	return &Client{redis: rdb, prefix: prefix, cacheTTL: cacheTTL, lockTTL: lockTTL}, nil
}

func (c *Client) Close() error {
	if c == nil || c.redis == nil {
		return nil
	}
	return c.redis.Close()
}

func (c *Client) Ping(ctx context.Context) error {
	if c == nil || c.redis == nil {
		return fmt.Errorf("redis nil")
	}
	return c.redis.Ping(ctx).Err()
}

func (c *Client) key(name string) string {
	return c.prefix + ":" + name
}

func (c *Client) SetSnapshot(ctx context.Context, blob *publish.Blob, nodes []*model.Node) error {
	if c == nil || blob == nil {
		return nil
	}
	blobJSON, err := json.Marshal(blob)
	if err != nil {
		return err
	}
	nodesJSON, err := json.Marshal(nodes)
	if err != nil {
		return err
	}
	countryJSON, err := json.Marshal(blob.ByCountry)
	if err != nil {
		return err
	}
	pipe := c.redis.TxPipeline()
	pipe.Set(ctx, c.key("publish"), blobJSON, c.cacheTTL)
	pipe.Set(ctx, c.key("nodes:hq"), nodesJSON, c.cacheTTL)
	pipe.Set(ctx, c.key("countries"), countryJSON, c.cacheTTL)
	_, err = pipe.Exec(ctx)
	return err
}

func (c *Client) GetBlob(ctx context.Context) (*publish.Blob, error) {
	if c == nil {
		return nil, redis.Nil
	}
	data, err := c.redis.Get(ctx, c.key("publish")).Bytes()
	if err != nil {
		return nil, err
	}
	var blob publish.Blob
	if err := json.Unmarshal(data, &blob); err != nil {
		return nil, err
	}
	return &blob, nil
}

type Lock struct {
	client *Client
	key    string
	token  string
}

func (c *Client) Acquire(ctx context.Context, name string) (*Lock, bool, error) {
	token := randomToken()
	key := c.key("lock:" + name)
	ok, err := c.redis.SetNX(ctx, key, token, c.lockTTL).Result()
	if err != nil || !ok {
		return nil, ok, err
	}
	return &Lock{client: c, key: key, token: token}, true, nil
}

var releaseScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0`)

func (l *Lock) Release(ctx context.Context) error {
	if l == nil || l.client == nil {
		return nil
	}
	return releaseScript.Run(ctx, l.client.redis, []string{l.key}, l.token).Err()
}

func randomToken() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes[:])
}
