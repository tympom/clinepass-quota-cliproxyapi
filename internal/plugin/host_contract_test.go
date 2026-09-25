package plugin

import (
	"encoding/json"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

// These tests decode our RPC responses using the *actual* host-side SDK
// types (from the same github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi
// package the real host uses to parse them), instead of our own hand-rolled
// anonymous structs. A field-name/shape mismatch against the real wire
// contract fails here instead of silently producing an empty card list or a
// "management_failure" in production.

type rpcEnvelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// rpcManagementRegistrationResponse mirrors CLIProxyAPI's own
// internal/pluginhost/rpc_schema.go decode target for management.register.
type rpcManagementRegistrationResponse struct {
	Routes    []pluginapi.ManagementRoute `json:"routes,omitempty"`
	Resources []pluginapi.ResourceRoute   `json:"resources,omitempty"`
}

func TestRegisterWithoutKeysThenConfigure(t *testing.T) {
	mgr := NewManager(nil)
	var env rpcEnvelope
	raw, err := mgr.HandleCall(pluginabi.MethodPluginRegister, []byte(`{"config_yaml":null}`))
	if err != nil || json.Unmarshal(raw, &env) != nil || !env.OK {
		t.Fatalf("keyless register failed: %v, %s", err, raw)
	}
	var reg struct {
		Metadata pluginapi.Metadata `json:"metadata"`
	}
	if err := json.Unmarshal(env.Result, &reg); err != nil || len(reg.Metadata.ConfigFields) != 3 {
		t.Fatalf("keyless config editor fields unavailable: %v, %+v", err, reg.Metadata.ConfigFields)
	}
	if len(mgr.cfg.APIKeys) != 0 {
		t.Fatal("unconfigured plugin has credentials")
	}

	raw, err = mgr.HandleCall(pluginabi.MethodPluginReconfigure, []byte(`{"config_yaml":"YXBpLWtleXM6CiAgLSB2YWx1ZTogc2stdGVzdAo="}`))
	if err != nil || json.Unmarshal(raw, &env) != nil || !env.OK {
		t.Fatalf("keyed reconfigure failed: %v, %s", err, raw)
	}
	if len(mgr.cfg.APIKeys) != 1 || mgr.cfg.APIKeys[0].Value != "sk-test" {
		t.Fatal("configured key not applied")
	}
}

func TestManagementRegisterMatchesHostWireContract(t *testing.T) {
	mgr := NewManager(NewHostBridge(func(string, []byte) ([]byte, error) { return nil, nil }))

	raw, err := mgr.HandleCall(pluginabi.MethodManagementRegister, []byte(`{"Plugin":{},"BasePath":"/v0/management","ResourceBasePath":"/v0/resource/plugins"}`))
	if err != nil {
		t.Fatalf("HandleCall returned error: %v", err)
	}
	var env rpcEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("undecodable envelope: %v", err)
	}
	if !env.OK {
		t.Fatalf("management.register returned error envelope: %+v", env.Error)
	}
	var reg rpcManagementRegistrationResponse
	if err := json.Unmarshal(env.Result, &reg); err != nil {
		t.Fatalf("management.register result does not decode into the host's pluginapi.ManagementRoute/ResourceRoute shape: %v\nraw: %s", err, env.Result)
	}
	if len(reg.Routes) != 1 || reg.Routes[0].Method != "POST" || reg.Routes[0].Path != "/plugins/"+pluginName+"/quota-usage" {
		t.Fatalf("unexpected Routes: %+v", reg.Routes)
	}
	if len(reg.Resources) != 1 || reg.Resources[0].Path != "/quota" || reg.Resources[0].Menu != "ClinePass Quota" {
		t.Fatalf("unexpected Resources: %+v", reg.Resources)
	}
}

func TestManagementHandleMatchesHostWireContract(t *testing.T) {
	mgr := NewManager(NewHostBridge(func(string, []byte) ([]byte, error) { return nil, nil }))

	req := pluginapi.ManagementRequest{Method: "GET", Path: "/v0/resource/plugins/" + pluginName + "/quota"}
	payload, _ := json.Marshal(req)

	raw, err := mgr.HandleCall(pluginabi.MethodManagementHandle, payload)
	if err != nil {
		t.Fatalf("HandleCall returned error: %v", err)
	}
	var env rpcEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("undecodable envelope: %v", err)
	}
	if !env.OK {
		t.Fatalf("management.handle returned error envelope: %+v", env.Error)
	}
	var resp pluginapi.ManagementResponse
	if err := json.Unmarshal(env.Result, &resp); err != nil {
		t.Fatalf("management.handle result does not decode into the host's pluginapi.ManagementResponse shape: %v\nraw: %s", err, env.Result)
	}
	if len(resp.Body) == 0 {
		t.Fatalf("expected non-empty Body")
	}
}
