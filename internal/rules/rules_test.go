package rules

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestFetchAndCache(t *testing.T) {
	// Serve a test domain list
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("# comment\ngithub.com\nopenai.com\n\nfull:anthropic.com\ndomain:claude.ai\n"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	sources := map[string]string{srv.URL: "proxy"}

	ok, err := FetchAll(srv.Client(), dir, sources)
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}
	if ok != 1 {
		t.Fatalf("expected 1 fetched, got %d", ok)
	}

	domains := CachedDomains(dir, "proxy")
	expected := map[string]bool{
		"github.com": true, "openai.com": true,
		"anthropic.com": true, "claude.ai": true,
	}
	if len(domains) != len(expected) {
		t.Fatalf("expected %d domains, got %d: %v", len(expected), len(domains), domains)
	}
	for _, d := range domains {
		if !expected[d] {
			t.Errorf("unexpected domain %q", d)
		}
	}
}

func TestCachedDomainsEmpty(t *testing.T) {
	dir := t.TempDir()
	domains := CachedDomains(dir, "proxy")
	if len(domains) != 0 {
		t.Errorf("expected 0 domains from empty cache, got %d", len(domains))
	}
}

func TestCachedDomainsByAction(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(cacheDir(dir), 0700)

	writeCache(filepath.Join(cacheDir(dir), "proxy_abc.domains"), []string{"a.com", "b.com"})
	writeCache(filepath.Join(cacheDir(dir), "direct_def.domains"), []string{"c.com"})

	proxy := CachedDomains(dir, "proxy")
	direct := CachedDomains(dir, "direct")

	if len(proxy) != 2 {
		t.Errorf("expected 2 proxy domains, got %d", len(proxy))
	}
	if len(direct) != 1 {
		t.Errorf("expected 1 direct domain, got %d", len(direct))
	}
}

func TestFetchHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	defer srv.Close()

	dir := t.TempDir()
	ok, err := FetchAll(srv.Client(), dir, map[string]string{srv.URL: "proxy"})
	if ok != 0 {
		t.Errorf("expected 0 fetched, got %d", ok)
	}
	if err == nil {
		t.Error("expected error for 404 response")
	}
}

func TestCacheInfo(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(cacheDir(dir), 0700)
	writeCache(filepath.Join(cacheDir(dir), "proxy_a.domains"), []string{"x.com", "y.com"})
	writeCache(filepath.Join(cacheDir(dir), "direct_b.domains"), []string{"z.com"})

	files, proxyN, directN := CacheInfo(dir)
	if files != 2 {
		t.Errorf("expected 2 files, got %d", files)
	}
	if proxyN != 2 {
		t.Errorf("expected 2 proxy domains, got %d", proxyN)
	}
	if directN != 1 {
		t.Errorf("expected 1 direct domain, got %d", directN)
	}
}
