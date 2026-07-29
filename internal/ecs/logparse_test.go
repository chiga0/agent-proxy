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
