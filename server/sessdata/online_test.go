package sessdata

import (
	"net"
	"reflect"
	"sort"
	"sync/atomic"
	"testing"
)

func TestOnlineSortGroupsUsersAndOrdersConnections(t *testing.T) {
	datas := Onlines{
		{Username: "bob", Ip: net.ParseIP("192.168.90.1"), Token: "bob-1"},
		{Username: "alice", Ip: net.ParseIP("192.168.90.10"), Token: "alice-10"},
		{Username: "Bob", Ip: net.ParseIP("192.168.90.3"), Token: "bob-3"},
		{Username: "alice", Ip: net.ParseIP("192.168.90.2"), Token: "alice-2"},
		{Username: "bob", Token: "sleeper-b"},
		{Username: "bob", Token: "sleeper-a"},
	}
	want := []string{"alice-2", "alice-10", "sleeper-a", "sleeper-b", "bob-1", "bob-3"}
	for _, reverse := range []bool{false, true} {
		if reverse {
			for i, j := 0, len(datas)-1; i < j; i, j = i+1, j-1 {
				datas[i], datas[j] = datas[j], datas[i]
			}
		}
		sort.Sort(datas)
		var got []string
		for _, row := range datas {
			got = append(got, row.Token)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("unexpected connection order: got %v, want %v", got, want)
		}
	}
}

func TestGetOnlineSessGroupsUsersAfterFiltering(t *testing.T) {
	old := sessions
	sessions = make(map[string]*Session)
	defer func() { sessions = old }()
	for i, username := range []string{"bob", "alice", "bob", "alice"} {
		sess := NewSession("")
		sess.Username = username
		sess.Group = "ops"
		sess.IsActive = true
		sess.CSess = &ConnSession{IpAddr: net.IPv4(192, 168, 90, byte(i+1)), dSess: &atomic.Value{}}
		sess.CSess.dSess.Store(&DtlsSession{isActive: -1})
	}
	result := GetOnlineSess("group", "ops", false)
	var got []string
	for _, row := range result {
		got = append(got, row.Username+":"+row.Ip.String())
	}
	want := []string{"alice:192.168.90.2", "alice:192.168.90.4", "bob:192.168.90.1", "bob:192.168.90.3"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected filtered order: got %v, want %v", got, want)
	}
}

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
