package timing

import (
	"testing"
	"time"
)

func TestMeasured_PositionsPhaseRelativeToOrigin(t *testing.T) {
	origin := time.Now()
	start := origin.Add(10 * time.Millisecond)
	end := start.Add(25 * time.Millisecond)

	phase := Measured("DNS", origin, start, end)

	if phase.Status != StatusMeasured {
		t.Errorf("Status = %v, want StatusMeasured", phase.Status)
	}
	if phase.Offset != 10*time.Millisecond {
		t.Errorf("Offset = %v, want 10ms", phase.Offset)
	}
	if phase.Duration != 25*time.Millisecond {
		t.Errorf("Duration = %v, want 25ms", phase.Duration)
	}
}

func TestUnmeasured_HasNoDuration(t *testing.T) {
	phase := Unmeasured("TLS", StatusReused)

	if phase.Status != StatusReused || phase.Offset != 0 || phase.Duration != 0 {
		t.Errorf("Unmeasured = %+v, want reused phase with zero timings", phase)
	}
}

func TestSequence_RecordsContiguousPhases(t *testing.T) {
	origin := time.Now().Add(-5 * time.Millisecond)
	sequence := NewSequence(origin)

	sequence.Mark("first")
	time.Sleep(2 * time.Millisecond)
	sequence.Mark("second")

	phases := sequence.Phases()
	if len(phases) != 2 {
		t.Fatalf("got %d phases, want 2", len(phases))
	}
	if phases[0].Offset < 5*time.Millisecond {
		t.Errorf("first phase offset %v should include time before the sequence started", phases[0].Offset)
	}
	if got, want := phases[1].Offset, phases[0].Offset+phases[0].Duration; got != want {
		t.Errorf("second phase offset = %v, want end of first phase %v", got, want)
	}
	if phases[1].Duration < 2*time.Millisecond {
		t.Errorf("second phase duration = %v, want >= 2ms", phases[1].Duration)
	}
}

func TestSequence_EmptyWhenNothingMarked(t *testing.T) {
	if phases := NewSequence(time.Now()).Phases(); len(phases) != 0 {
		t.Errorf("got %d phases, want none", len(phases))
	}
}
