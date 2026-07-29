package ecs

import (
	"bufio"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// LogEntry represents a parsed Squid access log line.
type LogEntry struct {
	Timestamp float64
	Elapsed   int
	ClientIP  string
	Status    string
	Bytes     int64
	Method    string
	Domain    string
	Port      int
	HierIP    string
}

// DomainStat aggregates traffic per domain.
type DomainStat struct {
	Domain   string
	Requests int
	Bytes    int64
}

// ParseLogLine parses a single Squid access log line.
// Format: timestamp elapsed client_ip status/code bytes method url - hier/ip -
func ParseLogLine(line string) *LogEntry {
	fields := strings.Fields(line)
	if len(fields) < 10 {
		return nil
	}

	ts, _ := strconv.ParseFloat(fields[0], 64)
	elapsed, _ := strconv.Atoi(fields[1])

	// fields[3] = status/code (e.g. TCP_TUNNEL/200)
	status := strings.SplitN(fields[3], "/", 2)[0]

	// fields[4] = bytes
	bytes, _ := strconv.ParseInt(fields[4], 10, 64)

	// fields[5] = method (CONNECT, GET, etc.)
	method := fields[5]

	// fields[6] = url (CONNECT domain:port or GET http://domain/path)
	url := fields[6]
	domain, port := extractDomainPort(url, method)

	// fields[8] = HIER_DIRECT/ip or HIER_NONE/ip
	hierIP := ""
	if len(fields) > 8 {
		hier := fields[8]
		if idx := strings.LastIndex(hier, "/"); idx >= 0 {
			hierIP = hier[idx+1:]
		}
	}

	return &LogEntry{
		Timestamp: ts,
		Elapsed:   elapsed,
		ClientIP:  fields[2],
		Status:    status,
		Bytes:     bytes,
		Method:    method,
		Domain:    domain,
		Port:      port,
		HierIP:    hierIP,
	}
}

func extractDomainPort(url, method string) (string, int) {
	if method == "CONNECT" {
		// CONNECT domain:port
		parts := strings.SplitN(url, ":", 2)
		port := 443
		if len(parts) == 2 {
			if p, err := strconv.Atoi(parts[1]); err == nil {
				port = p
			}
		}
		return parts[0], port
	}
	// GET/POST http://domain:port/path
	url = strings.TrimPrefix(url, "http://")
	url = strings.TrimPrefix(url, "https://")
	slashIdx := strings.Index(url, "/")
	if slashIdx >= 0 {
		url = url[:slashIdx]
	}
	parts := strings.SplitN(url, ":", 2)
	port := 80
	if len(parts) == 2 {
		if p, err := strconv.Atoi(parts[1]); err == nil {
			port = p
		}
	}
	return parts[0], port
}

// ParseLogLines parses multiple log lines.
func ParseLogLines(text string) []LogEntry {
	var entries []LogEntry
	scanner := bufio.NewScanner(strings.NewReader(text))
	for scanner.Scan() {
		if e := ParseLogLine(scanner.Text()); e != nil {
			entries = append(entries, *e)
		}
	}
	return entries
}

// AggregateByDomain groups entries by domain and sums bytes/requests.
func AggregateByDomain(entries []LogEntry) []DomainStat {
	m := make(map[string]*DomainStat)
	for _, e := range entries {
		if e.Domain == "" {
			continue
		}
		s, ok := m[e.Domain]
		if !ok {
			s = &DomainStat{Domain: e.Domain}
			m[e.Domain] = s
		}
		s.Requests++
		s.Bytes += e.Bytes
	}
	var stats []DomainStat
	for _, s := range m {
		stats = append(stats, *s)
	}
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].Bytes > stats[j].Bytes
	})
	return stats
}

// chineseDomainPatterns are domain patterns that typically indicate Chinese services
// that should be in no_proxy for users in China.
var chineseDomainPatterns = []string{
	".cn", ".com.cn", ".net.cn", ".org.cn",
	"aliyun", "alibaba", "taobao", "tmall", "1688",
	"baidu", "tencent", "qq.com", "weixin",
	"huawei", "bytedance", "douyin", "xiaomi",
	"jd.com", "sina", "weibo", "zhihu",
	"bilibili", "csdn", "gitee", "coding.net",
	"dingtalk", "feishu", "lark",
	"aliyuncs", "myqcloud", "huaweicloud",
}

// LooksChinese returns true if the domain matches known Chinese service patterns.
func LooksChinese(domain string) bool {
	d := strings.ToLower(domain)
	for _, p := range chineseDomainPatterns {
		if strings.Contains(d, p) {
			return true
		}
	}
	return false
}

// FormatBytes formats bytes into human-readable string.
func FormatBytes(b int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)
	switch {
	case b >= GB:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(GB))
	case b >= MB:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(MB))
	case b >= KB:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(KB))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
