package projectmgmt

import "testing"

func TestValidateTasks(t *testing.T) {
	if err := ValidateTasks("../../.."); err != nil {
		t.Fatalf("ValidateTasks returned error: %v", err)
	}
}
