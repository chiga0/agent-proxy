package ecs

import (
	"testing"
)

func TestParseLogLine(t *testing.T) {
	line := "1785301780.091    560 127.0.0.1 TCP_TUNNEL/200 3812 CONNECT chatgpt.com:443 - HIER_DIRECT/172.64.155.209 -"
	e := ParseLogLine(line)
	if e == nil {
		t.Fatal("ParseLogLine returned nil")
	}
	if e.Domain != "chatgpt.com" {
		t.Errorf("Domain = %q, want chatgpt.com", e.Domain)
	}
	if e.Port != 443 {
		t.Errorf("Port = %d, want 443", e.Port)
	}
	if e.Bytes != 3812 {
		t.Errorf("Bytes = %d, want 3812", e.Bytes)
	}
	if e.Method != "CONNECT" {
		t.Errorf("Method = %q, want CONNECT", e.Method)
	}
	if e.HierIP != "172.64.155.209" {
		t.Errorf("HierIP = %q, want 172.64.155.209", e.HierIP)
	}
	if e.Status != "TCP_TUNNEL" {
		t.Errorf("Status = %q, want TCP_TUNNEL", e.Status)
	}
}

func TestParseLogLineGET(t *testing.T) {
	line := "1785301780.091    200 127.0.0.1 TCP_MISS/200 1234 GET http://example.com/path - HIER_DIRECT/93.184.216.34 -"
	e := ParseLogLine(line)
	if e == nil {
		t.Fatal("ParseLogLine returned nil")
	}
	if e.Domain != "example.com" {
		t.Errorf("Domain = %q, want example.com", e.Domain)
	}
	if e.Method != "GET" {
		t.Errorf("Method = %q, want GET", e.Method)
	}
}

func TestParseLogLineMalformed(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{"empty string", ""},
		{"single word", "hello"},
		{"too few fields", "1785301780.091 560 127.0.0.1 TCP_TUNNEL/200"},
		{"nine fields", "1 2 3 4 5 6 7 8 9"},
		{"whitespace only", "   \t  \n  "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if e := ParseLogLine(tt.line); e != nil {
				t.Errorf("ParseLogLine(%q) = %+v, want nil", tt.line, e)
			}
		})
	}
}

func TestParseLogLineCONNECTNoPort(t *testing.T) {
	// CONNECT without explicit port should default to 443
	line := "1785301780.091    560 127.0.0.1 TCP_TUNNEL/200 3812 CONNECT example.com - HIER_DIRECT/1.2.3.4 -"
	e := ParseLogLine(line)
	if e == nil {
		t.Fatal("ParseLogLine returned nil")
	}
	if e.Domain != "example.com" {
		t.Errorf("Domain = %q, want example.com", e.Domain)
	}
	if e.Port != 443 {
		t.Errorf("Port = %d, want 443 (default)", e.Port)
	}
}

func TestParseLogLineCONNECTCustomPort(t *testing.T) {
	line := "1785301780.091    560 127.0.0.1 TCP_TUNNEL/200 3812 CONNECT example.com:8443 - HIER_DIRECT/1.2.3.4 -"
	e := ParseLogLine(line)
	if e == nil {
		t.Fatal("ParseLogLine returned nil")
	}
	if e.Port != 8443 {
		t.Errorf("Port = %d, want 8443", e.Port)
	}
}

func TestParseLogLineGETWithPort(t *testing.T) {
	line := "1785301780.091    200 127.0.0.1 TCP_MISS/200 1234 GET http://example.com:8080/path - HIER_DIRECT/1.2.3.4 -"
	e := ParseLogLine(line)
	if e == nil {
		t.Fatal("ParseLogLine returned nil")
	}
	if e.Domain != "example.com" {
		t.Errorf("Domain = %q, want example.com", e.Domain)
	}
	if e.Port != 8080 {
		t.Errorf("Port = %d, want 8080", e.Port)
	}
}

func TestParseLogLineGETDefaultPort(t *testing.T) {
	line := "1785301780.091    200 127.0.0.1 TCP_MISS/200 1234 GET http://example.com/path - HIER_DIRECT/1.2.3.4 -"
	e := ParseLogLine(line)
	if e == nil {
		t.Fatal("ParseLogLine returned nil")
	}
	if e.Port != 80 {
		t.Errorf("Port = %d, want 80 (default)", e.Port)
	}
}

func TestParseLogLinePOST(t *testing.T) {
	line := "1785301780.091    300 127.0.0.1 TCP_MISS/200 5678 POST http://api.example.com/v1/data - HIER_DIRECT/1.2.3.4 -"
	e := ParseLogLine(line)
	if e == nil {
		t.Fatal("ParseLogLine returned nil")
	}
	if e.Method != "POST" {
		t.Errorf("Method = %q, want POST", e.Method)
	}
	if e.Domain != "api.example.com" {
		t.Errorf("Domain = %q, want api.example.com", e.Domain)
	}
	if e.Bytes != 5678 {
		t.Errorf("Bytes = %d, want 5678", e.Bytes)
	}
}

func TestParseLogLineTimestamp(t *testing.T) {
	line := "1785301780.091    560 127.0.0.1 TCP_TUNNEL/200 3812 CONNECT chatgpt.com:443 - HIER_DIRECT/172.64.155.209 -"
	e := ParseLogLine(line)
	if e == nil {
		t.Fatal("ParseLogLine returned nil")
	}
	if e.Timestamp != 1785301780.091 {
		t.Errorf("Timestamp = %f, want 1785301780.091", e.Timestamp)
	}
	if e.Elapsed != 560 {
		t.Errorf("Elapsed = %d, want 560", e.Elapsed)
	}
	if e.ClientIP != "127.0.0.1" {
		t.Errorf("ClientIP = %q, want 127.0.0.1", e.ClientIP)
	}
}

func TestParseLogLineInvalidNumbers(t *testing.T) {
	// Non-numeric timestamp and bytes should not crash, just zero values
	line := "notanumber    abc 127.0.0.1 TCP_TUNNEL/200 notbytes CONNECT example.com:443 - HIER_DIRECT/1.2.3.4 -"
	e := ParseLogLine(line)
	if e == nil {
		t.Fatal("ParseLogLine returned nil for line with invalid numbers")
	}
	if e.Timestamp != 0 {
		t.Errorf("Timestamp = %f, want 0 for invalid input", e.Timestamp)
	}
	if e.Bytes != 0 {
		t.Errorf("Bytes = %d, want 0 for invalid input", e.Bytes)
	}
}

func TestParseLogLinesEmpty(t *testing.T) {
	entries := ParseLogLines("")
	if len(entries) != 0 {
		t.Errorf("ParseLogLines(\"\") returned %d entries, want 0", len(entries))
	}
}

func TestParseLogLinesMixedValidInvalid(t *testing.T) {
	text := `1785301780.091    560 127.0.0.1 TCP_TUNNEL/200 3812 CONNECT chatgpt.com:443 - HIER_DIRECT/172.64.155.209 -
garbage line
1785301781.091    200 127.0.0.1 TCP_MISS/200 1234 GET http://example.com/path - HIER_DIRECT/93.184.216.34 -

1785301782.091    100 127.0.0.1 TCP_TUNNEL/200 999 CONNECT api.openai.com:443 - HIER_DIRECT/1.2.3.4 -`

	entries := ParseLogLines(text)
	if len(entries) != 3 {
		t.Fatalf("ParseLogLines returned %d entries, want 3 (skipping garbage and empty)", len(entries))
	}
	if entries[0].Domain != "chatgpt.com" {
		t.Errorf("entries[0].Domain = %q", entries[0].Domain)
	}
	if entries[1].Domain != "example.com" {
		t.Errorf("entries[1].Domain = %q", entries[1].Domain)
	}
	if entries[2].Domain != "api.openai.com" {
		t.Errorf("entries[2].Domain = %q", entries[2].Domain)
	}
}

func TestParseLogLinesOnlyInvalid(t *testing.T) {
	text := "short\nalso short\n"
	entries := ParseLogLines(text)
	if len(entries) != 0 {
		t.Errorf("got %d entries, want 0", len(entries))
	}
}

func TestAggregateByDomainEmpty(t *testing.T) {
	stats := AggregateByDomain(nil)
	if len(stats) != 0 {
		t.Errorf("AggregateByDomain(nil) returned %d stats, want 0", len(stats))
	}

	stats = AggregateByDomain([]LogEntry{})
	if len(stats) != 0 {
		t.Errorf("AggregateByDomain([]) returned %d stats, want 0", len(stats))
	}
}

func TestAggregateByDomainSkipsEmptyDomain(t *testing.T) {
	entries := []LogEntry{
		{Domain: "", Bytes: 100},
		{Domain: "a.com", Bytes: 200},
		{Domain: "", Bytes: 300},
	}
	stats := AggregateByDomain(entries)
	if len(stats) != 1 {
		t.Fatalf("got %d domains, want 1 (empty domains skipped)", len(stats))
	}
	if stats[0].Domain != "a.com" {
		t.Errorf("Domain = %q, want a.com", stats[0].Domain)
	}
	if stats[0].Bytes != 200 {
		t.Errorf("Bytes = %d, want 200", stats[0].Bytes)
	}
}

func TestAggregateByDomainSortOrder(t *testing.T) {
	entries := []LogEntry{
		{Domain: "small.com", Bytes: 10},
		{Domain: "large.com", Bytes: 10000},
		{Domain: "medium.com", Bytes: 500},
	}
	stats := AggregateByDomain(entries)
	if len(stats) != 3 {
		t.Fatalf("got %d domains, want 3", len(stats))
	}
	// Should be sorted by bytes descending
	if stats[0].Domain != "large.com" {
		t.Errorf("stats[0] = %q, want large.com", stats[0].Domain)
	}
	if stats[1].Domain != "medium.com" {
		t.Errorf("stats[1] = %q, want medium.com", stats[1].Domain)
	}
	if stats[2].Domain != "small.com" {
		t.Errorf("stats[2] = %q, want small.com", stats[2].Domain)
	}
}

func TestLooksChinese(t *testing.T) {
	tests := []struct {
		domain string
		want   bool
	}{
		{"dataworks-alibaba.cn-wulanchabu.log.aliyuncs.com", true},
		{"xast.aliyun-inc.com", true},
		{"rapt-grpc-inner.aliyunportal.com", true},
		{"chatgpt.com", false},
		{"github.com", false},
		{"api.openai.com", false},
		{"www.baidu.com", true},
		{"csdn.net", true},
	}
	for _, tt := range tests {
		if got := LooksChinese(tt.domain); got != tt.want {
			t.Errorf("LooksChinese(%q) = %v, want %v", tt.domain, got, tt.want)
		}
	}
}

func TestLooksChineseCaseInsensitive(t *testing.T) {
	if !LooksChinese("WWW.BAIDU.COM") {
		t.Error("LooksChinese should be case-insensitive")
	}
	if !LooksChinese("Aliyun.Com") {
		t.Error("LooksChinese should match mixed case")
	}
}

func TestLooksChineseEmpty(t *testing.T) {
	if LooksChinese("") {
		t.Error("LooksChinese(\"\") should be false")
	}
}

func TestAggregateByDomain(t *testing.T) {
	entries := []LogEntry{
		{Domain: "a.com", Bytes: 100},
		{Domain: "b.com", Bytes: 200},
		{Domain: "a.com", Bytes: 150},
	}
	stats := AggregateByDomain(entries)
	if len(stats) != 2 {
		t.Fatalf("got %d domains, want 2", len(stats))
	}
	// Sorted by bytes desc: a.com (250) > b.com (200)
	if stats[0].Domain != "a.com" || stats[0].Bytes != 250 {
		t.Errorf("stats[0] = %+v, want a.com/250", stats[0])
	}
	if stats[0].Requests != 2 {
		t.Errorf("stats[0].Requests = %d, want 2", stats[0].Requests)
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
	}
	for _, tt := range tests {
		if got := FormatBytes(tt.bytes); got != tt.want {
			t.Errorf("FormatBytes(%d) = %q, want %q", tt.bytes, got, tt.want)
		}
	}
}

func TestFormatBytesZero(t *testing.T) {
	if got := FormatBytes(0); got != "0 B" {
		t.Errorf("FormatBytes(0) = %q, want \"0 B\"", got)
	}
}

func TestFormatBytesLargeGB(t *testing.T) {
	// 5.5 GB
	b := int64(5.5 * 1024 * 1024 * 1024)
	got := FormatBytes(b)
	if got != "5.5 GB" {
		t.Errorf("FormatBytes(%d) = %q, want \"5.5 GB\"", b, got)
	}
}

func TestExtractDomainPortHTTPS(t *testing.T) {
	// https:// URL form (less common in Squid logs but should work)
	domain, port := extractDomainPort("https://secure.example.com/path", "GET")
	if domain != "secure.example.com" {
		t.Errorf("domain = %q, want secure.example.com", domain)
	}
	if port != 80 {
		t.Errorf("port = %d, want 80 (default for non-CONNECT)", port)
	}
}

func TestExtractDomainPortCONNECTInvalidPort(t *testing.T) {
	// CONNECT with non-numeric port should default to 443
	domain, port := extractDomainPort("example.com:notaport", "CONNECT")
	if domain != "example.com" {
		t.Errorf("domain = %q, want example.com", domain)
	}
	if port != 443 {
		t.Errorf("port = %d, want 443 (default for invalid port)", port)
	}
}
