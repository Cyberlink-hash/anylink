package dbdata

import (
	"path/filepath"
	"testing"

	"github.com/bjdgyc/anylink/base"
)

func TestNetworkPolicyMatches(t *testing.T) {
	p := NetworkPolicy{Usernames: []string{"leader", "backup"}, SourceIPs: []string{"203.0.113.0/24", "2001:db8::1"}}
	for _, tc := range []struct {
		name, user, remote string
		want               bool
	}{
		{"leader office", "leader", "203.0.113.12:1234", true},
		{"second user", "backup", "203.0.113.12:1234", true},
		{"ordinary user", "member", "203.0.113.12:1234", false},
		{"home", "leader", "198.51.100.1:1234", false},
		{"ipv6", "leader", "[2001:db8::1]:1234", true},
		{"invalid source", "leader", "invalid", false},
		{"exact username", "Leader", "203.0.113.12:1234", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := p.Matches(tc.user, tc.remote); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
	if (&NetworkPolicy{}).Matches("leader", "203.0.113.12:1234") {
		t.Fatal("empty policy allowed")
	}
	if !(&NetworkPolicy{Usernames: []string{"leader"}}).Matches("leader", "invalid") {
		t.Fatal("optional source was required")
	}
	if !(&NetworkPolicy{SourceIPs: []string{"203.0.113.12"}}).Matches("member", "203.0.113.12:1234") {
		t.Fatal("optional username was required")
	}
}

func TestNetworkPolicyLifecycle(t *testing.T) {
	oldType, oldSource := base.Cfg.DbType, base.Cfg.DbSource
	base.Cfg.DbType = "sqlite3"
	base.Cfg.DbSource = filepath.Join(t.TempDir(), "policy.db")
	initDb()
	defer func() { xdb.Close(); base.Cfg.DbType, base.Cfg.DbSource = oldType, oldSource }()
	p := NetworkPolicy{Rules: []GroupLinkAcl{{Action: Allow, Protocol: TCP, Val: "10.0.0.0/8", Port: "443"}}, Name: "leaders", Usernames: []string{" leader ", "leader"}, SourceIPs: []string{"203.0.113.0/24"}}
	if err := SetNetworkPolicy(&p); err != nil {
		t.Fatal(err)
	}
	if len(p.Usernames) != 1 || p.Usernames[0] != "leader" {
		t.Fatal("not normalized")
	}
	for _, bad := range []NetworkPolicy{{Name: "empty"}, {Name: "bad", SourceIPs: []string{"not-ip"}}, {Id: -1, Name: "bad", Usernames: []string{"u"}}} {
		if SetNetworkPolicy(&bad) == nil {
			t.Fatal("invalid policy accepted")
		}
	}
	groups := []*Group{
		{Name: "products", RouteInclude: []ValData{{Val: "10.0.0.0/8"}}, LinkAcl: []GroupLinkAcl{{Val: "10.1.0.0/16", Protocol: TCP, Port: "443", Action: Allow, NetworkPolicyId: p.Id}}},
		{Name: "engineering", RouteInclude: []ValData{{Val: "10.0.0.0/8"}}, LinkAcl: []GroupLinkAcl{{Val: "10.2.0.0/16", Protocol: TCP, Port: "22", Action: Allow, NetworkPolicyId: p.Id}}},
	}
	for _, g := range groups {
		if err := SetGroup(g); err != nil {
			t.Fatal(err)
		}
		if !MatchNetworkPolicies(g, "leader", "203.0.113.1:1234")[p.Id] {
			t.Fatal("shared policy not matched")
		}
	}
	groups[0].NetworkPolicyIDs = []int{p.Id}
	if err := SetGroup(groups[0]); err != nil {
		t.Fatal(err)
	}
	matches, loaded := LoadNetworkPolicies(groups[0], "leader", "203.0.113.1:1234")
	if !matches[p.Id] || len(loaded[p.Id]) != 1 {
		t.Fatal("rules snapshot missing")
	}
	snapshot := MatchNetworkPolicies(groups[0], "leader", "203.0.113.1:1234")
	p.Usernames = []string{"replacement"}
	if err := SetNetworkPolicy(&p); err != nil {
		t.Fatal(err)
	}
	if !snapshot[p.Id] || MatchNetworkPolicies(groups[0], "leader", "203.0.113.1:1234")[p.Id] {
		t.Fatal("reconnect snapshot incorrect")
	}
	if DeleteNetworkPolicy(p.Id) == nil {
		t.Fatal("referenced policy deleted")
	}
	groups[0].LinkAcl = nil
	groups[0].NetworkPolicyIDs = nil
	if err := SetGroup(groups[0]); err != nil {
		t.Fatal(err)
	}
	if DeleteNetworkPolicy(p.Id) == nil {
		t.Fatal("second group reference ignored")
	}
	groups[1].LinkAcl = nil
	if err := SetGroup(groups[1]); err != nil {
		t.Fatal(err)
	}
	if err := DeleteNetworkPolicy(p.Id); err != nil {
		t.Fatal(err)
	}
	groups[0].LinkAcl = []GroupLinkAcl{{NetworkPolicyId: p.Id, Val: "10.0.0.0/8", Port: "443"}}
	if SetGroup(groups[0]) == nil {
		t.Fatal("dangling reference saved")
	}
	if MatchNetworkPolicies(groups[0], "leader", "203.0.113.1:1234") != nil {
		t.Fatal("missing policy not detected")
	}
}

func TestNetworkRulesValidation(t *testing.T) {
	for _, port := range []string{"65535", "65534-65535", "80,443", "0"} {
		rules := []GroupLinkAcl{{Action: Allow, Protocol: TCP, Val: "10.30.0.10", Port: port}}
		if err := ValidateNetworkRules(rules); err != nil {
			t.Fatal(err)
		}
		if rules[0].Val != "10.30.0.10/32" {
			t.Fatal("single IP not normalized")
		}
	}
	for _, rule := range []GroupLinkAcl{
		{Action: Allow, Protocol: TCP, Val: "10.30.0.1/24", Port: "443"},
		{Action: Allow, Protocol: TCP, Val: "10.30.0.0/24", Port: "65536"},
		{Action: Allow, Protocol: TCP, Val: "10.30.0.0/24", Port: "100-90"},
		{Action: "bad", Protocol: TCP, Val: "10.30.0.0/24", Port: "443"},
	} {
		if ValidateNetworkRules([]GroupLinkAcl{rule}) == nil {
			t.Fatal("invalid rule accepted")
		}
	}
}
