package evaluation

import (
	"encoding/json"
	"testing"
)

func TestEvaluateSnapshot_Archived(t *testing.T) {
	snap := &FlagSnapshot{FlagKey: "f", Archived: true, Enabled: true}
	res := evaluateSnapshot(snap, "f", UserContext{UserID: "u"})
	if res.Enabled || res.Reason != "archived" {
		t.Fatalf("expected archived, got %+v", res)
	}
}

func TestEvaluateSnapshot_Disabled(t *testing.T) {
	snap := &FlagSnapshot{FlagKey: "f", Archived: false, Enabled: false}
	res := evaluateSnapshot(snap, "f", UserContext{UserID: "u"})
	if res.Enabled || res.Reason != "disabled" {
		t.Fatalf("expected disabled, got %+v", res)
	}
}

func TestEvaluateSnapshot_Targeting_ServeEnabled(t *testing.T) {
	snap := &FlagSnapshot{
		FlagKey: "f",
		Enabled: true,
		Value:   raw(`true`),
		Rules: []RuleSnapshot{
			{Priority: 0, Attribute: "plan", Operator: "equals",
				Value: raw(`"enterprise"`), Action: "serve_enabled"},
		},
	}
	ctx := UserContext{UserID: "u", Attributes: map[string]any{"plan": "enterprise"}}
	res := evaluateSnapshot(snap, "f", ctx)
	if !res.Enabled || res.Reason != "targeting_match" {
		t.Fatalf("expected targeting_match enabled, got %+v", res)
	}
}

func TestEvaluateSnapshot_Targeting_ServeDisabled(t *testing.T) {
	snap := &FlagSnapshot{
		FlagKey:        "f",
		Enabled:        true,
		RolloutPercent: 100,
		Rules: []RuleSnapshot{
			{Priority: 0, Attribute: "plan", Operator: "equals",
				Value: raw(`"free"`), Action: "serve_disabled"},
		},
	}
	ctx := UserContext{UserID: "u", Attributes: map[string]any{"plan": "free"}}
	res := evaluateSnapshot(snap, "f", ctx)
	if res.Enabled || res.Reason != "targeting_match" {
		t.Fatalf("expected targeting_match disabled, got %+v", res)
	}
}

func TestEvaluateSnapshot_Targeting_ServePercent(t *testing.T) {
	snap := &FlagSnapshot{
		FlagKey: "f",
		Enabled: true,
		Rules: []RuleSnapshot{
			{Priority: 0, Attribute: "plan", Operator: "equals",
				Value: raw(`"pro"`), Action: "serve_percent", ActionValue: raw(`100`)},
		},
	}
	ctx := UserContext{UserID: "u", Attributes: map[string]any{"plan": "pro"}}
	res := evaluateSnapshot(snap, "f", ctx)
	if !res.Enabled || res.Reason != "targeting_match" {
		t.Fatalf("expected targeting_match with 100%%, got %+v", res)
	}
}

func TestEvaluateSnapshot_Targeting_NoMatch_FallsBackToRollout(t *testing.T) {
	// Pravilo matchuje plan=enterprise, ali user je free.
	// Padamo na rollout_percent=100 → enabled, reason="rollout".
	snap := &FlagSnapshot{
		FlagKey:        "f",
		Enabled:        true,
		RolloutPercent: 100,
		Rules: []RuleSnapshot{
			{Priority: 0, Attribute: "plan", Operator: "equals",
				Value: raw(`"enterprise"`), Action: "serve_enabled"},
		},
	}
	ctx := UserContext{UserID: "u", Attributes: map[string]any{"plan": "free"}}
	res := evaluateSnapshot(snap, "f", ctx)
	if !res.Enabled || res.Reason != "rollout" {
		t.Fatalf("expected rollout enabled, got %+v", res)
	}
}

func TestEvaluateSnapshot_RolloutZeroPercent(t *testing.T) {
	snap := &FlagSnapshot{
		FlagKey:        "f",
		Enabled:        true,
		RolloutPercent: 0,
	}
	res := evaluateSnapshot(snap, "f", UserContext{UserID: "u"})
	if res.Enabled || res.Reason != "rollout" {
		t.Fatalf("expected rollout disabled at 0%%, got %+v", res)
	}
}

func TestEvaluateSnapshot_RolloutHundredPercent(t *testing.T) {
	snap := &FlagSnapshot{
		FlagKey:        "f",
		Enabled:        true,
		RolloutPercent: 100,
		Value:          raw(`"green"`),
	}
	res := evaluateSnapshot(snap, "f", UserContext{UserID: "u"})
	if !res.Enabled || res.Reason != "rollout" {
		t.Fatalf("expected rollout enabled at 100%%, got %+v", res)
	}
	if string(res.Value) != `"green"` {
		t.Fatalf("value should be preserved when enabled, got %s", res.Value)
	}
}

func TestEvaluateSnapshot_ValueOmittedWhenDisabled(t *testing.T) {
	snap := &FlagSnapshot{
		FlagKey:        "f",
		Enabled:        true,
		RolloutPercent: 0,
		Value:          raw(`"green"`),
	}
	res := evaluateSnapshot(snap, "f", UserContext{UserID: "u"})
	if res.Enabled {
		t.Fatalf("expected disabled")
	}
	if len(res.Value) != 0 {
		t.Fatalf("value should be nil when disabled, got %s", res.Value)
	}
}

func TestEvaluateSnapshot_DeterministicAcrossCalls(t *testing.T) {
	// Isti snapshot + isti user → isti rezultat u 100 poziva.
	snap := &FlagSnapshot{
		FlagKey:        "checkout",
		Enabled:        true,
		RolloutPercent: 50,
	}
	ctx := UserContext{UserID: "u-42"}
	first := evaluateSnapshot(snap, "checkout", ctx)
	for i := 0; i < 100; i++ {
		got := evaluateSnapshot(snap, "checkout", ctx)
		if got.Enabled != first.Enabled {
			t.Fatalf("not deterministic: %v vs %v", first.Enabled, got.Enabled)
		}
	}
}

func TestEvaluateSnapshot_ArchivedBeatsEverything(t *testing.T) {
	// Arhiviran flag ignoriše enabled, rules, rollout.
	snap := &FlagSnapshot{
		FlagKey:        "f",
		Archived:       true,
		Enabled:        true,
		RolloutPercent: 100,
		Rules: []RuleSnapshot{
			{Priority: 0, Attribute: "plan", Operator: "equals",
				Value: raw(`"pro"`), Action: "serve_enabled"},
		},
	}
	ctx := UserContext{UserID: "u", Attributes: map[string]any{"plan": "pro"}}
	res := evaluateSnapshot(snap, "f", ctx)
	if res.Enabled || res.Reason != "archived" {
		t.Fatalf("archived must win, got %+v", res)
	}
}

func TestEvaluateSnapshot_DisabledBeatsRules(t *testing.T) {
	// Ako je enabled=false, čak i pravilo serve_enabled ne pomaže.
	snap := &FlagSnapshot{
		FlagKey: "f",
		Enabled: false,
		Rules: []RuleSnapshot{
			{Priority: 0, Attribute: "plan", Operator: "equals",
				Value: raw(`"pro"`), Action: "serve_enabled"},
		},
	}
	ctx := UserContext{UserID: "u", Attributes: map[string]any{"plan": "pro"}}
	res := evaluateSnapshot(snap, "f", ctx)
	if res.Enabled || res.Reason != "disabled" {
		t.Fatalf("disabled must win over rules, got %+v", res)
	}
}

// raw helper — isti kao u rules_test.go; ako je već definisan u paketu,
// obriši ovaj i koristi postojeći.
var _ = json.RawMessage(nil)
