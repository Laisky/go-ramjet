package session

import (
	"sync"
	"testing"
)

func TestRawStashBasic(t *testing.T) {
	t.Parallel()
	s := NewRawStash()
	if _, ok := s.Get("missing"); ok {
		t.Fatal("Get on empty stash should miss")
	}
	s.Stash("call_1", "raw bytes 1")
	got, ok := s.Get("call_1")
	if !ok || got != "raw bytes 1" {
		t.Fatalf("Get returned %q,%v", got, ok)
	}
	if s.Len() != 1 {
		t.Fatalf("Len: want 1 got %d", s.Len())
	}
}

func TestRawStashOverwrite(t *testing.T) {
	t.Parallel()
	s := NewRawStash()
	s.Stash("c", "first")
	s.Stash("c", "second")
	got, _ := s.Get("c")
	if got != "second" {
		t.Fatalf("expected overwrite to win; got %q", got)
	}
	if s.Len() != 1 {
		t.Fatalf("Len after overwrite: want 1 got %d", s.Len())
	}
}

func TestRawStashNilSafe(t *testing.T) {
	t.Parallel()
	var s *RawStash
	s.Stash("c", "x") // must not panic
	if v, ok := s.Get("c"); ok || v != "" {
		t.Fatalf("nil stash Get should miss; got %q,%v", v, ok)
	}
	if s.Len() != 0 {
		t.Fatalf("nil stash Len: want 0 got %d", s.Len())
	}
}

func TestRawStashEmptyCallIDIgnored(t *testing.T) {
	t.Parallel()
	s := NewRawStash()
	s.Stash("", "ignored")
	if s.Len() != 0 {
		t.Fatalf("empty callID should be ignored; got Len=%d", s.Len())
	}
	if _, ok := s.Get(""); ok {
		t.Fatal("empty callID Get should miss")
	}
}

func TestRawStashConcurrent(t *testing.T) {
	t.Parallel()
	s := NewRawStash()
	var wg sync.WaitGroup
	const n = 50
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			s.Stash(callID(i), "v")
		}(i)
	}
	wg.Wait()
	if s.Len() != n {
		t.Fatalf("Len after %d goroutines: want %d got %d", n, n, s.Len())
	}
}

func callID(i int) string {
	const hex = "0123456789abcdef"
	return "call_" + string([]byte{hex[i>>4], hex[i&0xf]})
}
