package evaluation

import (
	"encoding/json"
	"strings"
)

// ruleOutcome opisuje rezultat primene jednog targeting pravila.
type ruleOutcome struct {
	matched       bool
	action        string
	actionPercent int
}

// evaluateRules prolazi kroz pravila po prioritetu (već sortirana ASC)
// i vraća prvi match. Ako nijedno ne matchuje, matched = false.
func evaluateRules(rules []RuleSnapshot, ctx UserContext) ruleOutcome {
	for _, r := range rules {
		if ruleMatches(r, ctx) {
			out := ruleOutcome{matched: true, action: r.Action}
			if r.Action == "serve_percent" {
				var p int
				if err := json.Unmarshal(r.ActionValue, &p); err == nil {
					out.actionPercent = p
				}
			}
			return out
		}
	}
	return ruleOutcome{matched: false}
}

func ruleMatches(r RuleSnapshot, ctx UserContext) bool {
	attrVal := resolveAttribute(r.Attribute, ctx)

	switch r.Operator {
	case "equals":
		var target string
		if err := json.Unmarshal(r.Value, &target); err != nil {
			return false
		}
		return attributeToString(attrVal) == target

	case "not_equals":
		var target string
		if err := json.Unmarshal(r.Value, &target); err != nil {
			return false
		}
		return attributeToString(attrVal) != target

	case "in":
		var targets []string
		if err := json.Unmarshal(r.Value, &targets); err != nil {
			return false
		}
		v := attributeToString(attrVal)
		for _, t := range targets {
			if v == t {
				return true
			}
		}
		return false

	case "not_in":
		var targets []string
		if err := json.Unmarshal(r.Value, &targets); err != nil {
			return false
		}
		v := attributeToString(attrVal)
		for _, t := range targets {
			if v == t {
				return false
			}
		}
		return true

	case "contains":
		var target string
		if err := json.Unmarshal(r.Value, &target); err != nil {
			return false
		}
		return strings.Contains(attributeToString(attrVal), target)
	}
	return false
}

// resolveAttribute traži vrednost atributa u user kontekstu.
// "user_id" je specijalan slučaj — uvek dostupan iz ctx.UserID.
func resolveAttribute(name string, ctx UserContext) any {
	if name == "user_id" {
		return ctx.UserID
	}
	if ctx.Attributes == nil {
		return nil
	}
	return ctx.Attributes[name]
}

func attributeToString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	default:
		b, err := json.Marshal(x)
		if err != nil {
			return ""
		}
		s := string(b)
		if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
			return s[1 : len(s)-1]
		}
		return s
	}
}
