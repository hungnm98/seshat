package redis

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

var ErrNotImplemented = errors.New("redis cache is not implemented in the MVP skeleton")

type Config struct {
	Addr               string
	Password           string
	DB                 int
	DialTimeout        time.Duration
	ReadTimeout        time.Duration
	WriteTimeout       time.Duration
	UseTLS             bool
	InsecureSkipVerify bool
}

type Client struct {
	cfg Config
}

type ProbeResult struct {
	Addr      string    `json:"addr"`
	Healthy   bool      `json:"healthy"`
	CheckedAt time.Time `json:"checked_at"`
	Status    string    `json:"status,omitempty"`
	LastError string    `json:"last_error,omitempty"`
}

func New(cfg Config) *Client {
	if cfg.DialTimeout <= 0 {
		cfg.DialTimeout = 5 * time.Second
	}
	if cfg.ReadTimeout <= 0 {
		cfg.ReadTimeout = 5 * time.Second
	}
	if cfg.WriteTimeout <= 0 {
		cfg.WriteTimeout = 5 * time.Second
	}
	if cfg.DB < 0 {
		cfg.DB = 0
	}
	return &Client{cfg: cfg}
}

func (c *Client) Health(ctx context.Context) (ProbeResult, error) {
	_, err := c.doSimple(ctx, "PING")
	result := ProbeResult{
		Addr:      c.cfg.Addr,
		Healthy:   err == nil,
		CheckedAt: time.Now().UTC(),
	}
	if err != nil {
		result.LastError = err.Error()
		return result, err
	}
	result.Status = "PONG"
	return result, nil
}

func (c *Client) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	args := []string{"SET", key, value}
	if ttl > 0 {
		args = append(args, "EX", strconv.Itoa(int(ttl.Seconds())))
	}
	_, err := c.doSimple(ctx, args...)
	return err
}

func (c *Client) Get(ctx context.Context, key string) (string, bool, error) {
	resp, err := c.doSimple(ctx, "GET", key)
	if err != nil {
		return "", false, err
	}
	if resp == nil {
		return "", false, nil
	}
	value, ok := resp.(string)
	return value, ok, nil
}

func (c *Client) doSimple(ctx context.Context, args ...string) (interface{}, error) {
	conn, err := c.dial(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	_ = conn.SetWriteDeadline(time.Now().Add(c.cfg.WriteTimeout))
	if c.cfg.Password != "" {
		if err := writeRESP(conn, "AUTH", c.cfg.Password); err != nil {
			return nil, err
		}
		if _, err := readRESP(bufio.NewReader(conn)); err != nil {
			return nil, err
		}
	}
	if c.cfg.DB > 0 {
		if err := writeRESP(conn, "SELECT", strconv.Itoa(c.cfg.DB)); err != nil {
			return nil, err
		}
		if _, err := readRESP(bufio.NewReader(conn)); err != nil {
			return nil, err
		}
	}
	_ = conn.SetReadDeadline(time.Now().Add(c.cfg.ReadTimeout))
	if err := writeRESP(conn, args...); err != nil {
		return nil, err
	}
	return readRESP(bufio.NewReader(conn))
}

func (c *Client) dial(ctx context.Context) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: c.cfg.DialTimeout}
	if c.cfg.UseTLS {
		return tls.DialWithDialer(dialer, "tcp", c.cfg.Addr, &tls.Config{InsecureSkipVerify: c.cfg.InsecureSkipVerify}) // local MVP wrapper only
	}
	return dialer.DialContext(ctx, "tcp", c.cfg.Addr)
}

func writeRESP(conn net.Conn, args ...string) error {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("*%d\r\n", len(args)))
	for _, arg := range args {
		builder.WriteString(fmt.Sprintf("$%d\r\n%s\r\n", len(arg), arg))
	}
	_, err := conn.Write([]byte(builder.String()))
	return err
}

func readRESP(reader *bufio.Reader) (interface{}, error) {
	prefix, err := reader.ReadByte()
	if err != nil {
		return nil, err
	}
	line, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimSuffix(line, "\r\n")
	switch prefix {
	case '+':
		return line, nil
	case '-':
		return nil, errors.New(line)
	case ':':
		return line, nil
	case '$':
		if line == "-1" {
			return nil, nil
		}
		size, err := strconv.Atoi(line)
		if err != nil {
			return nil, err
		}
		buf := make([]byte, size+2)
		if _, err := readFull(reader, buf); err != nil {
			return nil, err
		}
		return string(buf[:size]), nil
	default:
		return nil, fmt.Errorf("unexpected RESP prefix %q", prefix)
	}
}

func readFull(reader *bufio.Reader, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := reader.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}
