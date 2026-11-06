package dbdata

import "testing"

func TestNetworkPolicyDynamicGroupMembers(t *testing.T) {
	setupUserManageDB(t)
	a := Group{Name: "dev", Status: 1}
	b := Group{Name: "ops", Status: 1}
	for _, g := range []*Group{&a, &b} {
		if err := Add(g); err != nil {
			t.Fatal(err)
		}
	}
	users := []User{
		{Username: "alice", Status: 1, Groups: []string{a.Name}},
		{Username: "bob", Status: 1, Groups: []string{b.Name}},
		{Username: "both", Status: 1, Groups: []string{a.Name, b.Name}},
		{Username: "outside", Status: 1, Groups: []string{"other"}},
	}
	for i := range users {
		if err := Add(&users[i]); err != nil {
			t.Fatal(err)
		}
	}
	p := associationPolicy("dynamic-members")
	p.UserMode = NetworkPolicyGroupMembers
	p.Usernames = nil
	p.GroupIds = policyGroupIDs(a.Id, b.Id)
	if err := SetNetworkPolicy(&p); err != nil {
		t.Fatal("group-only policy rejected", err)
	}
	a = loadPolicyGroup(t, a.Id)
	b = loadPolicyGroup(t, b.Id)
	check := func(group Group, user string, want bool) {
		t.Helper()
		matches, _ := LoadNetworkPolicies(&group, user, "192.0.2.1:1234")
		if matches[p.Id] != want {
			t.Fatalf("%s in %s: %v want %v", user, group.Name, matches, want)
		}
	}
	check(a, "alice", true)
	check(b, "bob", true)
	check(a, "both", true)
	check(b, "both", true)
	check(a, "outside", false)
	p.ExcludedUsernames = []string{" Both ", "both", ""}
	if err := SetNetworkPolicy(&p); err != nil {
		t.Fatal(err)
	}
	if len(p.ExcludedUsernames) != 1 || p.ExcludedUsernames[0] != "both" {
		t.Fatal("exclusions not normalized")
	}
	check(a, "both", false)
	check(b, "both", false)
	check(a, "alice", true)
	newcomer := User{Username: "new-member", Status: 1, Groups: []string{a.Name, b.Name}}
	if err := Add(&newcomer); err != nil {
		t.Fatal(err)
	}
	check(a, newcomer.Username, true)
	check(b, newcomer.Username, true)
	// Changes to current membership are picked up on the next tunnel, without resaving policy.
	users[0].Groups = []string{b.Name}
	if err := Set(&users[0]); err != nil {
		t.Fatal(err)
	}
	check(a, "alice", false)
	check(b, "alice", true)
	// Unchecking everyone must skip all current users, never mean unrestricted.
	p.ExcludedUsernames = []string{"alice", "bob", "both", "new-member"}
	if err := SetNetworkPolicy(&p); err != nil {
		t.Fatal(err)
	}
	check(b, "alice", false)
	check(b, "bob", false)
	check(a, "both", false)
	check(a, "new-member", false)
	p.SourceIPs = []string{"198.51.100.0/24"}
	p.ExcludedUsernames = nil
	if err := SetNetworkPolicy(&p); err != nil {
		t.Fatal(err)
	}
	check(a, "both", false)
	matches, _ := LoadNetworkPolicies(&a, "both", "198.51.100.3:443")
	if !matches[p.Id] {
		t.Fatal("source condition rejected matching member")
	}
	if p.Matches("both", "198.51.100.3:443") {
		t.Fatal("member context not required")
	}
	p.UserMode = ""
	p.Usernames = []string{"both"}
	if err := SetNetworkPolicy(&p); err == nil {
		t.Fatal("old client downgraded group mode")
	}
}

func TestNetworkPolicyMemberModeKeepsLegacySemantics(t *testing.T) {
	setupUserManageDB(t)
	p := associationPolicy("legacy-members")
	if err := SetNetworkPolicy(&p); err != nil {
		t.Fatal(err)
	}
	if !p.Matches("tester", "") || p.Matches("new-user", "") {
		t.Fatal("legacy whitelist changed")
	}
	p.ExcludedUsernames = []string{"tester"}
	if err := SetNetworkPolicy(&p); err == nil {
		t.Fatal("mixed modes accepted")
	}
	p.ExcludedUsernames = nil
	p.UserMode = "unknown"
	if err := SetNetworkPolicy(&p); err == nil {
		t.Fatal("unknown mode accepted")
	}
	p.UserMode = NetworkPolicyGroupMembers
	if err := SetNetworkPolicy(&p); err == nil {
		t.Fatal("fixed list accepted in group mode")
	}
	p.Usernames = nil
	if err := SetNetworkPolicy(&p); err != nil {
		t.Fatal("explicit conversion rejected", err)
	}
	var saved NetworkPolicy
	if err := One("Id", p.Id, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.UserMode != NetworkPolicyGroupMembers {
		t.Fatal("mode not persisted")
	}
}
