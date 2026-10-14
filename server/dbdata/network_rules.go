package dbdata

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/songgao/water/waterutil"
)

func ValidateNetworkRules(rules []GroupLinkAcl) error {
	for i := range rules {
		rule := &rules[i]
		if rule.NetworkPolicyId != 0 {
			return fmt.Errorf("第%d条规则不能嵌套网络策略", i+1)
		}
		if rule.Action != Allow && rule.Action != Deny {
			return fmt.Errorf("第%d条规则动作错误", i+1)
		}
		rule.Val = strings.TrimSpace(rule.Val)
		if ip := net.ParseIP(rule.Val); ip != nil && ip.To4() != nil {
			rule.Val = ip.String() + "/32"
		}
		ip, network, err := net.ParseCIDR(rule.Val)
		if err != nil || ip.To4() == nil || !ip.Equal(network.IP) {
			return fmt.Errorf("第%d条规则目标必须是IPv4地址或标准网段", i+1)
		}
		rule.IpNet = network
		switch rule.Protocol {
		case TCP:
			rule.IpProto = waterutil.TCP
		case UDP:
			rule.IpProto = waterutil.UDP
		case ICMP:
			rule.IpProto = waterutil.ICMP
		case ALL:
			rule.IpProto = 0
		default:
			return fmt.Errorf("第%d条规则协议错误", i+1)
		}
		if rule.Protocol == ICMP {
			rule.Port = "0"
		}
		rule.Port = strings.TrimSpace(rule.Port)
		rule.Ports = map[uint16]int8{}
		for _, part := range strings.Split(rule.Port, ",") {
			limits := strings.Split(strings.TrimSpace(part), "-")
			if len(limits) > 2 {
				return fmt.Errorf("第%d条规则端口错误", i+1)
			}
			first, err := strconv.ParseUint(limits[0], 10, 16)
			if err != nil {
				return fmt.Errorf("第%d条规则端口错误", i+1)
			}
			last := first
			if len(limits) == 2 {
				last, err = strconv.ParseUint(limits[1], 10, 16)
				if err != nil || first > last || first == 0 {
					return fmt.Errorf("第%d条规则端口范围错误", i+1)
				}
			}
			for port := first; port <= last; port++ {
				rule.Ports[uint16(port)] = 1
			}
		}
	}
	return nil
}
