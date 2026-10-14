package handler

import (
	"encoding/binary"
	"net"
	"testing"

	"github.com/bjdgyc/anylink/dbdata"
	"github.com/bjdgyc/anylink/sessdata"
	"github.com/songgao/water/waterutil"
)

func TestNetworkPolicyACL(t *testing.T) {
	_, network, _ := net.ParseCIDR("10.20.0.0/24")
	g := &dbdata.Group{LinkAcl: []dbdata.GroupLinkAcl{
		{NetworkPolicyId: 1, Action: dbdata.Allow, Protocol: dbdata.TCP, IpProto: waterutil.TCP, IpNet: network, Ports: map[uint16]int8{22: 1}},
		{Action: dbdata.Deny, Protocol: dbdata.ALL, IpNet: network, Ports: map[uint16]int8{0: 1}},
	}}
	packet := make([]byte, 40)
	packet[0] = 0x45
	packet[9] = byte(waterutil.TCP)
	copy(packet[16:20], net.ParseIP("10.20.0.10").To4())
	binary.BigEndian.PutUint16(packet[22:24], 22)
	pl := &sessdata.Payload{LType: sessdata.LTypeIPData, Data: packet}
	for _, tc := range []struct {
		name     string
		policies map[int]bool
		want     bool
	}{
		{"matched leader", map[int]bool{1: true}, true},
		{"unmatched member or source", map[int]bool{1: false}, false},
		{"missing policy", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if checkLinkAcl(g, pl, tc.policies) != tc.want {
				t.Fatal("unexpected ACL result")
			}
		})
	}
	// 条件不满足时应继续匹配，而不是阻断整个连接。
	g.LinkAcl[1].Action = dbdata.Allow
	if !checkLinkAcl(g, pl, map[int]bool{1: false}) {
		t.Fatal("unmatched condition did not continue")
	}
	g.LinkAcl[1].Action = dbdata.Deny
	g.LinkAcl[0].NetworkPolicyId = 0
	if !checkLinkAcl(g, pl, nil) {
		t.Fatal("legacy rule changed")
	}
	g.LinkAcl[0].NetworkPolicyId = 1
	g.ClientDns = []dbdata.ValData{{Val: "10.20.0.10"}}
	binary.BigEndian.PutUint16(packet[22:24], 53)
	if !checkLinkAcl(g, pl, map[int]bool{1: false}) {
		t.Fatal("DNS exception changed")
	}
	g.LinkAcl = nil
	if !checkLinkAcl(g, pl, nil) {
		t.Fatal("empty legacy ACL changed")
	}
}

func TestNetworkPolicyRoutesAndSources(t *testing.T) {
	rules := map[int][]dbdata.GroupLinkAcl{
		1: {{Action: dbdata.Allow, Protocol: dbdata.TCP, Val: "10.30.0.10", Port: "443"}},
		2: {{Action: dbdata.Allow, Protocol: dbdata.TCP, Val: "10.30.0.10", Port: "443"}},
		3: {{Action: dbdata.Deny, Protocol: dbdata.TCP, Val: "10.30.0.10", Port: "443"}},
	}
	for _, list := range rules {
		if err := dbdata.ValidateNetworkRules(list); err != nil {
			t.Fatal(err)
		}
	}
	policies := map[int]dbdata.NetworkPolicy{
		1: {Usernames: []string{"zhangsan"}},
		2: {Usernames: []string{"lisi"}, SourceIPs: []string{"203.0.113.1"}},
		3: {Usernames: []string{"lisi"}},
	}
	packet := make([]byte, 40)
	packet[0] = 0x45
	packet[9] = byte(waterutil.TCP)
	copy(packet[16:20], net.ParseIP("10.30.0.10").To4())
	binary.BigEndian.PutUint16(packet[22:24], 443)
	pl := &sessdata.Payload{LType: sessdata.LTypeIPData, Data: packet}
	g := &dbdata.Group{RouteInclude: []dbdata.ValData{{Val: "10.30.0.0/24"}}, NetworkPolicyIDs: []int{1, 2, 3}}
	for _, tc := range []struct {
		name, user, source string
		want               bool
	}{
		{"zhangsan anywhere", "zhangsan", "198.51.100.1:1234", true},
		{"lisi office", "lisi", "203.0.113.1:1234", true},
		{"lisi home", "lisi", "198.51.100.1:1234", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			matches := map[int]bool{}
			for id, policy := range policies {
				matches[id] = policy.Matches(tc.user, tc.source)
			}
			if checkLinkAcl(g, pl, matches, rules) != tc.want {
				t.Fatal("access result mismatch")
			}
		})
	}
	matches := map[int]bool{1: true, 2: false, 3: false}
	g.RouteInclude = []dbdata.ValData{{Val: "10.10.0.0/24"}}
	if checkLinkAcl(g, pl, matches, rules) {
		t.Fatal("policy escaped route boundary")
	}
	g.RouteInclude = []dbdata.ValData{{Val: "all"}}
	g.RouteExclude = []dbdata.ValData{{Val: "10.30.0.0/24"}}
	if checkLinkAcl(g, pl, matches, rules) {
		t.Fatal("excluded route allowed")
	}
	g.ClientDns = []dbdata.ValData{{Val: "10.30.0.10"}}
	binary.BigEndian.PutUint16(packet[22:24], 53)
	if checkLinkAcl(g, pl, matches, rules) {
		t.Fatal("DNS escaped excluded route")
	}
	g.RouteExclude = nil
	binary.BigEndian.PutUint16(packet[22:24], 443)
	g.NetworkPolicyIDs = []int{3, 1}
	if checkLinkAcl(g, pl, map[int]bool{1: true, 3: true}, rules) {
		t.Fatal("policy order ignored")
	}
	binary.BigEndian.PutUint16(packet[6:8], 1)
	if checkLinkAcl(g, pl, matches, rules) {
		t.Fatal("fragment bypass")
	}
}

func TestNetworkPolicyDoesNotChangeRoutes(t *testing.T) {
	group := &dbdata.Group{RouteInclude: []dbdata.ValData{{Val: "10.10.0.0/24"}}, RouteExclude: []dbdata.ValData{{Val: "10.10.0.20/32"}}}
	packet := make([]byte, 40)
	packet[0] = 0x45
	packet[9] = byte(waterutil.TCP)
	copy(packet[16:20], net.ParseIP("10.30.0.10").To4())
	binary.BigEndian.PutUint16(packet[22:24], 443)
	pl := &sessdata.Payload{LType: sessdata.LTypeIPData, Data: packet}
	if !checkLinkAcl(group, pl, nil) {
		t.Fatal("unconfigured group behavior changed")
	}
	group.NetworkPolicyIDs = []int{1}
	rules := map[int][]dbdata.GroupLinkAcl{1: {{Action: dbdata.Allow, Protocol: dbdata.TCP, Val: "10.30.0.0/24", Port: "443"}}}
	if err := dbdata.ValidateNetworkRules(rules[1]); err != nil {
		t.Fatal(err)
	}
	if checkLinkAcl(group, pl, map[int]bool{1: true}, rules) {
		t.Fatal("escaped group routes")
	}
	// 现有 applyUserPolicy 使用替换而不是合并；这里以替换后的最终路由检查。
	group.RouteInclude = []dbdata.ValData{{Val: "10.30.0.0/24"}}
	group.RouteExclude = []dbdata.ValData{{Val: "10.30.0.20/32"}}
	if !checkLinkAcl(group, pl, map[int]bool{1: true}, rules) {
		t.Fatal("final user routes ignored")
	}
	if group.RouteInclude[0].Val != "10.30.0.0/24" || group.RouteExclude[0].Val != "10.30.0.20/32" {
		t.Fatal("ACL modified routes")
	}
	copy(packet[16:20], net.ParseIP("10.30.0.20").To4())
	if checkLinkAcl(group, pl, map[int]bool{1: true}, rules) {
		t.Fatal("user exclusion ignored")
	}
}
