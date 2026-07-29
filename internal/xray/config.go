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

	conf := map[string]interface{}{
		"log": map[string]interface{}{
			"loglevel": "warning",
		},
		"inbounds": []map[string]interface{}{
			{
				"listen": "127.0.0.1",
				"port":   localPort,
				"protocol": "http",
				"settings": map[string]interface{}{
					"timeout": 300,
				},
			},
		},
		"outbounds": []map[string]interface{}{
			{
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
									"flow":       "xtls-rprx-vision",
								},
							},
						},
					},
				},
				"streamSettings": map[string]interface{}{
					"network":  "tcp",
					"security": "reality",
					"realitySettings": map[string]interface{}{
						"serverName": cfg.Proxy.XrayServerName(),
						"publicKey":  x.PublicKey,
						"shortId":    x.ShortID,
						"fingerprint": "chrome",
					},
				},
				"mux": map[string]interface{}{
					"enabled":     mux,
					"concurrency": 8,
				},
			},
		},
	}

	return json.MarshalIndent(conf, "", "  ")
}

// ServerConfig generates the remote xray server config (VLESS inbound → freedom outbound).
func ServerConfig(cfg *config.Config) ([]byte, error) {
	x := cfg.Proxy.Xray

	conf := map[string]interface{}{
		"log": map[string]interface{}{
			"loglevel": "warning",
		},
		"inbounds": []map[string]interface{}{
			{
				"listen": "0.0.0.0",
				"port":   cfg.Proxy.Port,
				"protocol": "vless",
				"settings": map[string]interface{}{
					"clients": []map[string]interface{}{
						{
							"id":   x.UUID,
							"flow": "xtls-rprx-vision",
						},
					},
					"decryption": "none",
				},
				"streamSettings": map[string]interface{}{
					"network":  "tcp",
					"security": "reality",
					"realitySettings": map[string]interface{}{
						"dest":       fmt.Sprintf("%s:443", cfg.Proxy.XrayServerName()),
						"serverNames": []string{cfg.Proxy.XrayServerName()},
						"privateKey": x.PrivateKey,
						"shortIds":   []string{x.ShortID},
					},
				},
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
