package handler

import (
	"encoding/binary"
	"net"
	"strings"

	"github.com/bjdgyc/anylink/dbdata"
	"github.com/bjdgyc/anylink/sessdata"
	"github.com/songgao/water/waterutil"
)

// 最终下发路由是访问权限上限，启用的用户策略覆盖组路由。
func withinGroupRoutes(group *dbdata.Group, ip net.IP) bool {
	for _, route := range group.RouteExclude {
		_, network, err := net.ParseCIDR(route.Val)
		if err != nil {
			return false
		}
		if network.Contains(ip) {
			return false
		}
	}
	allowed := len(group.RouteInclude) == 0
	for _, route := range group.RouteInclude {
		if strings.EqualFold(route.Val, dbdata.ALL) {
			allowed = true
			continue
		}
		_, network, err := net.ParseCIDR(route.Val)
		if err != nil {
			return false
		}
		if network.Contains(ip) {
			allowed = true
		}
	}
	return allowed
}

func matchACLRule(rule dbdata.GroupLinkAcl, ip net.IP, protocol waterutil.IPProtocol, port uint16) bool {
	if rule.Protocol != "" && rule.Protocol != dbdata.ALL && rule.IpProto != protocol {
		return false
	}
	if rule.IpNet == nil || !rule.IpNet.Contains(ip) {
		return false
	}
	return protocol == waterutil.ICMP || dbdata.ContainsInPorts(rule.Ports, port) || dbdata.ContainsInPorts(rule.Ports, 0)
}

func checkLinkAcl(group *dbdata.Group, pl *sessdata.Payload, policies map[int]bool, ruleSets ...map[int][]dbdata.GroupLinkAcl) bool {
	if pl.LType != sessdata.LTypeIPData || pl.PType != 0x00 {
		return true
	}
	if group == nil {
		return false
	}
	hasNetworkPolicies := len(group.NetworkPolicyIDs) > 0
	for _, rule := range group.LinkAcl {
		if rule.NetworkPolicyId != 0 {
			hasNetworkPolicies = true
			break
		}
	}
	if !hasNetworkPolicies && len(group.LinkAcl) == 0 {
		return true
	}
	if len(pl.Data) < 20 || pl.Data[0]>>4 != 4 {
		return false
	}
	headerLen := int(pl.Data[0]&15) * 4
	if headerLen < 20 || len(pl.Data) < headerLen {
		return false
	}
	ip := net.IP(pl.Data[16:20])
	if hasNetworkPolicies && !withinGroupRoutes(group, ip) {
		return false
	}
	protocol := waterutil.IPProtocol(pl.Data[9])
	var port uint16
	// 分片没有完整传输层信息，启用访问规则时拒绝，避免绕过端口规则。
	if binary.BigEndian.Uint16(pl.Data[6:8])&0x3fff != 0 && hasNetworkPolicies {
		return false
	}
	if protocol == waterutil.TCP || protocol == waterutil.UDP {
		if len(pl.Data) < headerLen+4 {
			return false
		}
		port = binary.BigEndian.Uint16(pl.Data[headerLen+2 : headerLen+4])
	}
	for _, dns := range group.ClientDns {
		if dns.Val == ip.String() && port == 53 {
			return true
		}
	}
	for _, id := range group.NetworkPolicyIDs {
		matched, exists := policies[id]
		if !exists || len(ruleSets) == 0 || len(ruleSets[0][id]) == 0 {
			return false
		}
		if !matched {
			continue
		}
		for _, rule := range ruleSets[0][id] {
			if matchACLRule(rule, ip, protocol, port) {
				return rule.Action == dbdata.Allow
			}
		}
	}
	if len(group.LinkAcl) == 0 {
		return true
	}
	for _, rule := range group.LinkAcl {
		if rule.NetworkPolicyId != 0 {
			matched, exists := policies[rule.NetworkPolicyId]
			if !exists {
				return false
			}
			if !matched {
				continue
			}
		}
		if matchACLRule(rule, ip, protocol, port) {
			return rule.Action == dbdata.Allow
		}
	}
	return false
}
