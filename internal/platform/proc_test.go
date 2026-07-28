package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCountPatternInFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello world\nhello foo\nbar hello\n"), 0644)

	count, err := CountPatternInFile(path, "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3, got %d", count)
	}

	count, err = CountPatternInFile(path, "missing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0, got %d", count)
	}

	_, err = CountPatternInFile("/nonexistent/path", "x")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestFindPIDsByPattern(t *testing.T) {
	// Should return empty for a pattern that matches nothing
	pids := FindPIDsByPattern("zzz_nonexistent_process_zzz_12345")
	if len(pids) != 0 {
		t.Errorf("expected 0 PIDs, got %d", len(pids))
	}
}

func TestIsProcessAlive(t *testing.T) {
	// Current process should be alive
	if !IsProcessAlive(os.Getpid()) {
		t.Error("current process should be alive")
	}
	// PID 99999999 should not exist
	if IsProcessAlive(99999999) {
		t.Error("PID 99999999 should not be alive")
	}
}
