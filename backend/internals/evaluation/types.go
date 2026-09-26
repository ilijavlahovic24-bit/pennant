package evaluation

import "encoding/json"

// UserContext nosi atribute korisnika za targeting pravila.
// Klijent može poslati:
//   {"user_id":"u-42","attributes":{"plan":"enterprise","country":"RS"}}
// ili flat:
//   {"user_id":"u-42","plan":"enterprise","country":"RS"}
type UserContext struct {
	UserID     string         `json:"user_id"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

// UnmarshalJSON podržava i "attributes" objekat i flat top-level atribute.
func (u *UserContext) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	out := UserContext{Attributes: map[string]any{}}
	if v, ok := raw["user_id"]; ok {
		_ = json.Unmarshal(v, &out.UserID)
	}
	if v, ok := raw["attributes"]; ok {
		_ = json.Unmarshal(v, &out.Attributes)
	}
	for k, v := range raw {
		if k == "user_id" || k == "attributes" {
			continue
		}
		var val any
		if err := json.Unmarshal(v, &val); err == nil {
			out.Attributes[k] = val
		}
	}
	*u = out
	return nil
}

// FlagSnapshot je keširani prikaz stanja flaga u jednom environmentu.
// Serijalizuje se u Redis.
type FlagSnapshot struct {
	FlagKey        string          `json:"flag_key"`
	FlagType       string          `json:"flag_type"`
	Archived       bool            `json:"archived"`
	Enabled        bool            `json:"enabled"`
	RolloutPercent int             `json:"rollout_percent"`
	Value          json.RawMessage `json:"value,omitempty"`
	Rules          []RuleSnapshot  `json:"rules"`
}

type RuleSnapshot struct {
	Priority    int             `json:"priority"`
	Attribute   string          `json:"attribute"`
	Operator    string          `json:"operator"`
	Value       json.RawMessage `json:"value"`
	Action      string          `json:"action"`
	ActionValue json.RawMessage `json:"action_value,omitempty"`
}

// Result je rezultat evaluacije jednog flaga.
type Result struct {
	FlagKey string          `json:"flag_key"`
	Enabled bool            `json:"enabled"`
	Value   json.RawMessage `json:"value,omitempty"`
	Reason  string          `json:"reason"`
}
