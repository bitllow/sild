package domain

import (
	"encoding/json"
	"fmt"
	"unicode/utf8"

	"github.com/bitllow/sild/backend/internal/store/models"
)

// Limits on structured message parts, in characters. Results and summaries come
// from data providers and reach a requester's screen, so they stay short.
const (
	maxParts         = 10
	maxSteps         = 50
	maxLabelChars    = 200
	maxTextChars     = 1024
	maxInternalBytes = 1024
	maxFixItems      = 20
	maxEvidence      = 10
)

type stepPart struct {
	Label    string          `json:"label"`
	Result   string          `json:"result,omitempty"`
	State    string          `json:"state"`
	Internal json.RawMessage `json:"internal,omitempty"`
}

// A finding behind a diagnosis. Source names where it came from in the
// requester's words; internal holds the exact reference, for operators only.
type evidenceItem struct {
	Finding  string          `json:"finding"`
	Source   string          `json:"source"`
	Internal json.RawMessage `json:"internal,omitempty"`
}

type messagePart struct {
	Type     string         `json:"type"`
	Steps    []stepPart     `json:"steps,omitempty"`
	Cause    string         `json:"cause,omitempty"`
	Fix      []string       `json:"fix,omitempty"`
	Evidence []evidenceItem `json:"evidence,omitempty"`
}

// partsBySender is which part types each sender kind may attach.
var partsBySender = map[models.SenderKind]map[string]bool{
	models.SenderBot:    {"steps": true, "diagnosis": true},
	models.SenderSystem: {"approval": true},
}

// validateParts checks a parts document from sender and returns it
// re-encoded, so only known fields are stored.
func validateParts(raw json.RawMessage, sender models.SenderKind) (json.RawMessage, error) {
	allowed := partsBySender[sender]
	if allowed == nil {
		return nil, invalid("only an assistant sends parts")
	}
	var parts []messagePart
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil, invalid("parts must be an array of parts")
	}
	if len(parts) == 0 || len(parts) > maxParts {
		return nil, invalid(fmt.Sprintf("parts must hold 1 to %d parts", maxParts))
	}
	for i, p := range parts {
		if !allowed[p.Type] {
			return nil, invalid(fmt.Sprintf("parts[%d].type %q is not a part this sender may send", i, p.Type))
		}
		switch p.Type {
		case "steps":
			if len(p.Steps) == 0 || len(p.Steps) > maxSteps || p.Cause != "" || len(p.Fix) > 0 || len(p.Evidence) > 0 {
				return nil, invalid(fmt.Sprintf("parts[%d]: a steps part holds 1 to %d steps and nothing else", i, maxSteps))
			}
			for j, s := range p.Steps {
				switch s.State {
				case "running", "done", "not_available", "needs_approval":
				default:
					return nil, invalid(fmt.Sprintf("parts[%d].steps[%d].state must be running, done, not_available or needs_approval", i, j))
				}
				if !short(s.Label, maxLabelChars) || s.Label == "" || !short(s.Result, maxTextChars) || len(s.Internal) > maxInternalBytes {
					return nil, invalid(fmt.Sprintf("parts[%d].steps[%d] is empty or too long", i, j))
				}
			}
		case "diagnosis":
			if p.Cause == "" || !short(p.Cause, maxTextChars) {
				return nil, invalid(fmt.Sprintf("parts[%d]: a diagnosis cause is empty or longer than %d characters", i, maxTextChars))
			}
			if len(p.Fix) > maxFixItems || len(p.Steps) > 0 {
				return nil, invalid(fmt.Sprintf("parts[%d]: a diagnosis holds at most %d fix steps and no steps", i, maxFixItems))
			}
			for _, f := range p.Fix {
				if f == "" || !short(f, maxTextChars) {
					return nil, invalid(fmt.Sprintf("parts[%d]: a fix step is empty or too long", i))
				}
			}
			if len(p.Evidence) > maxEvidence {
				return nil, invalid(fmt.Sprintf("parts[%d]: a diagnosis holds at most %d evidence items", i, maxEvidence))
			}
			for j, e := range p.Evidence {
				if e.Finding == "" || !short(e.Finding, maxTextChars) || e.Source == "" || !short(e.Source, maxLabelChars) || len(e.Internal) > maxInternalBytes {
					return nil, invalid(fmt.Sprintf("parts[%d].evidence[%d] is empty or too long", i, j))
				}
			}
		case "approval":
			if len(p.Steps) > 0 || p.Cause != "" || len(p.Fix) > 0 || len(p.Evidence) > 0 {
				return nil, invalid(fmt.Sprintf("parts[%d]: an approval part holds nothing else", i))
			}
		}
	}
	return json.Marshal(parts)
}

// hasApproval reports a requester's approval of data access, which wakes the assistant like a reply.
func hasApproval(raw []byte) bool {
	var parts []messagePart
	if json.Unmarshal(raw, &parts) != nil {
		return false
	}
	for _, p := range parts {
		if p.Type == "approval" {
			return true
		}
	}
	return false
}

func short(s string, max int) bool { return utf8.ValidString(s) && utf8.RuneCountInString(s) <= max }
