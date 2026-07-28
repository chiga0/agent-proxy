package platform

import (
	"errors"
	"os"
	"strings"
)

// ErrPACNotSupported indicates system PAC configuration is unavailable
// on this platform (e.g., Linux without gsettings). CLI env vars still work.
var ErrPACNotSupported = errors.New("system PAC not supported on this platform")

// CountPatternInFile counts occurrences of pattern in the file at path.
// Pure Go implementation — works on all platforms.
func CountPatternInFile(path, pattern string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strings.Count(string(data), pattern), nil
}
