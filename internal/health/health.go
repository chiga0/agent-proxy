package health

import (
	"log"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/chiga0/agent-proxy/internal/config"
	"github.com/chiga0/agent-proxy/internal/tunnel"
)

const (
	checkInterval   = 30 * time.Second
	failureThreshold = 3
	probeURL        = "http://www.google.com/generate_204"
	probeTimeout    = 10 * time.Second
)

// State exposes health status for metrics/dashboard.
var (
	consecutiveFailures atomic.Int64
	lastCheckOK         atomic.Bool
	lastRecovery        atomic.Int64 // unix timestamp of last recovery, 0 = never
)

func ConsecutiveFailures() int64 { return consecutiveFailures.Load() }
func LastCheckOK() bool          { return lastCheckOK.Load() }
func LastRecovery() int64        { return lastRecovery.Load() }

// Watch runs the health check loop. Blocking — call in a goroutine.
// Only active when tunnel mode is enabled.
func Watch() {
	for {
		time.Sleep(checkInterval)

		cfg, err := config.Load()
		if err != nil || !cfg.Proxy.Tunnel {
			continue
		}

		if probe(cfg) {
			if consecutiveFailures.Load() > 0 {
				log.Printf("[health] proxy recovered after %d consecutive failures", consecutiveFailures.Load())
			}
			consecutiveFailures.Store(0)
			lastCheckOK.Store(true)
			continue
		}

		n := consecutiveFailures.Add(1)
		lastCheckOK.Store(false)
		log.Printf("[health] proxy check failed (%d/%d consecutive)", n, failureThreshold)

		if n >= failureThreshold {
			recover(cfg)
		}
	}
}

func probe(cfg *config.Config) bool {
	proxyURL, err := url.Parse(cfg.ProxyURL())
	if err != nil {
		return false
	}
	client := &http.Client{
		Timeout:   probeTimeout,
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
	}
	resp, err := client.Get(probeURL)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 204
}

func recover(cfg *config.Config) {
	log.Printf("[health] attempting tunnel restart...")

	tunnel.Stop(cfg)
	time.Sleep(time.Second)

	started, err := tunnel.Start(cfg)
	if err != nil {
		log.Printf("[health] tunnel restart failed: %v", err)
		return
	}

	if started {
		log.Printf("[health] tunnel restarted successfully")
	} else {
		log.Printf("[health] tunnel was already running")
	}
	consecutiveFailures.Store(0)
	lastRecovery.Store(time.Now().Unix())

	// Verify the recovery actually fixed things
	time.Sleep(2 * time.Second)
	if probe(cfg) {
		log.Printf("[health] post-recovery check passed")
	} else {
		log.Printf("[health] post-recovery check still failing — may need manual intervention")
	}
}
