package dbdata

import (
	"reflect"
	"testing"
)

func policyGroupIDs(ids ...int) *[]int {
	if ids == nil {
		ids = []int{}
	}
	return &ids
}
func associationPolicy(name string) NetworkPolicy {
	return NetworkPolicy{Name: name, Usernames: []string{"tester"}, Rules: []GroupLinkAcl{{Action: Deny, Protocol: ALL, Val: "10.50.0.0/16", Port: "0"}}}
}
func loadPolicyGroup(t *testing.T, id int) Group {
	t.Helper()
	var g Group
	if err := One("Id", id, &g); err != nil {
		t.Fatal(err)
	}
	return g
}

func TestNetworkPolicyGroupAssociationOrderAndVisibility(t *testing.T) {
	setupUserManageDB(t)
	a, b, p := associationPolicy("first"), associationPolicy("last"), associationPolicy("shared")
	for _, policy := range []*NetworkPolicy{&a, &b, &p} {
		if err := SetNetworkPolicy(policy); err != nil {
			t.Fatal(err)
		}
	}
	one := Group{Name: "one", Note: "keep", NetworkPolicyIDs: []int{a.Id, p.Id, b.Id}, ClientDns: []ValData{{Val: "10.0.0.53"}}, RouteInclude: []ValData{{Val: "10.0.0.0/8"}}}
	two := Group{Name: "two", NetworkPolicyIDs: []int{b.Id}, RouteInclude: []ValData{{Val: "10.0.0.0/8"}}}
	legacy := Group{Name: "legacy", LinkAcl: []GroupLinkAcl{{NetworkPolicyId: p.Id, Val: "10.0.0.0/8", Port: "0", Protocol: ALL, Action: Deny}}}
	for _, g := range []*Group{&one, &two, &legacy} {
		if err := Add(g); err != nil {
			t.Fatal(err)
		}
	}
	policies := []NetworkPolicy{p}
	if err := FillNetworkPolicyGroups(policies); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(*policies[0].GroupIds, []int{one.Id}) || len(policies[0].Groups) != 2 || policies[0].Groups[0].Order != 2 || !policies[0].Groups[1].Legacy {
		t.Fatalf("bad association display: %+v", policies[0])
	}
	p.GroupIds = policyGroupIDs(one.Id, two.Id)
	p.OriginalGroupIds = policyGroupIDs(one.Id)
	if err := SetNetworkPolicy(&p); err != nil {
		t.Fatal(err)
	}
	updated := loadPolicyGroup(t, one.Id)
	if !samePolicyOrder(updated.NetworkPolicyIDs, one.NetworkPolicyIDs) || updated.Note != one.Note || !reflect.DeepEqual(updated.ClientDns, one.ClientDns) {
		t.Fatal("existing group fields/order changed")
	}
	if !samePolicyOrder(loadPolicyGroup(t, two.Id).NetworkPolicyIDs, []int{b.Id, p.Id}) {
		t.Fatal("new association did not append")
	}
	updated.OriginalNetworkPolicyIDs = policyGroupIDs(a.Id, p.Id, b.Id)
	updated.NetworkPolicyIDs = []int{b.Id, p.Id, a.Id}
	if err := SetGroup(&updated); err != nil {
		t.Fatal("group order edit failed", err)
	}
	p.GroupIds = policyGroupIDs(two.Id)
	p.OriginalGroupIds = policyGroupIDs(one.Id, two.Id)
	if err := SetNetworkPolicy(&p); err != nil {
		t.Fatal(err)
	}
	if !samePolicyOrder(loadPolicyGroup(t, one.Id).NetworkPolicyIDs, []int{b.Id, a.Id}) {
		t.Fatal("unlink reordered other policies")
	}
	if len(loadPolicyGroup(t, legacy.Id).LinkAcl) != 1 {
		t.Fatal("legacy ACL changed")
	}
	if err := DeleteNetworkPolicy(p.Id); err == nil {
		t.Fatal("referenced policy deleted")
	}
	// Omitted association fields preserve associations for older API callers.
	p.GroupIds = nil
	p.OriginalGroupIds = nil
	p.Note = "updated"
	if err := SetNetworkPolicy(&p); err != nil {
		t.Fatal(err)
	}
	if !samePolicyOrder(loadPolicyGroup(t, two.Id).NetworkPolicyIDs, []int{b.Id, p.Id}) {
		t.Fatal("omitted associations removed links")
	}
	// Explicit empty selection removes direct links, but never legacy ACL references.
	p.GroupIds = policyGroupIDs()
	p.OriginalGroupIds = policyGroupIDs(two.Id)
	if err := SetNetworkPolicy(&p); err != nil {
		t.Fatal(err)
	}
	policies = []NetworkPolicy{p}
	if err := FillNetworkPolicyGroups(policies); err != nil {
		t.Fatal(err)
	}
	if len(*policies[0].GroupIds) != 0 || len(policies[0].Groups) != 1 || !policies[0].Groups[0].Legacy {
		t.Fatal("legacy reference not visible after direct unlink")
	}
}

func TestNetworkPolicyGroupAtomicSaveAndStaleEditors(t *testing.T) {
	setupUserManageDB(t)
	one := Group{Name: "one", RouteInclude: []ValData{{Val: "10.0.0.0/8"}}}
	two := Group{Name: "two", RouteInclude: []ValData{{Val: "10.0.0.0/8"}}}
	for _, g := range []*Group{&one, &two} {
		if err := Add(g); err != nil {
			t.Fatal(err)
		}
	}
	p := associationPolicy("new-shared")
	p.GroupIds = policyGroupIDs(one.Id, 99999)
	if err := SetNetworkPolicy(&p); err == nil {
		t.Fatal("missing group accepted")
	}
	if CountAll(&NetworkPolicy{}) != 0 {
		t.Fatal("failed validation saved policy")
	}
	p.GroupIds = policyGroupIDs(one.Id, two.Id)
	if _, err := xdb.Exec(`CREATE TRIGGER fail_group_link BEFORE UPDATE ON "group" WHEN NEW.name = 'two' BEGIN SELECT RAISE(ABORT, 'test rollback'); END`); err != nil {
		t.Fatal(err)
	}
	if err := SetNetworkPolicy(&p); err == nil {
		t.Fatal("expected write failure")
	}
	if CountAll(&NetworkPolicy{}) != 0 || len(loadPolicyGroup(t, one.Id).NetworkPolicyIDs) != 0 {
		t.Fatal("partial association committed")
	}
	if _, err := xdb.Exec(`DROP TRIGGER fail_group_link`); err != nil {
		t.Fatal(err)
	}
	p.Id = 0
	if err := SetNetworkPolicy(&p); err != nil {
		t.Fatal(err)
	}
	stale := p
	stale.Note = "must not save"
	stale.GroupIds = policyGroupIDs(one.Id)
	stale.OriginalGroupIds = policyGroupIDs(one.Id)
	if err := SetNetworkPolicy(&stale); err == nil {
		t.Fatal("stale policy association overwritten")
	}
	var saved NetworkPolicy
	if err := One("Id", p.Id, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Note != "" {
		t.Fatal("stale policy changed content")
	}
	one.OriginalNetworkPolicyIDs = policyGroupIDs()
	if err := SetGroup(&one); err == nil {
		t.Fatal("stale group edit removed new association")
	}
	fresh := loadPolicyGroup(t, one.Id)
	fresh.OriginalNetworkPolicyIDs = policyGroupIDs(p.Id)
	fresh.Note = "valid edit"
	if err := SetGroup(&fresh); err != nil {
		t.Fatal(err)
	}
	if !samePolicyOrder(loadPolicyGroup(t, one.Id).NetworkPolicyIDs, []int{p.Id}) {
		t.Fatal("valid group edit lost link")
	}
	p.GroupIds = policyGroupIDs(one.Id, one.Id)
	p.OriginalGroupIds = nil
	if err := SetNetworkPolicy(&p); err == nil {
		t.Fatal("duplicate group accepted")
	}
}
