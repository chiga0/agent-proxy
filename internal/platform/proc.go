package platform

import (
	"os"
	"strings"
)

// CountPatternInFile counts occurrences of pattern in the file at path.
// Pure Go implementation — works on all platforms.
func CountPatternInFile(path, pattern string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strings.Count(string(data), pattern), nil
}
