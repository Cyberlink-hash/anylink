package sessdata

import (
	"sync/atomic"
	"testing"
)

func TestOnlineUsernameSearchCaseInsensitive(t *testing.T) {
	old := sessions
	sessions = make(map[string]*Session)
	defer func() { sessions = old }()
	sess := NewSession("")
	sess.Username = "Bob"
	sess.CSess = &ConnSession{dSess: &atomic.Value{}}
	sess.CSess.dSess.Store(&DtlsSession{isActive: -1})
	for _, query := range []string{"bob", "BOB", "b"} {
		result := GetOnlineSess("username", query, true)
		if len(result) != 1 || result[0].Username != "Bob" {
			t.Fatalf("search %s: %+v", query, result)
		}
	}
	if result := GetOnlineSess("username", "alice", true); len(result) != 0 {
		t.Fatal("unmatched user returned")
	}
}
