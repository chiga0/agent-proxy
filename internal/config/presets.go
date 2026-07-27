package config

// Preset domain groups — enabled by default for zero-config experience.
var Presets = map[string]PresetInfo{
	"ai": {
		Description: "AI services (OpenAI, Anthropic, Google AI, OpenRouter, Copilot)",
		Domains: []string{
			// OpenAI / ChatGPT
			"chatgpt.com",
			"openai.com",
			"oaistatic.com",
			"oaiusercontent.com",
			// Anthropic / Claude
			"anthropic.com",
			"claude.ai",
			// Google AI / Gemini
			"gemini.google.com",
			"googleapis.com",
			"generativelanguage.googleapis.com",
			"ai.google.dev",
			// OpenRouter
			"openrouter.ai",
			// GitHub Copilot
			"copilot.github.com",
			// Mistral
			"mistral.ai",
			// Perplexity
			"perplexity.ai",
		},
	},
	"dev": {
		Description: "Developer tools (GitHub, StackOverflow, package registries)",
		Domains: []string{
			// GitHub
			"github.com",
			"githubusercontent.com",
			"github.io",
			"githubassets.com",
			// Stack Overflow
			"stackoverflow.com",
			"stackexchange.com",
			// Package registries
			"npmjs.com",
			"registry.npmjs.org",
			"pypi.org",
			"files.pythonhosted.org",
			"crates.io",
			"rubygems.org",
			// Go
			"go.dev",
			"pkg.go.dev",
			"proxy.golang.org",
			// Rust
			"docs.rs",
			// Docker
			"hub.docker.com",
			"docker.io",
			// Hugging Face
			"huggingface.co",
		},
	},
	"search": {
		Description: "Search engines and knowledge bases",
		Domains: []string{
			"google.com",
			"www.google.com",
			"duckduckgo.com",
			"bing.com",
			"wikipedia.org",
			"en.wikipedia.org",
		},
	},
	"cloud": {
		Description: "Cloud provider docs and consoles (AWS, GCP, Azure)",
		Domains: []string{
			"aws.amazon.com",
			"docs.aws.amazon.com",
			"console.aws.amazon.com",
			"cloud.google.com",
			"console.cloud.google.com",
			"cloud.console.aliyun.com",
			"portal.azure.com",
			"learn.microsoft.com",
		},
	},
}

// PresetOrder defines the display order.
var PresetOrder = []string{"ai", "dev", "search", "cloud"}

type PresetInfo struct {
	Description string
	Domains     []string
}

// DefaultPresets returns all preset names (all enabled by default).
func DefaultPresets() []string {
	return append([]string{}, PresetOrder...)
}

// PresetDomains returns the union of domains from the given preset names.
func PresetDomains(names []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, name := range names {
		info, ok := Presets[name]
		if !ok {
			continue
		}
		for _, d := range info.Domains {
			if !seen[d] {
				seen[d] = true
				result = append(result, d)
			}
		}
	}
	return result
}
