package config

import (
	"strings"
	"testing"
)

func TestLoadAcceptsBareStringKeys(t *testing.T) {
	t.Setenv("TEST_CP_KEY", "key-from-env")
	c, err := Load([]byte("api-keys: [\"key-bare\", {value: key-mapped, label: work}, \"${TEST_CP_KEY}\"]\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := []APIKey{{Value: "key-bare"}, {Value: "key-mapped", Label: "work"}, {Value: "key-from-env"}}
	if len(c.APIKeys) != len(want) {
		t.Fatalf("keys = %+v", c.APIKeys)
	}
	for i := range want {
		if c.APIKeys[i] != want[i] {
			t.Fatalf("key %d = %+v, want %+v", i, c.APIKeys[i], want[i])
		}
	}
}

func TestLoadRejectsMalformedKeysWithoutLeaking(t *testing.T) {
	for _, y := range []string{
		"api-keys:\n  - [key-secret-1]\n",
		"api-keys:\n  - \"\"\n",
		"api-keys:\n  - key-secret-2\n  - value: key-secret-2\n",
	} {
		_, err := Load([]byte(y))
		if err == nil {
			t.Fatalf("expected rejection for %q", y)
		}
		if strings.Contains(err.Error(), "key-secret") {
			t.Fatalf("error leaks key: %v", err)
		}
	}
}
