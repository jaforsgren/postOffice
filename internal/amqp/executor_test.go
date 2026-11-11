package amqp

import (
	"context"
	"net"
	"postOffice/internal/postman"
	"testing"
	"time"
)

func phaseNames(t *testing.T, req *postman.Request) []string {
	t.Helper()
	resp := PublishContext(context.Background(), req, nil)
	if resp.Error == nil {
		t.Fatal("expected publish to fail")
	}
	names := make([]string, len(resp.Timing))
	for i, phase := range resp.Timing {
		names[i] = phase.Name
	}
	return names
}

func assertPhaseNames(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("phases = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("phases = %v, want %v", got, want)
		}
	}
}

func TestPublish_DialFailureMeasuresDialOnly(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().String()
	listener.Close()

	got := phaseNames(t, &postman.Request{URL: postman.URL{Raw: "amqp://" + addr + "/queue"}})

	assertPhaseNames(t, got, PhaseDial)
}

func TestPublish_HandshakeFailureMeasuresDialAndHandshake(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr == nil {
			conn.Close()
		}
	}()

	got := phaseNames(t, &postman.Request{URL: postman.URL{Raw: "amqp://" + listener.Addr().String() + "/queue"}})

	assertPhaseNames(t, got, PhaseDial, PhaseHandshake)
}

func TestPublishContext_CancelStopsAStalledHandshake(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		if conn, acceptErr := listener.Accept(); acceptErr == nil {
			accepted <- conn
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		conn := <-accepted
		defer conn.Close()
		cancel()
	}()

	start := time.Now()
	resp := PublishContext(ctx, &postman.Request{URL: postman.URL{Raw: "amqp://" + listener.Addr().String() + "/queue"}}, nil)

	if resp.Error == nil {
		t.Fatal("expected the cancelled publish to fail")
	}
	if elapsed := time.Since(start); elapsed >= defaultTimeout {
		t.Errorf("expected cancel to end the handshake early, took %v", elapsed)
	}
}
