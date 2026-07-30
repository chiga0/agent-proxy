package setup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandHome(t *testing.T) {
	home, _ := os.UserHomeDir()

	tests := []struct {
		input string
		want  string
	}{
		{"~/.ssh/id_rsa", home + "/.ssh/id_rsa"},
		{"~/key.pem", home + "/key.pem"},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
		{"~", "~"}, // no slash, not expanded
	}
	for _, tt := range tests {
		if got := ExpandHome(tt.input); got != tt.want {
			t.Errorf("ExpandHome(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestValidateSSHKeyNotFound(t *testing.T) {
	_, err := ValidateSSHKey("/nonexistent/path/key.pem")
	if err == nil {
		t.Error("expected error for nonexistent key")
	}
}

func TestValidateSSHKeyFixesPermissions(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "test_key")
	os.WriteFile(keyPath, []byte("fake-key"), 0644)

	got, err := ValidateSSHKey(keyPath)
	if err != nil {
		t.Fatalf("ValidateSSHKey: %v", err)
	}
	if got != keyPath {
		t.Errorf("expected same path, got %q", got)
	}

	info, _ := os.Stat(keyPath)
	if info.Mode().Perm() != 0600 {
		t.Errorf("expected 0600 permissions, got 0%o", info.Mode().Perm())
	}
}
