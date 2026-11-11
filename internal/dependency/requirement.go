package dependency

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	requestKey     = "request"
	maxAgeKey      = "max_age"
	refreshOnKey   = "refresh_on"
	whenMissingKey = "when_missing"

	minHTTPStatus = 100
	maxHTTPStatus = 599
)

var requirementKeys = []string{requestKey, maxAgeKey, refreshOnKey, whenMissingKey}

// requirementYAML mirrors the mapping form of a requires entry before validation.
type requirementYAML struct {
	Request     string   `yaml:"request"`
	MaxAge      string   `yaml:"max_age"`
	RefreshOn   []int    `yaml:"refresh_on"`
	WhenMissing []string `yaml:"when_missing"`
}

// UnmarshalYAML accepts either a bare request path or a mapping with rules.
// Unknown keys are rejected here because node decoding ignores the decoder's KnownFields setting.
func (r *Requirement) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		*r = Requirement{Request: value.Value}
		return r.validate(value.Line)
	}
	if value.Kind != yaml.MappingNode {
		return &InvalidRequirementError{Line: value.Line, Reason: "expected a request path or a mapping"}
	}

	for i := 0; i < len(value.Content); i += 2 {
		key := value.Content[i]
		if !slices.Contains(requirementKeys, key.Value) {
			return &InvalidRequirementError{
				Line:   key.Line,
				Reason: fmt.Sprintf("unknown key %q (allowed: %s)", key.Value, strings.Join(requirementKeys, ", ")),
			}
		}
	}

	var raw requirementYAML
	if err := value.Decode(&raw); err != nil {
		return err
	}

	*r = Requirement{Request: raw.Request, RefreshOn: raw.RefreshOn, WhenMissing: raw.WhenMissing}
	if raw.MaxAge != "" {
		maxAge, err := time.ParseDuration(raw.MaxAge)
		if err != nil {
			return &InvalidRequirementError{Line: value.Line, Reason: fmt.Sprintf("%s %q is not a duration like 15m or 1h", maxAgeKey, raw.MaxAge)}
		}
		r.MaxAge = maxAge
		if maxAge <= 0 {
			return &InvalidRequirementError{Line: value.Line, Reason: fmt.Sprintf("%s must be positive, got %s", maxAgeKey, raw.MaxAge)}
		}
	}
	return r.validate(value.Line)
}

func (r Requirement) validate(line int) error {
	if r.Request == "" {
		return &InvalidRequirementError{Line: line, Reason: requestKey + " is required"}
	}
	if r.MaxAge > 0 && len(r.WhenMissing) > 0 {
		return &InvalidRequirementError{Line: line, Reason: maxAgeKey + " and " + whenMissingKey + " cannot be combined"}
	}
	for _, status := range r.RefreshOn {
		if status < minHTTPStatus || status > maxHTTPStatus {
			return &InvalidRequirementError{Line: line, Reason: fmt.Sprintf("%s status %d is not between %d and %d", refreshOnKey, status, minHTTPStatus, maxHTTPStatus)}
		}
	}
	return nil
}

// Rules describes the requirement's freshness rules for display, e.g.
// "max_age 15m, refresh_on 401"; it is empty for a bare request path.
func (r Requirement) Rules() string {
	var rules []string
	if r.MaxAge > 0 {
		rules = append(rules, maxAgeKey+" "+formatDuration(r.MaxAge))
	}
	if len(r.RefreshOn) > 0 {
		statuses := make([]string, len(r.RefreshOn))
		for i, status := range r.RefreshOn {
			statuses[i] = fmt.Sprint(status)
		}
		rules = append(rules, refreshOnKey+" "+strings.Join(statuses, "/"))
	}
	if len(r.WhenMissing) > 0 {
		rules = append(rules, whenMissingKey+" "+strings.Join(r.WhenMissing, ", "))
	}
	return strings.Join(rules, ", ")
}

// formatDuration drops the zero minute and second units time.Duration prints ("1h0m0s" → "1h").
func formatDuration(d time.Duration) string {
	text := d.String()
	if strings.HasSuffix(text, "m0s") {
		text = strings.TrimSuffix(text, "0s")
	}
	if strings.HasSuffix(text, "h0m") {
		text = strings.TrimSuffix(text, "0m")
	}
	return text
}
