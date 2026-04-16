package objectstore

import "testing"

func TestNormalizeEndpoint(t *testing.T) {
	c, err := New(Config{Endpoint: "localhost:9000", Bucket: "seshat"})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if got := c.base.String(); got != "http://localhost:9000" {
		t.Fatalf("unexpected normalized endpoint: %s", got)
	}
}
