package timing

import "time"

// Measured builds a phase that ran from start to end, positioned relative to origin.
func Measured(name string, origin, start, end time.Time) Phase {
	return Phase{
		Name:     name,
		Status:   StatusMeasured,
		Offset:   start.Sub(origin),
		Duration: end.Sub(start),
	}
}

// Unmeasured builds a phase that did not run, with the reason given by status.
func Unmeasured(name string, status Status) Phase {
	return Phase{Name: name, Status: status}
}

// Sequence records back-to-back phases for protocols whose steps run strictly one after another.
type Sequence struct {
	origin time.Time
	cursor time.Time
	phases []Phase
}

// NewSequence starts a sequence whose first phase begins now.
// Phase offsets are reported relative to origin, the start of the whole request.
func NewSequence(origin time.Time) *Sequence {
	return &Sequence{origin: origin, cursor: time.Now()}
}

// Mark records a phase spanning from the end of the previous phase until now.
func (s *Sequence) Mark(name string) {
	now := time.Now()
	s.phases = append(s.phases, Measured(name, s.origin, s.cursor, now))
	s.cursor = now
}

// Phases returns the phases recorded so far.
func (s *Sequence) Phases() []Phase {
	return s.phases
}
