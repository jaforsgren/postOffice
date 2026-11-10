package timing

import "time"

// Status describes whether a phase was actually measured for a request.
type Status int

const (
	// StatusMeasured means the phase happened and Offset/Duration are valid.
	StatusMeasured Status = iota
	// StatusReused means the phase was skipped because an existing connection was reused.
	StatusReused
	// StatusNotApplicable means the phase does not apply to this request
	// (e.g. no DNS lookup for an IP literal, no TLS handshake for plain HTTP).
	StatusNotApplicable
)

// Phase is a single step of a request's lifecycle.
// Offset is measured from the start of the request so phases can be laid out as a waterfall.
type Phase struct {
	Name     string
	Status   Status
	Offset   time.Duration
	Duration time.Duration
}
