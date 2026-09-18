package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"

	"clinepass-quota-cliproxyapi/internal/config"
)

// pluginName / pluginVersion are reported in registration metadata and used
// as the Management API/resource route prefix.
const (
	pluginName    = "clinepass-quota-cliproxyapi"
	pluginVersion = "0.1.0"
)

// githubRepoURL satisfies the host's validPlugin gate (Metadata.GitHubRepository
// must be non-empty). This plugin is locally built and not published.
const githubRepoURL = "https://github.com/local/clinepass-quota-cliproxyapi"

// Manager owns dispatcher state (the current config snapshot) and routes
// every RPC method. Safe for concurrent HandleCall use.
type Manager struct {
	bridge *HostBridge // immutable after NewManager

	mu  sync.RWMutex
	cfg config.Config
}

// NewManager returns a dispatcher whose outbound traffic flows through bridge.
func NewManager(bridge *HostBridge) *Manager {
	return &Manager{bridge: bridge}
}

// HandleCall dispatches one RPC method and returns envelope bytes. Handler
// failures travel inside the envelope; a recovered panic becomes a
// "plugin_error" envelope so the host process never dies with us.
func (m *Manager) HandleCall(method string, request []byte) (resp []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			resp = ErrEnvelope("plugin_error", fmt.Sprintf("internal error handling %s", method))
			err = nil
		}
	}()
	switch method {
	case pluginabi.MethodPluginRegister, pluginabi.MethodPluginReconfigure:
		return m.handleLifecycle(request)
	case pluginabi.MethodPluginShutdown:
		return okEnvelope(struct{}{}), nil
	case pluginabi.MethodManagementRegister:
		return m.registerManagement(request)
	case pluginabi.MethodManagementHandle:
		return m.handleManagementCall(request)
	default:
		return ErrEnvelope("unknown_method", "unknown method: "+method), nil
	}
}

type capabilities struct {
	ManagementAPI bool `json:"management_api"`
}

type registrationResult struct {
	SchemaVersion uint32             `json:"schema_version"`
	Metadata      pluginapi.Metadata `json:"metadata"`
	Capabilities  capabilities       `json:"capabilities"`
}

func registrationEnvelope() []byte {
	return okEnvelope(registrationResult{
		SchemaVersion: pluginabi.SchemaVersion,
		Metadata: pluginapi.Metadata{
			Name:             pluginName,
			Version:          pluginVersion,
			Author:           pluginName,
			GitHubRepository: githubRepoURL,
			ConfigFields: []pluginapi.ConfigField{
				{Name: "base-url", Type: pluginapi.ConfigFieldTypeString, Description: "ClinePass API base URL (default https://api.cline.bot)."},
				{Name: "api-keys", Type: pluginapi.ConfigFieldTypeArray, Description: "ClinePass API keys to poll for usage-limit windows."},
				{Name: "request-timeout", Type: pluginapi.ConfigFieldTypeString, Description: "Upstream request timeout (default 15s)."},
			},
		},
		Capabilities: capabilities{ManagementAPI: true},
	})
}

// lifecycleRequest mirrors the host's rpcLifecycleRequest: config_yaml is
// base64-decoded to raw bytes by the JSON layer automatically ([]byte).
type lifecycleRequest struct {
	ConfigYAML []byte `json:"config_yaml"`
}

// handleLifecycle implements plugin.register / plugin.reconfigure: parse and
// validate config, then publish it for the management handler to read.
func (m *Manager) handleLifecycle(request []byte) ([]byte, error) {
	var req lifecycleRequest
	if err := json.Unmarshal(request, &req); err != nil {
		return ErrEnvelope("invalid_request", "malformed lifecycle request body"), nil
	}
	cfg, err := config.Load(req.ConfigYAML)
	if err != nil {
		return ErrEnvelope("invalid_config", err.Error()), nil
	}
	m.mu.Lock()
	m.cfg = cfg
	m.mu.Unlock()
	return registrationEnvelope(), nil
}

func (m *Manager) registerManagement(request []byte) ([]byte, error) {
	var req struct {
		Plugin           pluginapi.Metadata `json:"Plugin"`
		BasePath         string             `json:"BasePath"`
		ResourceBasePath string             `json:"ResourceBasePath"`
	}
	if err := json.Unmarshal(request, &req); err != nil {
		return ErrEnvelope("invalid_request", "malformed management registration request body"), nil
	}
	return okEnvelope(struct {
		Routes []struct {
			Method string `json:"method"`
			Path   string `json:"path"`
		} `json:"routes"`
		Resources []struct {
			Path        string `json:"path"`
			Menu        string `json:"menu"`
			Description string `json:"description"`
		} `json:"resources"`
	}{
		Routes: []struct {
			Method string `json:"method"`
			Path   string `json:"path"`
		}{{Method: "POST", Path: "/plugins/" + pluginName + "/quota-usage"}},
		Resources: []struct {
			Path        string `json:"path"`
			Menu        string `json:"menu"`
			Description string `json:"description"`
		}{{Path: "/quota", Menu: "ClinePass Quota", Description: "View ClinePass 5-hour, weekly, and monthly usage windows."}},
	}), nil
}

func (m *Manager) handleManagementCall(request []byte) ([]byte, error) {
	var req struct {
		pluginapi.ManagementRequest
	}
	if err := json.Unmarshal(request, &req); err != nil {
		return ErrEnvelope("invalid_request", "malformed management request body"), nil
	}
	resp, err := m.HandleManagement(context.Background(), req.ManagementRequest)
	if err != nil {
		return ErrEnvelope("management_failure", err.Error()), nil
	}
	return okEnvelope(resp), nil
}

func (m *Manager) snapshot() config.Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg
}

// okEnvelope / ErrEnvelope build pluginabi.Envelope JSON without importing
// pluginabi's Envelope struct at every call site.
func okEnvelope(v any) []byte {
	body, err := json.Marshal(v)
	if err != nil {
		return ErrEnvelope("plugin_error", "response encoding failed")
	}
	env := struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result,omitempty"`
	}{OK: true, Result: body}
	out, err := json.Marshal(env)
	if err != nil {
		return ErrEnvelope("plugin_error", "response encoding failed")
	}
	return out
}

// ErrEnvelope builds a pluginabi-shaped error envelope. Exported for main.go.
func ErrEnvelope(code, message string) []byte {
	env := struct {
		OK    bool `json:"ok"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}{OK: false}
	env.Error.Code = code
	env.Error.Message = message
	out, _ := json.Marshal(env)
	return out
}
