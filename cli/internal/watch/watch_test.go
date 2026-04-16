package watch

import "testing"

func TestShouldIgnoreDefaultDirectories(t *testing.T) {
	for _, rel := range []string{".git/config", ".seshat/index/graph.json", "vendor/a.go", "node_modules/pkg/index.js", "build/out"} {
		if !ShouldIgnore(rel, nil) {
			t.Fatalf("expected %s to be ignored", rel)
		}
	}
	if ShouldIgnore("internal/order/service.go", nil) {
		t.Fatal("did not expect source file to be ignored")
	}
}
