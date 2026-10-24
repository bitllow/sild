package domain

import (
	"encoding/json"
	"fmt"
	"unicode/utf8"
)

// Limits on structured message parts. Results and summaries come from data
// providers and reach a requester's screen, so they stay short.
const (
	maxParts      = 10
	maxSteps      = 50
	maxLabelBytes = 200
	maxTextBytes  = 1024
	maxFixItems   = 20
)

type stepPart struct {
	Label    string          `json:"label"`
	Result   string          `json:"result,omitempty"`
	State    string          `json:"state"`
	Internal json.RawMessage `json:"internal,omitempty"`
}

type messagePart struct {
	Type  string     `json:"type"`
	Steps []stepPart `json:"steps,omitempty"`
	Cause string     `json:"cause,omitempty"`
	Fix   []string   `json:"fix,omitempty"`
}

// validateParts checks a parts document and returns it re-encoded, so only
// known fields are stored.
func validateParts(raw json.RawMessage) (json.RawMessage, error) {
	var parts []messagePart
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil, invalid("parts must be an array of parts")
	}
	if len(parts) == 0 || len(parts) > maxParts {
		return nil, invalid(fmt.Sprintf("parts must hold 1 to %d parts", maxParts))
	}
	for i, p := range parts {
		switch p.Type {
		case "steps":
			if len(p.Steps) == 0 || len(p.Steps) > maxSteps || p.Cause != "" || len(p.Fix) > 0 {
				return nil, invalid(fmt.Sprintf("parts[%d]: a steps part holds 1 to %d steps and nothing else", i, maxSteps))
			}
			for j, s := range p.Steps {
				switch s.State {
				case "running", "done", "not_available":
				default:
					return nil, invalid(fmt.Sprintf("parts[%d].steps[%d].state must be running, done or not_available", i, j))
				}
				if !short(s.Label, maxLabelBytes) || s.Label == "" || !short(s.Result, maxTextBytes) || len(s.Internal) > maxTextBytes {
					return nil, invalid(fmt.Sprintf("parts[%d].steps[%d] is empty or too long", i, j))
				}
			}
		case "diagnosis":
			if p.Cause == "" || !short(p.Cause, maxTextBytes) || len(p.Fix) > maxFixItems || len(p.Steps) > 0 {
				return nil, invalid(fmt.Sprintf("parts[%d]: a diagnosis needs a cause and at most %d fix steps", i, maxFixItems))
			}
			for _, f := range p.Fix {
				if f == "" || !short(f, maxTextBytes) {
					return nil, invalid(fmt.Sprintf("parts[%d]: a fix step is empty or too long", i))
				}
			}
		default:
			return nil, invalid(fmt.Sprintf("parts[%d].type must be steps or diagnosis", i))
		}
	}
	return json.Marshal(parts)
}

func short(s string, max int) bool { return len(s) <= max && utf8.ValidString(s) }
