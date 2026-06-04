package store

import (
	"errors"
	"testing"
	"time"
)

func TestSessionByToken(t *testing.T) {
	s := newTestStore(t)

	uid, err := s.CreateUser("alice", "hash1")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	want := time.Now().Add(SessionTTLForTest()).UTC().Truncate(time.Second)
	if err := s.CreateSession("tok123", uid, want); err != nil {
		t.Fatalf("create session: %v", err)
	}

	sess, u, err := s.SessionByToken("tok123")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if sess.Token != "tok123" || sess.UserID != uid {
		t.Fatalf("unexpected session: %+v", sess)
	}
	if !sess.ExpiresAt.Equal(want) {
		t.Fatalf("expires_at did not round-trip: got %v want %v", sess.ExpiresAt, want)
	}
	if u.ID != uid || u.Username != "alice" {
		t.Fatalf("joined user wrong: %+v", u)
	}

	if _, _, err := s.SessionByToken("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing token: want ErrNotFound, got %v", err)
	}
}

// SessionTTLForTest keeps the test independent of the production TTL constant's
// exact value while still exercising a realistic future expiry.
func SessionTTLForTest() time.Duration { return 30 * 24 * time.Hour }
