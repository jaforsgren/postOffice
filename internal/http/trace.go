package http

import (
	"crypto/tls"
	"net/http/httptrace"
	"postOffice/internal/timing"
	"sync"
	"time"
)

const (
	PhaseDNS      = "DNS lookup"
	PhaseConnect  = "TCP connect"
	PhaseTLS      = "TLS handshake"
	PhaseTTFB     = "TTFB"
	PhaseDownload = "Content download"
)

// traceRecorder collects httptrace events for a single request.
// The transport may invoke hooks from different goroutines (e.g. parallel dial attempts),
// so every access is guarded by mu.
type traceRecorder struct {
	mu sync.Mutex

	dnsStart     time.Time
	dnsDone      time.Time
	connectStart time.Time
	connectDone  time.Time
	tlsStart     time.Time
	tlsDone      time.Time
	gotConn      time.Time
	connReused   bool
	wroteRequest time.Time
	firstByte    time.Time
	bodyRead     time.Time
}

func (r *traceRecorder) clientTrace() *httptrace.ClientTrace {
	return &httptrace.ClientTrace{
		DNSStart: func(httptrace.DNSStartInfo) { r.record(&r.dnsStart) },
		DNSDone:  func(httptrace.DNSDoneInfo) { r.record(&r.dnsDone) },
		ConnectStart: func(string, string) {
			r.mu.Lock()
			defer r.mu.Unlock()
			// Dual-stack dialing may start several attempts; the phase begins with the first one.
			if r.connectStart.IsZero() {
				r.connectStart = time.Now()
			}
		},
		ConnectDone:          func(string, string, error) { r.record(&r.connectDone) },
		TLSHandshakeStart:    func() { r.record(&r.tlsStart) },
		TLSHandshakeDone:     func(tls.ConnectionState, error) { r.record(&r.tlsDone) },
		WroteRequest:         func(httptrace.WroteRequestInfo) { r.record(&r.wroteRequest) },
		GotFirstResponseByte: func() { r.record(&r.firstByte) },
		GotConn: func(info httptrace.GotConnInfo) {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.gotConn = time.Now()
			r.connReused = info.Reused
		},
	}
}

func (r *traceRecorder) record(field *time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	*field = time.Now()
}

func (r *traceRecorder) markBodyRead() {
	r.record(&r.bodyRead)
}

// phases converts the recorded events into timing phases relative to origin.
// Phases the request never reached (e.g. after a failed dial) are omitted.
// finish closes any phase that started but never reported completion (e.g. on timeout).
func (r *traceRecorder) phases(origin, finish time.Time) []timing.Phase {
	r.mu.Lock()
	defer r.mu.Unlock()

	var result []timing.Phase
	connectionPhases := []struct {
		name       string
		start, end time.Time
	}{
		{PhaseDNS, r.dnsStart, r.dnsDone},
		{PhaseConnect, r.connectStart, r.connectDone},
		{PhaseTLS, r.tlsStart, r.tlsDone},
	}
	for _, cp := range connectionPhases {
		if phase, reached := r.connectionPhase(cp.name, origin, cp.start, cp.end, finish); reached {
			result = append(result, phase)
		}
	}

	if !r.wroteRequest.IsZero() && !r.firstByte.IsZero() {
		result = append(result, timing.Measured(PhaseTTFB, origin, r.wroteRequest, r.firstByte))
	}
	if !r.firstByte.IsZero() && !r.bodyRead.IsZero() {
		result = append(result, timing.Measured(PhaseDownload, origin, r.firstByte, r.bodyRead))
	}

	return result
}

func (r *traceRecorder) connectionPhase(name string, origin, start, end, finish time.Time) (timing.Phase, bool) {
	switch {
	case !start.IsZero():
		if end.IsZero() {
			end = finish
		}
		return timing.Measured(name, origin, start, end), true
	case r.gotConn.IsZero():
		return timing.Phase{}, false
	case r.connReused:
		return timing.Unmeasured(name, timing.StatusReused), true
	default:
		return timing.Unmeasured(name, timing.StatusNotApplicable), true
	}
}
