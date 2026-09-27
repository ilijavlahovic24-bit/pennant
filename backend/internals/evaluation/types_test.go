package evaluation

import (
	"encoding/json"
	"testing"
)

func TestUserContext_FlatAttributes(t *testing.T) {
	raw := `{"user_id":"u-42","plan":"enterprise","country":"RS"}`
	var ctx UserContext
	if err := json.Unmarshal([]byte(raw), &ctx); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ctx.UserID != "u-42" {
		t.Fatalf("user_id: got %q", ctx.UserID)
	}
	if ctx.Attributes["plan"] != "enterprise" {
		t.Fatalf("plan: got %v", ctx.Attributes["plan"])
	}
	if ctx.Attributes["country"] != "RS" {
		t.Fatalf("country: got %v", ctx.Attributes["country"])
	}
	if _, ok := ctx.Attributes["user_id"]; ok {
		t.Fatalf("user_id should not be in Attributes")
	}
}

func TestUserContext_NestedAttributes(t *testing.T) {
	raw := `{"user_id":"u-42","attributes":{"plan":"pro","tier":2}}`
	var ctx UserContext
	if err := json.Unmarshal([]byte(raw), &ctx); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ctx.UserID != "u-42" {
		t.Fatalf("user_id: got %q", ctx.UserID)
	}
	if ctx.Attributes["plan"] != "pro" {
		t.Fatalf("plan: got %v", ctx.Attributes["plan"])
	}
	if ctx.Attributes["tier"] != float64(2) {
		t.Fatalf("tier: got %v", ctx.Attributes["tier"])
	}
}

func TestUserContext_MixedAttributes(t *testing.T) {
	// I nested i flat — oba se spajaju.
	raw := `{"user_id":"u-42","attributes":{"plan":"pro"},"country":"RS"}`
	var ctx UserContext
	if err := json.Unmarshal([]byte(raw), &ctx); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ctx.Attributes["plan"] != "pro" {
		t.Fatalf("plan: got %v", ctx.Attributes["plan"])
	}
	if ctx.Attributes["country"] != "RS" {
		t.Fatalf("country: got %v", ctx.Attributes["country"])
	}
}

func TestUserContext_Empty(t *testing.T) {
	var ctx UserContext
	if err := json.Unmarshal([]byte(`{}`), &ctx); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ctx.UserID != "" {
		t.Fatalf("expected empty user_id")
	}
	if len(ctx.Attributes) != 0 {
		t.Fatalf("expected empty attributes, got %v", ctx.Attributes)
	}
}

func TestUserContext_InvalidJSON(t *testing.T) {
	var ctx UserContext
	if err := json.Unmarshal([]byte(`not-json`), &ctx); err == nil {
		t.Fatalf("expected error for invalid JSON")
	}
}
