// Package config loads and validates the clinepass-quota-cliproxyapi plugin
// configuration: the ClinePass API key(s) to poll and the upstream base URL.
package config

import (
	"fmt"
	"net/url"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Defaults.
const (
	DefaultBaseURL        = "https://api.cline.bot"
	DefaultRequestTimeout = 15 * time.Second
)

type APIKey struct {
	Value string
	Label string
}

type Config struct {
	BaseURL        string
	APIKeys        []APIKey
	AllowHTTP      bool
	RequestTimeout time.Duration
}

// rawConfig mirrors the YAML shape under plugins.configs.clinepass-quota-cliproxyapi.
type rawConfig struct {
	BaseURL        *string  `yaml:"base-url"`
	APIKeys        []rawKey `yaml:"api-keys"`
	AllowHTTP      bool     `yaml:"allow-http"`
	RequestTimeout *string  `yaml:"request-timeout"`
}

type rawKey struct {
	Value string `yaml:"value"`
	Label string `yaml:"label"`
}

// UnmarshalYAML accepts a key as a bare string ("sk-...") or as a
// {value, label} mapping, so the Management Center editor takes ["sk-..."].
// Errors carry no node values (see Load).
func (k *rawKey) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		k.Value = n.Value
		return nil
	case yaml.MappingNode:
		type plain rawKey
		return n.Decode((*plain)(k))
	default:
		return fmt.Errorf("api-keys: entry must be a string or a mapping")
	}
}

// Load decodes YAML, expands ${VAR} references in api-key values, applies
// defaults, and validates. Decode errors never echo decoded node values — a
// malformed entry (e.g. a nested list holding an API key) must not leak into the
// invalid_config envelope the host logs.
func Load(yamlBytes []byte) (Config, error) {
	var raw rawConfig
	if err := yaml.Unmarshal(yamlBytes, &raw); err != nil {
		return Config{}, fmt.Errorf("decode config: invalid YAML structure")
	}
	keys := make([]APIKey, len(raw.APIKeys))
	for i, k := range raw.APIKeys {
		keys[i] = APIKey{Value: os.ExpandEnv(k.Value), Label: k.Label}
	}
	requestTimeout, err := parseDuration("request-timeout", raw.RequestTimeout, DefaultRequestTimeout)
	if err != nil {
		return Config{}, err
	}
	if requestTimeout <= 0 {
		return Config{}, fmt.Errorf("request-timeout: must be positive")
	}
	c := Config{
		BaseURL:        orDefault(raw.BaseURL, DefaultBaseURL),
		APIKeys:        keys,
		AllowHTTP:      raw.AllowHTTP,
		RequestTimeout: requestTimeout,
	}
	if err := c.validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

func (c Config) validate() error {
	if err := validateURL("base-url", c.BaseURL, c.AllowHTTP); err != nil {
		return err
	}
	for i, k := range c.APIKeys {
		if k.Value == "" {
			return fmt.Errorf("api-keys[%d].value: expanded to empty", i)
		}
	}
	seen := make(map[string]bool, len(c.APIKeys))
	for _, k := range c.APIKeys {
		if seen[k.Value] {
			return fmt.Errorf("api-keys: duplicate key values are not allowed")
		}
		seen[k.Value] = true
	}
	return nil
}

func validateURL(name, raw string, allowHTTP bool) error {
	if raw == "" {
		return fmt.Errorf("%s: must not be empty", name)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%s: invalid URL", name)
	}
	if u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("%s: invalid URL %s://%s", name, u.Scheme, u.Host)
	}
	if u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return fmt.Errorf("%s: must not contain query, fragment, or userinfo", name)
	}
	switch u.Scheme {
	case "https":
	case "http":
		if !allowHTTP {
			return fmt.Errorf("%s: http scheme requires allow-http", name)
		}
	default:
		return fmt.Errorf("%s: unsupported scheme %q; use https", name, u.Scheme)
	}
	return nil
}

func parseDuration(name string, p *string, def time.Duration) (time.Duration, error) {
	if p == nil {
		return def, nil
	}
	d, err := time.ParseDuration(*p)
	if err != nil {
		return 0, fmt.Errorf("%s: invalid duration", name)
	}
	return d, nil
}

func orDefault(p *string, def string) string {
	if p != nil {
		return *p
	}
	return def
}
