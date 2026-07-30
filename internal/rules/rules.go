package rules

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func cacheDir(dataDir string) string {
	return filepath.Join(dataDir, "rules")
}

func cacheFile(dataDir, url, action string) string {
	h := sha256.Sum256([]byte(url))
	name := fmt.Sprintf("%x", h[:8])
	return filepath.Join(cacheDir(dataDir), action+"_"+name+".domains")
}

// FetchAll downloads each URL and caches the domain list locally.
// The caller provides an *http.Client (typically configured with the proxy,
// since rule list URLs are often hosted on blocked sites like GitHub).
// Returns the number of sources successfully fetched.
func FetchAll(client *http.Client, dataDir string, sources map[string]string) (int, error) {
	if err := os.MkdirAll(cacheDir(dataDir), 0700); err != nil {
		return 0, fmt.Errorf("create rules cache dir: %w", err)
	}

	ok := 0
	var lastErr error

	for url, action := range sources {
		if action == "" {
			action = "proxy"
		}
		domains, err := fetch(client, url)
		if err != nil {
			lastErr = fmt.Errorf("fetch %s: %w", url, err)
			continue
		}
		if err := writeCache(cacheFile(dataDir, url, action), domains); err != nil {
			lastErr = fmt.Errorf("cache %s: %w", url, err)
			continue
		}
		ok++
	}
	return ok, lastErr
}

func fetch(client *http.Client, url string) ([]string, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var domains []string
	seen := make(map[string]bool)
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		// Strip v2fly-style prefixes: "full:", "domain:", "regexp:", "keyword:"
		for _, prefix := range []string{"full:", "domain:", "regexp:", "keyword:"} {
			if strings.HasPrefix(line, prefix) {
				line = strings.TrimPrefix(line, prefix)
				break
			}
		}
		line = strings.ToLower(strings.TrimSpace(line))
		if line != "" && !seen[line] {
			seen[line] = true
			domains = append(domains, line)
		}
	}
	return domains, sc.Err()
}

func writeCache(path string, domains []string) error {
	var b strings.Builder
	b.WriteString("# auto-fetched by agent-proxy update-rules\n")
	for _, d := range domains {
		b.WriteString(d)
		b.WriteByte('\n')
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// CachedDomains returns all cached domains for the given action ("proxy" or "direct").
func CachedDomains(dataDir, action string) []string {
	dir := cacheDir(dataDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	prefix := action + "_"
	var domains []string
	seen := make(map[string]bool)

	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), prefix) || !strings.HasSuffix(e.Name(), ".domains") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if !seen[line] {
				seen[line] = true
				domains = append(domains, line)
			}
		}
	}
	return domains
}

// CacheInfo returns the number of cached files and total domains for display.
func CacheInfo(dataDir string) (files int, proxyDomains int, directDomains int) {
	proxy := CachedDomains(dataDir, "proxy")
	direct := CachedDomains(dataDir, "direct")

	dir := cacheDir(dataDir)
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".domains") {
			files++
		}
	}
	return files, len(proxy), len(direct)
}
