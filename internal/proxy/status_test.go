package proxy

import (
	"testing"
)

func TestCheckResultFields(t *testing.T) {
	r := CheckResult{Name: "test", OK: true, Detail: "ok"}
	if r.Name != "test" || !r.OK || r.Detail != "ok" {
		t.Errorf("unexpected CheckResult: %+v", r)
	}
}

func TestPrintStatus(t *testing.T) {
	results := []CheckResult{
		{Name: "check1", OK: true, Detail: "pass"},
		{Name: "check2", OK: false, Detail: "fail"},
	}
	// Just verify it doesn't panic
	PrintStatus(results)
}
