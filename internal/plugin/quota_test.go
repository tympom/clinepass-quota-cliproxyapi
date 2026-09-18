package plugin

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"

	"clinepass-quota-cliproxyapi/internal/config"
)

// fakeCall simulates the host's host.http.do callback: it decodes the
// hostHTTPReq the bridge sent and returns a canned ClinePass usage-limits
// response, exactly like the CLIProxyAPI host would after performing the
// real upstream HTTP round trip.
func fakeCall(t *testing.T, wantAuth string) RawCaller {
	t.Helper()
	return func(method string, payload []byte) ([]byte, error) {
		if method != pluginabi.MethodHostHTTPDo {
			t.Fatalf("unexpected host method %q", method)
		}
		var req hostHTTPReq
		if err := json.Unmarshal(payload, &req); err != nil {
			t.Fatalf("undecodable host.http.do request: %v", err)
		}
		if req.URL != "https://api.cline.bot"+usageLimitsPath {
			t.Fatalf("unexpected upstream URL %q", req.URL)
		}
		if got := req.Headers.Get("Authorization"); got != "Bearer "+wantAuth {
			t.Fatalf("unexpected Authorization header %q", got)
		}
		body := []byte(`{"success":true,"data":{"limits":[
			{"type":"five_hour","percentUsed":12.3,"resetsAt":"2026-09-18T20:00:00Z"},
			{"type":"weekly","percentUsed":41.0,"resetsAt":"2026-09-21T00:00:00Z"},
			{"type":"monthly","percentUsed":8.7,"resetsAt":"2026-10-01T00:00:00Z"},
			{"type":"unknown_future_window","percentUsed":99,"resetsAt":"2027-01-01T00:00:00Z"}
		]}}`)
		resp := pluginapi.HTTPResponse{StatusCode: 200, Body: body}
		respJSON, _ := json.Marshal(resp)
		env := struct {
			OK     bool            `json:"ok"`
			Result json.RawMessage `json:"result"`
		}{OK: true, Result: respJSON}
		out, _ := json.Marshal(env)
		return out, nil
	}
}

func TestFetchQuotaParsesUpstreamWindows(t *testing.T) {
	mgr := NewManager(NewHostBridge(fakeCall(t, "test-key-123")))
	cfg := config.Config{BaseURL: config.DefaultBaseURL, RequestTimeout: config.DefaultRequestTimeout}

	usage, err := mgr.fetchQuota(context.Background(), cfg, "test-key-123")
	if err != nil {
		t.Fatalf("fetchQuota returned error: %v", err)
	}
	if usage.Rolling.Percent != 12 || usage.Rolling.Status != "ok" || usage.Rolling.ResetsAt != "2026-09-18T20:00:00Z" {
		t.Fatalf("unexpected five_hour window: %+v", usage.Rolling)
	}
	if usage.Weekly.Percent != 41 || usage.Weekly.Status != "ok" || usage.Weekly.ResetsAt != "2026-09-21T00:00:00Z" {
		t.Fatalf("unexpected weekly window: %+v", usage.Weekly)
	}
	if usage.Monthly.Percent != 8 || usage.Monthly.Status != "ok" || usage.Monthly.ResetsAt != "2026-10-01T00:00:00Z" {
		t.Fatalf("unexpected monthly window: %+v", usage.Monthly)
	}
}

func TestFetchQuotaUpstreamFailureFails(t *testing.T) {
	mgr := NewManager(NewHostBridge(func(method string, payload []byte) ([]byte, error) {
		resp := pluginapi.HTTPResponse{StatusCode: 401, Body: []byte(`{"error":"unauthorized"}`)}
		respJSON, _ := json.Marshal(resp)
		env := struct {
			OK     bool            `json:"ok"`
			Result json.RawMessage `json:"result"`
		}{OK: true, Result: respJSON}
		out, _ := json.Marshal(env)
		return out, nil
	}))
	cfg := config.Config{BaseURL: config.DefaultBaseURL, RequestTimeout: config.DefaultRequestTimeout}
	if _, err := mgr.fetchQuota(context.Background(), cfg, "bad-key"); err == nil {
		t.Fatal("expected error for 401 upstream response, got nil")
	}
}

func TestHandleManagementListsConfiguredKeysWithBestEffortProfileFetch(t *testing.T) {
	calls := 0
	mgr := NewManager(NewHostBridge(func(method string, payload []byte) ([]byte, error) {
		calls++
		if method != pluginabi.MethodHostHTTPDo {
			t.Fatalf("unexpected host method %q", method)
		}
		// Malformed/unreachable-upstream simulation: listing must still
		// succeed with the name simply omitted, never a hard failure.
		return nil, nil
	}))
	mgr.mu.Lock()
	mgr.cfg = config.Config{
		BaseURL:        config.DefaultBaseURL,
		RequestTimeout: config.DefaultRequestTimeout,
		APIKeys:        []config.APIKey{{Value: "k1", Label: "primary"}},
	}
	mgr.mu.Unlock()

	resp, err := mgr.HandleManagement(context.Background(), pluginapi.ManagementRequest{
		Method: "POST",
		Path:   "/v0/management/plugins/" + pluginName + "/quota-usage",
	})
	if err != nil {
		t.Fatalf("HandleManagement returned error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected exactly one profile-fetch call per key, got %d calls", calls)
	}
	var list quotaList
	if err := json.Unmarshal(resp.Body, &list); err != nil {
		t.Fatalf("undecodable response body: %v", err)
	}
	if len(list.Cards) != 1 || list.Cards[0].Label != "primary" {
		t.Fatalf("unexpected cards: %+v", list.Cards)
	}
	if list.Cards[0].Name != "" {
		t.Fatalf("expected empty Name on a failed profile fetch, got %q", list.Cards[0].Name)
	}
}

func TestHandleManagementListsAccountNameFromProfile(t *testing.T) {
	mgr := NewManager(NewHostBridge(func(method string, payload []byte) ([]byte, error) {
		var req hostHTTPReq
		if err := json.Unmarshal(payload, &req); err != nil {
			t.Fatalf("undecodable host.http.do request: %v", err)
		}
		if req.URL != config.DefaultBaseURL+profilePath {
			t.Fatalf("unexpected profile URL %q", req.URL)
		}
		resp := pluginapi.HTTPResponse{StatusCode: 200, Body: []byte(`{"success":true,"data":{"id":"usr-1","email":"a@b.c","displayName":"Przemek"}}`)}
		respJSON, _ := json.Marshal(resp)
		env := struct {
			OK     bool            `json:"ok"`
			Result json.RawMessage `json:"result"`
		}{OK: true, Result: respJSON}
		out, _ := json.Marshal(env)
		return out, nil
	}))
	mgr.mu.Lock()
	mgr.cfg = config.Config{
		BaseURL:        config.DefaultBaseURL,
		RequestTimeout: config.DefaultRequestTimeout,
		APIKeys:        []config.APIKey{{Value: "k1"}},
	}
	mgr.mu.Unlock()

	resp, err := mgr.HandleManagement(context.Background(), pluginapi.ManagementRequest{
		Method: "POST",
		Path:   "/v0/management/plugins/" + pluginName + "/quota-usage",
	})
	if err != nil {
		t.Fatalf("HandleManagement returned error: %v", err)
	}
	var list quotaList
	if err := json.Unmarshal(resp.Body, &list); err != nil {
		t.Fatalf("undecodable response body: %v", err)
	}
	if len(list.Cards) != 1 || list.Cards[0].Name != "Przemek" {
		t.Fatalf("expected card Name %q, got %+v", "Przemek", list.Cards)
	}
}

func TestDefaultLabelMasksKeySuffix(t *testing.T) {
	mgr := NewManager(NewHostBridge(func(method string, payload []byte) ([]byte, error) {
		// Listing now performs a best-effort profile fetch; simulate a
		// failed/unreachable upstream rather than asserting zero calls.
		return nil, nil
	}))
	mgr.mu.Lock()
	mgr.cfg = config.Config{
		BaseURL:        config.DefaultBaseURL,
		RequestTimeout: config.DefaultRequestTimeout,
		APIKeys:        []config.APIKey{{Value: "sk-clinepass-abcd1234"}},
	}
	mgr.mu.Unlock()

	resp, err := mgr.HandleManagement(context.Background(), pluginapi.ManagementRequest{
		Method: "POST",
		Path:   "/v0/management/plugins/" + pluginName + "/quota-usage",
	})
	if err != nil {
		t.Fatalf("HandleManagement returned error: %v", err)
	}
	var list quotaList
	if err := json.Unmarshal(resp.Body, &list); err != nil {
		t.Fatalf("undecodable response body: %v", err)
	}
	if len(list.Cards) != 1 {
		t.Fatalf("expected exactly one card, got %+v", list.Cards)
	}
	got := list.Cards[0].Label
	if got != "ClinePass ••••1234" {
		t.Fatalf("expected masked-suffix label %q, got %q", "ClinePass ••••1234", got)
	}
	if strings.Contains(got, "sk-clinepass-abcd") {
		t.Fatalf("label must not leak the unmasked key prefix: %q", got)
	}
}
