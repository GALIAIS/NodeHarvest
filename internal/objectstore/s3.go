package objectstore

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/GALIAIS/NodeHarvest/internal/config"
	"github.com/GALIAIS/NodeHarvest/internal/publish"
)

type Client struct {
	s3     *minio.Client
	bucket string
	prefix string
}

func Open(ctx context.Context, cfg config.ObjectStoreConfig) (*Client, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	endpoint, err := url.Parse(cfg.Endpoint)
	if err != nil {
		return nil, err
	}
	client, err := minio.New(endpoint.Host, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, cfg.SessionToken),
		Secure: endpoint.Scheme == "https", Region: cfg.Region,
	})
	if err != nil {
		return nil, err
	}
	exists, err := client.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return nil, err
	}
	if !exists {
		if !cfg.AutoCreateBucket {
			return nil, fmt.Errorf("object-store bucket %q does not exist", cfg.Bucket)
		}
		if err := client.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{Region: cfg.Region}); err != nil {
			return nil, err
		}
	}
	return &Client{s3: client, bucket: cfg.Bucket, prefix: strings.Trim(cfg.Prefix, "/ ")}, nil
}

func (c *Client) UploadSnapshot(ctx context.Context, blob *publish.Blob) error {
	if c == nil || blob == nil {
		return nil
	}
	metadata, err := json.Marshal(blob)
	if err != nil {
		return err
	}
	stamp := time.Now().UTC().Format("20060102T150405.000000000Z")
	objects := []struct {
		key, contentType string
		body             []byte
	}{
		{"sub.txt", "text/plain; charset=utf-8", []byte(blob.Raw)},
		{"sub.base64", "text/plain; charset=utf-8", []byte(blob.Base64)},
		{"clash.yaml", "text/yaml; charset=utf-8", []byte(blob.Clash)},
		{"sub.meta.json", "application/json", metadata},
	}
	for _, object := range objects {
		if err := c.put(ctx, objectKey(c.prefix, "current", object.key), object.contentType, object.body); err != nil {
			return err
		}
		if err := c.put(ctx, objectKey(c.prefix, "snapshots", stamp, object.key), object.contentType, object.body); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) put(ctx context.Context, key, contentType string, body []byte) error {
	_, err := c.s3.PutObject(ctx, c.bucket, key, bytes.NewReader(body), int64(len(body)), minio.PutObjectOptions{
		ContentType: contentType, CacheControl: "public, max-age=120",
	})
	return err
}

func objectKey(parts ...string) string {
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		for _, segment := range strings.Split(strings.ReplaceAll(part, `\`, "/"), "/") {
			segment = strings.TrimSpace(segment)
			if segment != "" && segment != "." && segment != ".." {
				filtered = append(filtered, segment)
			}
		}
	}
	return path.Join(filtered...)
}
