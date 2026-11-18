package agent

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// plainRules rewrites a tool result for the model: every issue object (one
// with a "rule") loses its rule and code and gets a plain "label" and one
// plain "message" instead, and the keys of a "dropped" count ("H9.code")
// become labels. The model repeats what it reads and the user cannot read
// rule codes; the HTTP API keeps its codes. A result without issues is
// returned as it is.
func plainRules(raw json.RawMessage) json.RawMessage {
	if !bytes.Contains(raw, []byte(`"rule"`)) && !bytes.Contains(raw, []byte(`"dropped"`)) {
		return raw
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if dec.Decode(&v) != nil {
		return raw
	}
	out, err := json.Marshal(plainWalk(v))
	if err != nil {
		return raw
	}
	return out
}

func plainWalk(v any) any {
	switch x := v.(type) {
	case []any:
		for i := range x {
			x[i] = plainWalk(x[i])
		}
	case map[string]any:
		for k, e := range x {
			if k == "dropped" {
				if m, ok := e.(map[string]any); ok {
					x[k] = plainDropped(m)
					continue
				}
			}
			x[k] = plainWalk(e)
		}
		if _, ok := x["rule"].(string); ok {
			code, _ := x["code"].(string)
			t := planning.PlainText(code)
			delete(x, "rule")
			delete(x, "code")
			x["label"], x["message"] = t.Label, t.Message
		}
	}
	return v
}

// plainDropped renames the "rule.code" keys of alternatives' dropped counts
// (a key may also be a bare reason such as nutrition_missing).
func plainDropped(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, n := range m {
		name := k
		if _, code, ok := strings.Cut(k, "."); ok {
			name = planning.PlainText(code).Label
		} else if k == "nutrition_missing" {
			name = "missing nutrition data"
		}
		out[name] = n
	}
	return out
}
