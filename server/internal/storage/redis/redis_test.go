package redis

import "testing"

func TestNewDefaults(t *testing.T) {
	c := New(Config{Addr: "127.0.0.1:6379"})
	if c == nil {
		t.Fatal("expected client")
	}
	if c.cfg.DialTimeout <= 0 || c.cfg.ReadTimeout <= 0 || c.cfg.WriteTimeout <= 0 {
		t.Fatal("expected default timeouts")
	}
}
