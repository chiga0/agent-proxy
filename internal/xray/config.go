package xray

import (
	"encoding/json"
	"fmt"

	"github.com/chiga0/agent-proxy/internal/config"
)

// ClientConfig generates the local xray client config (HTTP inbound → VLESS outbound).
func ClientConfig(cfg *config.Config) ([]byte, error) {
	x := cfg.Proxy.Xray
	localPort := cfg.Proxy.LocalPort()
	mux := cfg.Proxy.XrayMuxEnabled()

	tlsSettings := map[string]interface{}{
		"serverName": cfg.Proxy.XrayServerName(),
	}

	// Reality or TLS based on config
	if x.PublicKey != "" {
		tlsSettings["publicKey"] = x.PublicKey
		tlsSettings["shortId"] = x.ShortID
		tlsSettings["fingerprint"] = "chrome"
	}
	if x.CertSha256 != "" {
		tlsSettings["pinnedPeerCertSha256"] = x.CertSha256
	}

	security := "tls"
	if x.PublicKey != "" {
		security = "reality"
	}

	outbound := map[string]interface{}{
		"protocol": "vless",
		"settings": map[string]interface{}{
			"vnext": []map[string]interface{}{
				{
					"address": cfg.Proxy.Host,
					"port":    cfg.Proxy.Port,
					"users": []map[string]interface{}{
						{
							"id":         x.UUID,
							"encryption": "none",
						},
					},
				},
			},
		},
		"streamSettings": map[string]interface{}{
			"network":  "tcp",
			"security": security,
			security + "Settings": tlsSettings,
		},
	}

	if mux {
		outbound["mux"] = map[string]interface{}{
			"enabled":     true,
			"concurrency": 8,
		}
	}

	conf := map[string]interface{}{
		"log": map[string]interface{}{
			"loglevel": "warning",
		},
		"inbounds": []map[string]interface{}{
			{
				"listen":   "127.0.0.1",
				"port":     localPort,
				"protocol": "http",
				"settings": map[string]interface{}{
					"timeout": 300,
				},
			},
		},
		"outbounds": []map[string]interface{}{outbound},
	}

	return json.MarshalIndent(conf, "", "  ")
}

// ServerConfig generates the remote xray server config (VLESS inbound → freedom outbound).
func ServerConfig(cfg *config.Config) ([]byte, error) {
	x := cfg.Proxy.Xray

	streamSettings := map[string]interface{}{
		"network": "tcp",
	}

	if x.PublicKey != "" && x.PrivateKey != "" {
		// Reality mode
		streamSettings["security"] = "reality"
		streamSettings["realitySettings"] = map[string]interface{}{
			"dest":        fmt.Sprintf("%s:443", cfg.Proxy.XrayServerName()),
			"serverNames": []string{cfg.Proxy.XrayServerName()},
			"privateKey":  x.PrivateKey,
			"shortIds":    []string{x.ShortID},
		}
	} else {
		// TLS mode with cert files
		streamSettings["security"] = "tls"
		certFile := x.CertFile
		keyFile := x.KeyFile
		if certFile == "" {
			certFile = "/usr/local/etc/xray/cert/cert.pem"
		}
		if keyFile == "" {
			keyFile = "/usr/local/etc/xray/cert/key.pem"
		}
		streamSettings["tlsSettings"] = map[string]interface{}{
			"certificates": []map[string]interface{}{
				{
					"certificateFile": certFile,
					"keyFile":         keyFile,
				},
			},
		}
	}

	conf := map[string]interface{}{
		"log": map[string]interface{}{
			"loglevel": "warning",
		},
		"inbounds": []map[string]interface{}{
			{
				"listen":   "0.0.0.0",
				"port":     cfg.Proxy.Port,
				"protocol": "vless",
				"settings": map[string]interface{}{
					"clients": []map[string]interface{}{
						{
							"id": x.UUID,
						},
					},
					"decryption": "none",
				},
				"streamSettings": streamSettings,
			},
		},
		"outbounds": []map[string]interface{}{
			{
				"protocol": "freedom",
				"settings": map[string]interface{}{},
			},
		},
	}

	return json.MarshalIndent(conf, "", "  ")
}
