package dbdata

import (
	"reflect"
	"testing"
)

func policyScopeFixture(t *testing.T) (User, Group, Group) {
	t.Helper()
	setupUserManageDB(t)
	a := Group{Name: "office", Status: 1, AllowLan: true, ClientDns: []ValData{{Val: "10.0.0.53"}}, NoGlobalDns: true, SplitDns: []ValData{{Val: "corp.example"}}, RouteInclude: []ValData{{Val: "10.0.0.0/8", IpMask: "10.0.0.0/255.0.0.0"}}, DsIncludeDomains: "corp.example"}
	b := Group{Name: "ops", Status: 1, ClientDns: []ValData{{Val: "10.1.0.53"}}, RouteInclude: []ValData{{Val: "10.1.0.0/16", IpMask: "10.1.0.0/255.255.0.0"}}}
	if err := Add(&a); err != nil {
		t.Fatal(err)
	}
	if err := Add(&b); err != nil {
		t.Fatal(err)
	}
	u := User{Username: "scope-user", Status: 1, Groups: []string{a.Name, b.Name}}
	if err := Add(&u); err != nil {
		t.Fatal(err)
	}
	return u, a, b
}
func inheritedPolicy(u User, g int) Policy {
	return Policy{Username: u.Username, GroupId: g, ConfigVersion: 1, Status: 1, LanMode: "inherit", DnsMode: "inherit", RouteMode: "inherit", DomainMode: "inherit"}
}

func TestPolicyScopeAndModuleInheritance(t *testing.T) {
	u, a, b := policyScopeFixture(t)
	legacy := Policy{Username: u.Username, Status: 1, ClientDns: []ValData{{Val: "10.9.0.53"}}, RouteInclude: []ValData{{Val: ALL}}}
	if err := SetPolicy(&legacy); err != nil {
		t.Fatal(err)
	}
	p := inheritedPolicy(u, a.Id)
	if err := SetPolicy(&p); err != nil {
		t.Fatal(err)
	}
	resolved, err := ResolveUserPolicy(u.Username, a.Id)
	if err != nil || resolved == nil || resolved.Id != p.Id {
		t.Fatalf("wrong selection: %+v %v", resolved, err)
	}
	actual := a
	sources := ApplyUserPolicy(&actual, resolved)
	if !reflect.DeepEqual(actual, a) || sources["dns"] != "用户组" {
		t.Fatalf("inherit stacked legacy policy: %+v", actual)
	}
	resolved, err = ResolveUserPolicy(u.Username, b.Id)
	if err != nil || resolved == nil || resolved.Id != legacy.Id {
		t.Fatal("all-group fallback missing", err)
	}
	actual = b
	ApplyUserPolicy(&actual, resolved)
	if actual.RouteInclude[0].Val != ALL || actual.ClientDns[0].Val != "10.9.0.53" {
		t.Fatal("legacy behavior changed")
	}
	p.RouteMode = "custom"
	p.RouteInclude = []ValData{{Val: ALL}}
	if err = SetPolicy(&p); err != nil {
		t.Fatal(err)
	}
	preview, err := PreviewPolicy(u.Username, a.Id, nil)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err = ResolveUserPolicy(u.Username, a.Id)
	if err != nil {
		t.Fatal(err)
	}
	actual = a
	ApplyUserPolicy(&actual, resolved)
	if !reflect.DeepEqual(preview.ClientDns, actual.ClientDns) || !reflect.DeepEqual(preview.RouteInclude, actual.RouteInclude) || preview.NoGlobalDns != actual.NoGlobalDns {
		t.Fatal("preview differs from runtime")
	}
	if preview.Sources["dns"] != "用户组" || preview.Sources["route"] != "指定组用户策略" {
		t.Fatal("incorrect source annotations")
	}
	// DNS custom can explicitly clear inherited split DNS and switch to global DNS.
	p.DnsMode = "custom"
	p.ClientDns = []ValData{{Val: "10.8.0.53"}}
	p.NoGlobalDns = false
	p.SplitDns = nil
	p.DomainMode = "custom"
	if err = SetPolicy(&p); err != nil {
		t.Fatal(err)
	}
	preview, err = PreviewPolicy(u.Username, a.Id, nil)
	if err != nil {
		t.Fatal(err)
	}
	if preview.NoGlobalDns || len(preview.SplitDns) != 0 || preview.DsIncludeDomains != "" {
		t.Fatal("custom clear did not override group")
	}
	// Disabling the specific policy explicitly falls back to the all-groups policy.
	p.Status = 0
	if err = SetPolicy(&p); err != nil {
		t.Fatal(err)
	}
	resolved, err = ResolveUserPolicy(u.Username, a.Id)
	if err != nil || resolved.Id != legacy.Id {
		t.Fatal("disabled policy did not fall back")
	}
}

func TestPolicyScopeValidationAndDraftPreview(t *testing.T) {
	u, a, b := policyScopeFixture(t)
	p := inheritedPolicy(u, a.Id)
	if err := SetPolicy(&p); err != nil {
		t.Fatal(err)
	}
	dup := inheritedPolicy(u, a.Id)
	if err := SetPolicy(&dup); err == nil {
		t.Fatal("duplicate scope allowed")
	}
	other := inheritedPolicy(u, b.Id)
	if err := SetPolicy(&other); err != nil {
		t.Fatal(err)
	}
	invalid := inheritedPolicy(u, 999)
	if err := SetPolicy(&invalid); err == nil {
		t.Fatal("nonexistent group allowed")
	}
	foreign := Group{Name: "foreign", Status: 1}
	if err := Add(&foreign); err != nil {
		t.Fatal(err)
	}
	invalid.GroupId = foreign.Id
	if err := SetPolicy(&invalid); err == nil {
		t.Fatal("unassigned group allowed")
	}
	if _, err := PreviewPolicy(u.Username, foreign.Id, nil); err == nil {
		t.Fatal("unassigned preview allowed")
	}
	p.RouteMode = "custom"
	p.RouteInclude = nil
	if err := SetPolicy(&p); err == nil {
		t.Fatal("empty custom route treated as full tunnel")
	}
	p.RouteInclude = []ValData{{Val: ALL}, {Val: "10.0.0.0/8"}}
	if err := SetPolicy(&p); err == nil {
		t.Fatal("mixed all route accepted")
	}
	p.RouteInclude = []ValData{{Val: ALL}}
	p.DnsMode = "custom"
	p.ClientDns = nil
	if err := SetPolicy(&p); err == nil {
		t.Fatal("full tunnel without DNS accepted")
	}
	p.DnsMode = "inherit"
	before := CountAll(&Policy{})
	preview, err := PreviewPolicy(u.Username, a.Id, &p)
	if err != nil {
		t.Fatal(err)
	}
	if preview.RouteInclude[0].Val != ALL {
		t.Fatal("draft ignored")
	}
	saved := Policy{}
	if err = One("Id", p.Id, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.RouteMode != "inherit" || CountAll(&Policy{}) != before {
		t.Fatal("preview mutated database")
	}
	saved.ConfigVersion = 0
	saved.GroupId = 0
	if err := SetPolicy(&saved); err == nil {
		t.Fatal("new policy downgraded to legacy")
	}
}

func TestPolicyScopeLegacySchemaUpgrade(t *testing.T) {
	setupUserManageDB(t)
	if err := xdb.DropTables(&Policy{}); err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		`CREATE TABLE policy (id INTEGER PRIMARY KEY AUTOINCREMENT, username TEXT NOT NULL, status INTEGER, client_dns TEXT, route_include TEXT, split_dns TEXT, no_global_dns INTEGER)`,
		`CREATE UNIQUE INDEX UQE_policy_username ON policy (username)`,
		`INSERT INTO policy (username,status,client_dns,route_include,split_dns,no_global_dns) VALUES ('old-user',1,'[{"val":"10.0.0.53"}]','[{"val":"all"}]','[]',0)`,
	} {
		if _, err := xdb.Exec(sql); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		if err := xdb.Sync2(&Policy{}); err != nil {
			t.Fatal(err)
		}
		if err := migratePolicyScope(); err != nil {
			t.Fatal(err)
		}
	}
	legacy := GetPolicy("old-user")
	if legacy.Id != 1 || legacy.ConfigVersion != 0 || legacy.GroupId != 0 || legacy.ClientDns[0].Val != "10.0.0.53" {
		t.Fatalf("migration changed data: %+v", legacy)
	}
	g := Group{NoGlobalDns: true, SplitDns: []ValData{{Val: "corp.example"}}, ClientDns: []ValData{{Val: "10.1.0.53"}}}
	ApplyUserPolicy(&g, legacy)
	if !g.NoGlobalDns || g.SplitDns[0].Val != "corp.example" || g.ClientDns[0].Val != "10.0.0.53" || g.RouteInclude[0].Val != ALL {
		t.Fatal("legacy DNS or routing behavior changed")
	}
	scoped := Policy{Username: "old-user", GroupId: 7, ConfigVersion: 1}
	if err := Add(&scoped); err != nil {
		t.Fatal("old unique username index still blocks scope", err)
	}
	duplicate := Policy{Username: "old-user", GroupId: 7}
	if err := Add(&duplicate); err == nil {
		t.Fatal("composite uniqueness missing")
	}
}
