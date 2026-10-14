package handler

import (
	"github.com/bjdgyc/anylink/dbdata"
	"github.com/bjdgyc/anylink/sessdata"
	"github.com/songgao/water/waterutil"
)

// Acl规则校验
func checkLinkAcl(group *dbdata.Group, pl *sessdata.Payload, policies map[int]bool) bool {
	if pl.LType == sessdata.LTypeIPData && pl.PType == 0x00 && len(group.LinkAcl) > 0 {
	} else {
		return true
	}

	ipDst := waterutil.IPv4Destination(pl.Data)
	ipPort := waterutil.IPv4DestinationPort(pl.Data)
	ipProto := waterutil.IPv4Protocol(pl.Data)
	// fmt.Println("sent:", ip_dst, ip_port)

	// 优先放行dns端口
	for _, v := range group.ClientDns {
		if v.Val == ipDst.String() && ipPort == 53 {
			return true
		}
	}

	for _, v := range group.LinkAcl {
		if v.NetworkPolicyId != 0 {
			matched, exists := policies[v.NetworkPolicyId]
			if !exists {
				return false
			}
			if !matched {
				continue
			}
		}
		// 放行允许ip的ping
		// if v.Ports == nil || len(v.Ports) == 0 {
		// 	//单端口历史数据兼容
		// 	port := uint16(v.Port.(float64))
		// 	if port == ipPort || port == 0 || ipProto == waterutil.ICMP {
		// 		if v.Action == dbdata.Allow {
		// 			return true
		// 		} else {
		// 			return false
		// 		}
		// 	}
		// } else {

		// 先判断协议
		// 兼容旧数据 v.Protocol == ""
		if v.Protocol == "" || v.Protocol == dbdata.ALL || v.IpProto == ipProto {
			// 循环判断ip和端口
			if v.IpNet.Contains(ipDst) {
				// icmp 不判断端口
				if ipProto == waterutil.ICMP {
					if v.Action == dbdata.Allow {
						return true
					} else {
						return false
					}
				}

				if dbdata.ContainsInPorts(v.Ports, ipPort) || dbdata.ContainsInPorts(v.Ports, 0) {
					if v.Action == dbdata.Allow {
						// log.Println(dbdata.Allow, v.Ports)
						return true
					} else {
						// log.Println(dbdata.Deny, v.Ports)
						return false
					}
				}
			}
		}
	}

	return false
}
