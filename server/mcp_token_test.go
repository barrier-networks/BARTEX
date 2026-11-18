package server

import (
	"testing"
	"time"
)

func TestMCPTokenMintResolve(t *testing.T) {
	s := newMCPTokenStore()
	tok, exp, err := s.mint("42")
	if err != nil {
		t.Fatal(err)
	}
	if tok == "" || exp.Before(time.Now()) {
		t.Fatalf("mint returned token=%q exp=%v", tok, exp)
	}
	if id, ok := s.resolve(tok); !ok || id != "42" {
		t.Fatalf("resolve = %q,%v", id, ok)
	}
	// The raw token is never stored; a different token must not resolve.
	if _, ok := s.resolve("not-the-token"); ok {
		t.Error("an unrelated token resolved")
	}
	if _, ok := s.resolve(""); ok {
		t.Error("empty token resolved")
	}
}

func TestMCPTokenExpiry(t *testing.T) {
	s := newMCPTokenStore()
	tok, _, _ := s.mint("7")
	// Force the stored entry to be expired.
	s.mu.Lock()
	for k := range s.m {
		s.m[k] = mcpTokenEntry{taskID: "7", expires: time.Now().Add(-time.Minute)}
	}
	s.mu.Unlock()
	if _, ok := s.resolve(tok); ok {
		t.Error("expired token resolved")
	}
	// Resolution of an expired token also evicts it.
	s.mu.Lock()
	_, present := s.m[hashToken(tok)]
	s.mu.Unlock()
	if present {
		t.Error("expired token was not evicted on resolve")
	}
}

func TestMCPTokenRevokeTask(t *testing.T) {
	s := newMCPTokenStore()
	a, _, _ := s.mint("1")
	b, _, _ := s.mint("1")
	c, _, _ := s.mint("2")
	s.revokeTask("1")
	if _, ok := s.resolve(a); ok {
		t.Error("token a for task 1 still resolves after revoke")
	}
	if _, ok := s.resolve(b); ok {
		t.Error("token b for task 1 still resolves after revoke")
	}
	if id, ok := s.resolve(c); !ok || id != "2" {
		t.Error("revoking task 1 wrongly dropped task 2's token")
	}
}
