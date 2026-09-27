package evaluation

import (
	"encoding/json"
	"testing"
)

func raw(s string) json.RawMessage { return json.RawMessage(s) }

func TestEvaluateRules_Empty(t *testing.T) {
	out := evaluateRules(nil, UserContext{UserID: "u"})
	if out.matched {
		t.Fatalf("empty rules must not match")
	}
}

func TestEvaluateRules_Equals_Match(t *testing.T) {
	rules := []RuleSnapshot{
		{Priority: 0, Attribute: "plan", Operator: "equals", Value: raw(`"enterprise"`), Action: "serve_enabled"},
	}
	ctx := UserContext{UserID: "u", Attributes: map[string]any{"plan": "enterprise"}}
	out := evaluateRules(rules, ctx)
	if !out.matched || out.action != "serve_enabled" {
		t.Fatalf("expected match serve_enabled, got %+v", out)
	}
}

func TestEvaluateRules_Equals_NoMatch(t *testing.T) {
	rules := []RuleSnapshot{
		{Priority: 0, Attribute: "plan", Operator: "equals", Value: raw(`"enterprise"`), Action: "serve_enabled"},
	}
	ctx := UserContext{UserID: "u", Attributes: map[string]any{"plan": "free"}}
	out := evaluateRules(rules, ctx)
	if out.matched {
		t.Fatalf("expected no match")
	}
}

func TestEvaluateRules_NotEquals(t *testing.T) {
	rules := []RuleSnapshot{
		{Priority: 0, Attribute: "plan", Operator: "not_equals", Value: raw(`"free"`), Action: "serve_enabled"},
	}
	ctx := UserContext{UserID: "u", Attributes: map[string]any{"plan": "pro"}}
	if out := evaluateRules(rules, ctx); !out.matched {
		t.Fatalf("expected match")
	}

	ctx2 := UserContext{UserID: "u", Attributes: map[string]any{"plan": "free"}}
	if out := evaluateRules(rules, ctx2); out.matched {
		t.Fatalf("expected no match")
	}
}

func TestEvaluateRules_In_Match(t *testing.T) {
	rules := []RuleSnapshot{
		{Priority: 0, Attribute: "country", Operator: "in", Value: raw(`["RS","BA","HR"]`), Action: "serve_enabled"},
	}
	ctx := UserContext{UserID: "u", Attributes: map[string]any{"country": "RS"}}
	if out := evaluateRules(rules, ctx); !out.matched {
		t.Fatalf("expected match")
	}

	ctx2 := UserContext{UserID: "u", Attributes: map[string]any{"country": "DE"}}
	if out := evaluateRules(rules, ctx2); out.matched {
		t.Fatalf("expected no match")
	}
}

func TestEvaluateRules_NotIn(t *testing.T) {
	rules := []RuleSnapshot{
		{Priority: 0, Attribute: "country", Operator: "not_in", Value: raw(`["US","CN"]`), Action: "serve_enabled"},
	}
	ctx := UserContext{UserID: "u", Attributes: map[string]any{"country": "RS"}}
	if out := evaluateRules(rules, ctx); !out.matched {
		t.Fatalf("expected match (RS not in [US,CN])")
	}

	ctx2 := UserContext{UserID: "u", Attributes: map[string]any{"country": "US"}}
	if out := evaluateRules(rules, ctx2); out.matched {
		t.Fatalf("expected no match (US in [US,CN])")
	}
}

func TestEvaluateRules_Contains(t *testing.T) {
	rules := []RuleSnapshot{
		{Priority: 0, Attribute: "email", Operator: "contains", Value: raw(`"@acme.com"`), Action: "serve_enabled"},
	}
	ctx := UserContext{UserID: "u", Attributes: map[string]any{"email": "alice@acme.com"}}
	if out := evaluateRules(rules, ctx); !out.matched {
		t.Fatalf("expected match")
	}

	ctx2 := UserContext{UserID: "u", Attributes: map[string]any{"email": "bob@other.com"}}
	if out := evaluateRules(rules, ctx2); out.matched {
		t.Fatalf("expected no match")
	}
}

func TestEvaluateRules_FirstMatchWins(t *testing.T) {
	// Prvo pravilo matchuje i ima serve_disabled — drugo bi dalo serve_enabled,
	// ali ne sme se evaluirati.
	rules := []RuleSnapshot{
		{Priority: 0, Attribute: "plan", Operator: "equals", Value: raw(`"free"`), Action: "serve_disabled"},
		{Priority: 1, Attribute: "plan", Operator: "equals", Value: raw(`"free"`), Action: "serve_enabled"},
	}
	ctx := UserContext{UserID: "u", Attributes: map[string]any{"plan": "free"}}
	out := evaluateRules(rules, ctx)
	if !out.matched || out.action != "serve_disabled" {
		t.Fatalf("expected first rule serve_disabled, got %+v", out)
	}
}

func TestEvaluateRules_NoneMatch(t *testing.T) {
	rules := []RuleSnapshot{
		{Priority: 0, Attribute: "plan", Operator: "equals", Value: raw(`"enterprise"`), Action: "serve_enabled"},
		{Priority: 1, Attribute: "country", Operator: "in", Value: raw(`["US"]`), Action: "serve_enabled"},
	}
	ctx := UserContext{UserID: "u", Attributes: map[string]any{"plan": "free", "country": "RS"}}
	out := evaluateRules(rules, ctx)
	if out.matched {
		t.Fatalf("expected no match, got %+v", out)
	}
}

func TestEvaluateRules_ServePercent(t *testing.T) {
	rules := []RuleSnapshot{
		{Priority: 0, Attribute: "plan", Operator: "equals", Value: raw(`"pro"`),
			Action: "serve_percent", ActionValue: raw(`25`)},
	}
	ctx := UserContext{UserID: "u", Attributes: map[string]any{"plan": "pro"}}
	out := evaluateRules(rules, ctx)
	if !out.matched {
		t.Fatalf("expected match")
	}
	if out.action != "serve_percent" || out.actionPercent != 25 {
		t.Fatalf("expected serve_percent 25, got %+v", out)
	}
}

func TestEvaluateRules_UserIDAttribute(t *testing.T) {
	// "user_id" je specijalan slučaj — dostupan iz ctx.UserID, ne iz Attributes.
	rules := []RuleSnapshot{
		{Priority: 0, Attribute: "user_id", Operator: "equals", Value: raw(`"u-42"`), Action: "serve_enabled"},
	}
	ctx := UserContext{UserID: "u-42"}
	if out := evaluateRules(rules, ctx); !out.matched {
		t.Fatalf("expected match on user_id")
	}

	ctx2 := UserContext{UserID: "u-99"}
	if out := evaluateRules(rules, ctx2); out.matched {
		t.Fatalf("expected no match on user_id")
	}
}

func TestEvaluateRules_MissingAttribute(t *testing.T) {
	// Atribut ne postoji u kontekstu — ne sme panično, mora false.
	rules := []RuleSnapshot{
		{Priority: 0, Attribute: "plan", Operator: "equals", Value: raw(`"pro"`), Action: "serve_enabled"},
	}
	ctx := UserContext{UserID: "u"}
	if out := evaluateRules(rules, ctx); out.matched {
		t.Fatalf("expected no match when attribute missing")
	}
}

func TestEvaluateRules_NumericAttribute(t *testing.T) {
	// Brojevi se konvertuju u string za poređenje.
	rules := []RuleSnapshot{
		{Priority: 0, Attribute: "level", Operator: "equals", Value: raw(`"5"`), Action: "serve_enabled"},
	}
	ctx := UserContext{UserID: "u", Attributes: map[string]any{"level": float64(5)}}
	if out := evaluateRules(rules, ctx); !out.matched {
		t.Fatalf("expected match for level=5")
	}
}

func TestEvaluateRules_InvalidJSONValue(t *testing.T) {
	// Ako je value neispravan JSON, pravilo se preskače (ne paniči).
	rules := []RuleSnapshot{
		{Priority: 0, Attribute: "plan", Operator: "equals", Value: raw(`not-json`), Action: "serve_enabled"},
	}
	ctx := UserContext{UserID: "u", Attributes: map[string]any{"plan": "pro"}}
	if out := evaluateRules(rules, ctx); out.matched {
		t.Fatalf("invalid rule should not match")
	}
}

func TestEvaluateRules_UnknownOperator(t *testing.T) {
	rules := []RuleSnapshot{
		{Priority: 0, Attribute: "plan", Operator: "regex", Value: raw(`"pro"`), Action: "serve_enabled"},
	}
	ctx := UserContext{UserID: "u", Attributes: map[string]any{"plan": "pro"}}
	if out := evaluateRules(rules, ctx); out.matched {
		t.Fatalf("unknown operator should not match")
	}
}
