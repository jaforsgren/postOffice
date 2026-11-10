package http

import (
	"net/http"
	"net/http/httptest"
	"postOffice/internal/postman"
	"postOffice/internal/timing"
	"strings"
	"testing"
	"time"
)

func newTimingServer(handler http.HandlerFunc, useTLS bool) *httptest.Server {
	if useTLS {
		return httptest.NewTLSServer(handler)
	}
	return httptest.NewServer(handler)
}

func okHandler(w http.ResponseWriter, _ *http.Request) {
	w.Write([]byte("ok"))
}

func executeGET(t *testing.T, executor *Executor, url string) *Response {
	t.Helper()
	resp, _ := executor.Execute(&postman.Request{Method: "GET", URL: postman.URL{Raw: url}}, nil, nil, nil, nil)
	if resp.Error != nil {
		t.Fatalf("request failed: %v", resp.Error)
	}
	return resp
}

func phasesByName(phases []timing.Phase) map[string]timing.Phase {
	byName := make(map[string]timing.Phase, len(phases))
	for _, phase := range phases {
		byName[phase.Name] = phase
	}
	return byName
}

func assertPhaseStatus(t *testing.T, phases map[string]timing.Phase, name string, want timing.Status) {
	t.Helper()
	phase, ok := phases[name]
	if !ok {
		t.Fatalf("phase %q missing", name)
	}
	if phase.Status != want {
		t.Errorf("phase %q status = %v, want %v", name, phase.Status, want)
	}
}

func TestExecute_TimingNewPlainConnection(t *testing.T) {
	server := newTimingServer(okHandler, false)
	defer server.Close()

	resp := executeGET(t, NewExecutor(), server.URL)
	phases := phasesByName(resp.Timing)

	assertPhaseStatus(t, phases, PhaseDNS, timing.StatusNotApplicable)
	assertPhaseStatus(t, phases, PhaseConnect, timing.StatusMeasured)
	assertPhaseStatus(t, phases, PhaseTLS, timing.StatusNotApplicable)
	assertPhaseStatus(t, phases, PhaseTTFB, timing.StatusMeasured)
	assertPhaseStatus(t, phases, PhaseDownload, timing.StatusMeasured)
}

func TestExecute_TimingNewTLSConnection(t *testing.T) {
	server := newTimingServer(okHandler, true)
	defer server.Close()

	executor := NewExecutor()
	executor.client = server.Client()

	resp := executeGET(t, executor, server.URL)
	phases := phasesByName(resp.Timing)

	assertPhaseStatus(t, phases, PhaseConnect, timing.StatusMeasured)
	assertPhaseStatus(t, phases, PhaseTLS, timing.StatusMeasured)
	if phases[PhaseTLS].Offset < phases[PhaseConnect].Offset+phases[PhaseConnect].Duration {
		t.Errorf("TLS handshake should start after TCP connect finishes: %+v", resp.Timing)
	}
}

func TestExecute_TimingReusedConnection(t *testing.T) {
	for _, useTLS := range []bool{false, true} {
		server := newTimingServer(okHandler, useTLS)

		executor := NewExecutor()
		executor.client = server.Client()

		executeGET(t, executor, server.URL)
		resp := executeGET(t, executor, server.URL)
		phases := phasesByName(resp.Timing)

		assertPhaseStatus(t, phases, PhaseDNS, timing.StatusReused)
		assertPhaseStatus(t, phases, PhaseConnect, timing.StatusReused)
		assertPhaseStatus(t, phases, PhaseTLS, timing.StatusReused)
		assertPhaseStatus(t, phases, PhaseTTFB, timing.StatusMeasured)
		assertPhaseStatus(t, phases, PhaseDownload, timing.StatusMeasured)

		server.Close()
	}
}

func TestExecute_TimingHostnameResolvesDNS(t *testing.T) {
	server := newTimingServer(okHandler, false)
	defer server.Close()

	url := strings.Replace(server.URL, "127.0.0.1", "localhost", 1)
	resp := executeGET(t, NewExecutor(), url)

	assertPhaseStatus(t, phasesByName(resp.Timing), PhaseDNS, timing.StatusMeasured)
}

func TestExecute_TimingPhasesFollowWaterfallOrder(t *testing.T) {
	const serverDelay = 20 * time.Millisecond
	server := newTimingServer(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(serverDelay)
		okHandler(w, r)
	}, false)
	defer server.Close()

	resp := executeGET(t, NewExecutor(), server.URL)
	phases := phasesByName(resp.Timing)

	ttfb := phases[PhaseTTFB]
	if ttfb.Duration < serverDelay {
		t.Errorf("TTFB = %v, want >= server delay %v", ttfb.Duration, serverDelay)
	}
	download := phases[PhaseDownload]
	if download.Offset != ttfb.Offset+ttfb.Duration {
		t.Errorf("download should start at first byte: ttfb=%+v download=%+v", ttfb, download)
	}
	if end := download.Offset + download.Duration; end > resp.Duration {
		t.Errorf("phases end at %v, beyond total %v", end, resp.Duration)
	}
}

func TestExecute_TimingOmitsUnreachedPhasesOnFailure(t *testing.T) {
	server := newTimingServer(okHandler, false)
	url := server.URL
	server.Close()

	resp, _ := NewExecutor().Execute(&postman.Request{Method: "GET", URL: postman.URL{Raw: url}}, nil, nil, nil, nil)
	if resp.Error == nil {
		t.Fatal("expected connection error")
	}

	phases := phasesByName(resp.Timing)
	assertPhaseStatus(t, phases, PhaseConnect, timing.StatusMeasured)
	for _, unreached := range []string{PhaseTLS, PhaseTTFB, PhaseDownload} {
		if _, ok := phases[unreached]; ok {
			t.Errorf("phase %q should be omitted when the connection fails", unreached)
		}
	}
}
