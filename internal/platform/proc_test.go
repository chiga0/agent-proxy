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

func TestCountPatternInFileEmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.txt")
	os.WriteFile(path, []byte(""), 0644)

	count, err := CountPatternInFile(path, "anything")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 for empty file, got %d", count)
	}
}

func TestCountPatternInFileEmptyPattern(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello"), 0644)

	// strings.Count("hello", "") returns len+1 = 6
	count, err := CountPatternInFile(path, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 6 {
		t.Errorf("expected 6 for empty pattern, got %d", count)
	}
}

func TestCountPatternInFileOverlapping(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "overlap.txt")
	os.WriteFile(path, []byte("aaa"), 0644)

	// strings.Count does NOT count overlapping: "aaa" has "aa" at 0 and 1, but Count returns 1
	count, err := CountPatternInFile(path, "aa")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 (non-overlapping), got %d", count)
	}
}

func TestCountPatternInFileMultiline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "multi.txt")
	os.WriteFile(path, []byte("proxy\nno_proxy\nproxy\nhttp_proxy\n"), 0644)

	count, err := CountPatternInFile(path, "proxy")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// "proxy" appears in: proxy, no_proxy, proxy, http_proxy = 4
	if count != 4 {
		t.Errorf("expected 4, got %d", count)
	}
}

func TestCountPatternInFileBinaryContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "binary.bin")
	data := []byte{0x00, 0x01, 0x02, 0x41, 0x42, 0x43, 0x00}
	os.WriteFile(path, data, 0644)

	count, err := CountPatternInFile(path, "ABC")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1, got %d", count)
	}
}

func TestCountPatternInFileDirectory(t *testing.T) {
	dir := t.TempDir()
	// Reading a directory should return an error
	_, err := CountPatternInFile(dir, "x")
	if err == nil {
		t.Error("expected error when reading a directory")
	}
}

func TestErrPACNotSupported(t *testing.T) {
	if ErrPACNotSupported == nil {
		t.Fatal("ErrPACNotSupported should not be nil")
	}
	if ErrPACNotSupported.Error() != "system PAC not supported on this platform" {
		t.Errorf("unexpected error message: %q", ErrPACNotSupported.Error())
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

func TestIsProcessAlivePIDZero(t *testing.T) {
	// PID 0 behavior varies by OS; just verify no panic
	_ = IsProcessAlive(0)
}

func TestIsProcessAliveNegativePID(t *testing.T) {
	// Negative PID behavior varies by OS; just verify no panic
	_ = IsProcessAlive(-1)
}

func TestGetProcessNameCurrent(t *testing.T) {
	name := GetProcessName(os.Getpid())
	// The current process is a test binary; name should be non-empty
	if name == "" {
		t.Error("GetProcessName for current PID returned empty string")
	}
}

func TestGetProcessNameNonexistent(t *testing.T) {
	name := GetProcessName(99999999)
	if name != "" {
		t.Errorf("GetProcessName(99999999) = %q, want empty", name)
	}
}

func TestGetProcessArgsCurrent(t *testing.T) {
	args := GetProcessArgs(os.Getpid())
	// The current process should have args (at least the test binary path)
	if args == "" {
		t.Error("GetProcessArgs for current PID returned empty string")
	}
}

func TestGetProcessArgsNonexistent(t *testing.T) {
	args := GetProcessArgs(99999999)
	if args != "" {
		t.Errorf("GetProcessArgs(99999999) = %q, want empty", args)
	}
}
