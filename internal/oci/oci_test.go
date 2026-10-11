package oci

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestFanOutEmpty(t *testing.T) {
	results, errs := fanOut(0, func(int) ([]ArtifactInfo, error) {
		t.Fatal("fn must not be called for an empty input")
		return nil, nil
	})
	if len(results) != 0 || len(errs) != 0 {
		t.Errorf("got %v / %v, want empty", results, errs)
	}
}

func TestFanOutPreservesInputOrder(t *testing.T) {
	n := 50
	results, errs := fanOut(n, func(i int) ([]ArtifactInfo, error) {
		return []ArtifactInfo{{Tag: fmt.Sprint(i)}}, nil
	})
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(results) != n {
		t.Fatalf("got %d results, want %d", len(results), n)
	}
	for i, a := range results {
		if a.Tag != fmt.Sprint(i) {
			t.Errorf("results[%d].Tag = %q, want %q", i, a.Tag, fmt.Sprint(i))
		}
	}
}

func TestFanOutCollectsPartialResultsAndErrors(t *testing.T) {
	boom := errors.New("boom")
	results, errs := fanOut(5, func(i int) ([]ArtifactInfo, error) {
		if i%2 == 0 {
			return nil, boom
		}
		return []ArtifactInfo{{Tag: fmt.Sprint(i)}}, nil
	})

	// 3 successes (i=1,3) plus the even indices that failed => 3 results.
	if len(results) != 2 {
		t.Errorf("got %d results, want 2: %v", len(results), results)
	}
	if len(errs) != 3 {
		t.Errorf("got %d errors, want 3: %v", len(errs), errs)
	}
	if !errors.Is(errs[0], boom) {
		t.Errorf("errors not joined: %v", errs)
	}
}

func TestFanOutRespectsConcurrencyBound(t *testing.T) {
	const total = 200
	var inFlight, peak atomic.Int64

	results, _ := fanOut(total, func(int) ([]ArtifactInfo, error) {
		cur := inFlight.Add(1)
		for {
			p := peak.Load()
			if cur <= p || peak.CompareAndSwap(p, cur) {
				break
			}
		}
		// Hold the slot so overlap is observable and the bound is testable.
		time.Sleep(time.Millisecond)
		inFlight.Add(-1)
		return nil, nil
	})

	if len(results) != 0 {
		t.Errorf("got %d results, want none", len(results))
	}
	if p := peak.Load(); p > listConcurrency {
		t.Errorf("peak concurrency %d exceeds bound %d", p, listConcurrency)
	}
}

func TestFanOutRunsEveryIndexExactlyOnce(t *testing.T) {
	const n = 100
	var mu sync.Mutex
	seen := make(map[int]int, n)

	_, _ = fanOut(n, func(i int) ([]ArtifactInfo, error) {
		mu.Lock()
		seen[i]++
		mu.Unlock()
		return nil, nil
	})

	if len(seen) != n {
		t.Errorf("touched %d indexes, want %d", len(seen), n)
	}
	for i, count := range seen {
		if count != 1 {
			t.Errorf("index %d ran %d times, want 1", i, count)
		}
	}
}
