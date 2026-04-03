package objectstore

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

var ErrNotImplemented = errors.New("object storage is not implemented in the MVP skeleton")

type Config struct {
	Endpoint              string
	Bucket                string
	UseTLS                bool
	AllowUnsignedRequests bool
	RequestTimeout        time.Duration
	HealthPath            string
	ObjectPathPrefix      string
	AdditionalHeaders     map[string]string
	InsecureSkipTLSVerify bool
}

type Client struct {
	cfg  Config
	http *http.Client
	base *url.URL
}

type ProbeResult struct {
	Endpoint   string    `json:"endpoint"`
	Bucket     string    `json:"bucket"`
	Healthy    bool      `json:"healthy"`
	CheckedAt  time.Time `json:"checked_at"`
	StatusCode int       `json:"status_code,omitempty"`
	Detail     string    `json:"detail,omitempty"`
}

type PutResult struct {
	Key        string    `json:"key"`
	URL        string    `json:"url"`
	StatusCode int       `json:"status_code"`
	UploadedAt time.Time `json:"uploaded_at"`
	Bytes      int       `json:"bytes"`
}

func New(cfg Config) (*Client, error) {
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = 10 * time.Second
	}
	if cfg.HealthPath == "" {
		cfg.HealthPath = "/minio/health/ready"
	}
	base, err := normalizeEndpoint(cfg.Endpoint, cfg.UseTLS)
	if err != nil {
		return nil, err
	}
	transport := &http.Transport{}
	if cfg.InsecureSkipTLSVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // local MVP wrapper only
	}
	return &Client{
		cfg: cfg,
		http: &http.Client{
			Timeout:   cfg.RequestTimeout,
			Transport: transport,
		},
		base: base,
	}, nil
}

func (c *Client) Health(ctx context.Context) (ProbeResult, error) {
	if c == nil {
		return ProbeResult{}, ErrNotImplemented
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base.ResolveReference(&url.URL{Path: c.cfg.HealthPath}).String(), nil)
	if err != nil {
		return ProbeResult{}, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return ProbeResult{Endpoint: c.base.String(), Bucket: c.cfg.Bucket, Healthy: false, CheckedAt: time.Now().UTC(), Detail: err.Error()}, err
	}
	defer resp.Body.Close()
	_, _ = io.ReadAll(io.LimitReader(resp.Body, 256))
	result := ProbeResult{
		Endpoint:   c.base.String(),
		Bucket:     c.cfg.Bucket,
		Healthy:    resp.StatusCode >= 200 && resp.StatusCode < 300,
		CheckedAt:  time.Now().UTC(),
		StatusCode: resp.StatusCode,
	}
	if !result.Healthy {
		result.Detail = resp.Status
		return result, fmt.Errorf("object store health check failed: %s", resp.Status)
	}
	return result, nil
}

func (c *Client) Put(ctx context.Context, key string, payload []byte, contentType string) (PutResult, error) {
	if c == nil {
		return PutResult{}, ErrNotImplemented
	}
	if !c.cfg.AllowUnsignedRequests && len(c.cfg.AdditionalHeaders) == 0 {
		return PutResult{}, fmt.Errorf("unsigned uploads are disabled and no auth headers are configured")
	}
	target, err := c.objectURL(key)
	if err != nil {
		return PutResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, target, bytes.NewReader(payload))
	if err != nil {
		return PutResult{}, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for k, v := range c.cfg.AdditionalHeaders {
		req.Header.Set(k, v)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return PutResult{}, err
	}
	defer resp.Body.Close()
	_, _ = io.ReadAll(io.LimitReader(resp.Body, 512))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return PutResult{}, fmt.Errorf("object upload failed: %s", resp.Status)
	}
	return PutResult{
		Key:        key,
		URL:        target,
		StatusCode: resp.StatusCode,
		UploadedAt: time.Now().UTC(),
		Bytes:      len(payload),
	}, nil
}

func (c *Client) MarshalProbe(result ProbeResult) string {
	data, err := json.Marshal(result)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func (c *Client) objectURL(key string) (string, error) {
	if c.base == nil {
		return "", ErrNotImplemented
	}
	base := *c.base
	base.Path = path.Join(base.Path, strings.Trim(c.cfg.Bucket, "/"), strings.TrimPrefix(c.cfg.ObjectPathPrefix, "/"), strings.TrimPrefix(key, "/"))
	return base.String(), nil
}

func normalizeEndpoint(endpoint string, useTLS bool) (*url.URL, error) {
	if endpoint == "" {
		return nil, fmt.Errorf("object store endpoint is required")
	}
	if !strings.Contains(endpoint, "://") {
		scheme := "http"
		if useTLS {
			scheme = "https"
		}
		endpoint = scheme + "://" + endpoint
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	return parsed, nil
}
