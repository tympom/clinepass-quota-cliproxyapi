package plugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"

	"clinepass-quota-cliproxyapi/internal/config"
	"clinepass-quota-cliproxyapi/resources"
)

// usageLimitsPath is the official ClinePass usage-limits endpoint (verified
// against Cline's own CLI/dashboard integration): GET returns the
// subscriber's five_hour/weekly/monthly quota windows for the caller's
// account, identified by the bearer API key.
const usageLimitsPath = "/api/v1/users/me/plan/usage-limits"

type quotaWindow struct {
	Status   string `json:"status"`
	Percent  int    `json:"percent"`
	ResetsAt string `json:"resets_at"`
}

type quotaUsage struct {
	Rolling quotaWindow `json:"rolling"` // maps upstream "five_hour"
	Weekly  quotaWindow `json:"weekly"`
	Monthly quotaWindow `json:"monthly"`
}

type quotaCard struct {
	KeyID string      `json:"key_id"`
	Label string      `json:"label"`
	Usage *quotaUsage `json:"usage,omitempty"`
}

type quotaRequest struct {
	KeyID string `json:"key_id"`
}

type quotaList struct {
	Cards []quotaCard `json:"cards"`
}

// upstreamLimitsResponse decodes ClinePass's usage-limits payload:
// {"success":true,"data":{"limits":[{"type":"five_hour","percentUsed":12.3,"resetsAt":"..."}]}}
type upstreamLimitsResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Limits []upstreamLimit `json:"limits"`
	} `json:"data"`
}

type upstreamLimit struct {
	Type        string  `json:"type"`
	PercentUsed float64 `json:"percentUsed"`
	ResetsAt    string  `json:"resetsAt"`
}

func quotaKeyID(key string) string {
	digest := sha256.Sum256([]byte(key))
	return "clinepass-key-" + hex.EncodeToString(digest[:])
}

// defaultLabel identifies a credential by the last few characters of its
// actual key value (masking the rest) instead of an opaque content hash, so
// a tile is recognizable at a glance against the key you actually configured
// — the same masking convention as Stripe/GitHub token displays. Safe here
// because the Management Center is already authenticated before this page
// is reachable.
func defaultLabel(key string) string {
	suffix := key
	if len(key) > 4 {
		suffix = key[len(key)-4:]
	}
	return "ClinePass ••••" + suffix
}

// HandleManagement serves the embedded quota page under
// /v0/resource/plugins/<id>/quota and the quota-usage JSON API under
// /v0/management/plugins/<id>/quota-usage.
func (m *Manager) HandleManagement(ctx context.Context, req pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
	if req.Method == http.MethodGet && req.Path == "/v0/resource/plugins/"+pluginName+"/quota" {
		return pluginapi.ManagementResponse{Headers: http.Header{"Content-Type": []string{"text/html; charset=utf-8"}}, Body: []byte(resources.QuotaPage)}, nil
	}
	if req.Method != http.MethodPost || req.Path != "/v0/management/plugins/"+pluginName+"/quota-usage" {
		return pluginapi.ManagementResponse{StatusCode: http.StatusNotFound, Body: []byte(`{"error":"not found"}`)}, nil
	}
	var body quotaRequest
	if len(req.Body) > 0 {
		if err := json.Unmarshal(req.Body, &body); err != nil {
			return pluginapi.ManagementResponse{StatusCode: http.StatusBadRequest, Body: []byte(`{"error":"invalid request"}`)}, nil
		}
	}
	cfg := m.snapshot()
	if body.KeyID == "" {
		cards := make([]quotaCard, 0, len(cfg.APIKeys))
		for _, key := range cfg.APIKeys {
			label := defaultLabel(key.Value)
			if key.Label != "" {
				label = key.Label
			}
			cards = append(cards, quotaCard{KeyID: quotaKeyID(key.Value), Label: label})
		}
		return quotaJSON(quotaList{Cards: cards})
	}
	for _, key := range cfg.APIKeys {
		id := quotaKeyID(key.Value)
		if id != body.KeyID {
			continue
		}
		label := defaultLabel(key.Value)
		if key.Label != "" {
			label = key.Label
		}
		usage, err := m.fetchQuota(ctx, cfg, key.Value)
		if err != nil {
			return pluginapi.ManagementResponse{StatusCode: http.StatusBadGateway, Body: []byte(`{"error":"quota refresh failed"}`)}, nil
		}
		return quotaJSON(quotaCard{KeyID: id, Label: label, Usage: &usage})
	}
	return pluginapi.ManagementResponse{StatusCode: http.StatusNotFound, Body: []byte(`{"error":"unknown quota key"}`)}, nil
}

func quotaJSON(v any) (pluginapi.ManagementResponse, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return pluginapi.ManagementResponse{}, fmt.Errorf("quota response encoding failed")
	}
	return pluginapi.ManagementResponse{Headers: http.Header{"Content-Type": []string{"application/json"}}, Body: body}, nil
}

func (m *Manager) fetchQuota(ctx context.Context, cfg config.Config, key string) (quotaUsage, error) {
	if m.bridge == nil {
		return quotaUsage{}, fmt.Errorf("quota bridge unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, cfg.RequestTimeout)
	defer cancel()
	resp, err := m.bridge.Do(ctx, pluginapi.HTTPRequest{
		Method: http.MethodGet,
		URL:    strings.TrimRight(cfg.BaseURL, "/") + usageLimitsPath,
		Headers: http.Header{
			"Authorization": []string{"Bearer " + key},
			"Accept":        []string{"application/json"},
		},
	})
	if err != nil || resp.StatusCode != http.StatusOK {
		return quotaUsage{}, fmt.Errorf("quota upstream request failed")
	}
	var decoded upstreamLimitsResponse
	if err := json.Unmarshal(resp.Body, &decoded); err != nil || !decoded.Success {
		return quotaUsage{}, fmt.Errorf("quota response invalid")
	}
	var usage quotaUsage
	for _, limit := range decoded.Data.Limits {
		window := quotaWindow{
			Status:   statusFor(limit.PercentUsed),
			Percent:  clampPercent(limit.PercentUsed),
			ResetsAt: limit.ResetsAt,
		}
		switch limit.Type {
		case "five_hour":
			usage.Rolling = window
		case "weekly":
			usage.Weekly = window
		case "monthly":
			usage.Monthly = window
		}
	}
	return usage, nil
}

func statusFor(percentUsed float64) string {
	switch {
	case percentUsed >= 90:
		return "critical"
	case percentUsed >= 70:
		return "warning"
	default:
		return "ok"
	}
}

func clampPercent(v float64) int {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return int(v)
}
