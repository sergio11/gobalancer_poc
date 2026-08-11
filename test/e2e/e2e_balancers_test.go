package e2e

import (
	"net/http"
	"testing"
	"time"
)

// TestE2E_WeightedRoundRobin_Distribution verifies traffic is distributed
// proportionally to backend weights (5:3:1 over 9 requests => 5/3/1).
func TestE2E_WeightedRoundRobin_Distribution(t *testing.T) {
	b1 := newMockBackend(t, "1", nil)
	b2 := newMockBackend(t, "2", nil)
	b3 := newMockBackend(t, "3", nil)

	env := setupTestLB(t, lbOptions{
		algorithm: "weighted-round-robin",
		backends:  []*mockBackend{b1, b2, b3},
		weights:   []int{5, 3, 1},
	})

	for i := 0; i < 9; i++ {
		env.get("/test", "")
	}

	if b1.hitCount() != 5 || b2.hitCount() != 3 || b3.hitCount() != 1 {
		t.Errorf("expected weighted distribution 5/3/1, got b1=%d, b2=%d, b3=%d",
			b1.hitCount(), b2.hitCount(), b3.hitCount())
	}
}

// TestE2E_IPHash_SessionAffinity verifies IP Hash keeps the same client IP
// pinned to the same backend and is deterministic for repeated requests.
func TestE2E_IPHash_SessionAffinity(t *testing.T) {
	b1 := newMockBackend(t, "1", nil)
	b2 := newMockBackend(t, "2", nil)
	b3 := newMockBackend(t, "3", nil)

	env := setupTestLB(t, lbOptions{
		algorithm: "ip-hash",
		backends:  []*mockBackend{b1, b2, b3},
	})

	recordHits := func(xff string) map[*mockBackend]int {
		before := map[*mockBackend]int{
			b1: int(b1.hitCount()),
			b2: int(b2.hitCount()),
			b3: int(b3.hitCount()),
		}
		for i := 0; i < 6; i++ {
			env.get("/test", xff)
		}
		return map[*mockBackend]int{
			b1: int(b1.hitCount()) - before[b1],
			b2: int(b2.hitCount()) - before[b2],
			b3: int(b3.hitCount()) - before[b3],
		}
	}

	// The same IP must always hit the exact same backend.
	first := recordHits("10.0.0.1")
	second := recordHits("10.0.0.1")
	if first[b1] != 6 && first[b2] != 6 && first[b3] != 6 {
		t.Fatalf("expected all 6 requests from one IP to land on a single backend, got %+v", first)
	}
	for k := range first {
		if first[k] != second[k] {
			t.Errorf("IP Hash not deterministic: first=%+v second=%+v", first, second)
		}
	}

	// A different IP must also be sticky (not necessarily a different backend).
	other := recordHits("10.0.0.2")
	if other[b1] != 6 && other[b2] != 6 && other[b3] != 6 {
		t.Fatalf("expected all 6 requests from a second IP to land on a single backend, got %+v", other)
	}
}

// TestE2E_LeastConnections_RoutesToFreeBackends verifies that a backend with
// an active (held) connection receives no new traffic while others are idle.
//
// Round-robin would keep cycling through all three backends, so the busy
// backend receiving zero new requests proves the algorithm picks idle ones.
func TestE2E_LeastConnections_RoutesToFreeBackends(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})

	b1 := newMockBackend(t, "1", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-entered:
		default:
			close(entered)
		}
		<-release
		w.WriteHeader(200)
	})
	b2 := newMockBackend(t, "2", nil)
	b3 := newMockBackend(t, "3", nil)

	env := setupTestLB(t, lbOptions{
		algorithm: "least-connections",
		backends:  []*mockBackend{b1, b2, b3},
	})

	// 1. Fire a request that blocks on b1 and wait until it is in-flight.
	done := make(chan struct{})
	go func() {
		defer close(done)
		env.get("/slow", "")
	}()

	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("request did not reach the blocking backend in time")
	}

	// 2. b1 now holds one active connection; new requests must avoid it.
	for i := 0; i < 3; i++ {
		env.get("/test", "")
	}

	if b1.hitCount() != 1 {
		t.Errorf("blocked backend should have received only the held request, got %d", b1.hitCount())
	}
	if b2.hitCount() != 3 {
		t.Errorf("idle backends should absorb the new requests, got b2=%d, b3=%d",
			b2.hitCount(), b3.hitCount())
	}

	// 3. Release the held request.
	close(release)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("held request did not complete after release")
	}
}
