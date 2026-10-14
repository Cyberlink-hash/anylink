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
