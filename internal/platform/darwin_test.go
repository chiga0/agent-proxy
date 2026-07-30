//go:build darwin

package platform

import (
	"testing"
)

func TestExtractNum(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"standard", "(1) Wi-Fi", "1"},
		{"multi_digit", "(12) Ethernet", "12"},
		{"no_parens", "Wi-Fi", "0"},
		{"empty", "", "0"},
		{"empty_parens", "()", "0"},
		{"single_char", "(X) Foo", "X"},
		{"leading_space", "  (3) Thunderbolt", "3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractNum(tt.input)
			if got != tt.want {
				t.Errorf("extractNum(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestCaptureExtraStateReturnsNil(t *testing.T) {
	data, err := CaptureExtraState("Wi-Fi")
	if err != nil {
		t.Fatalf("CaptureExtraState() error: %v", err)
	}
	if data != nil {
		t.Errorf("CaptureExtraState() = %v, want nil", data)
	}
}

func TestRestoreExtraStateNoOp(t *testing.T) {
	err := RestoreExtraState("Wi-Fi", nil)
	if err != nil {
		t.Fatalf("RestoreExtraState(nil) error: %v", err)
	}
	err = RestoreExtraState("Wi-Fi", map[string]string{"foo": "bar"})
	if err != nil {
		t.Fatalf("RestoreExtraState(data) error: %v", err)
	}
}
