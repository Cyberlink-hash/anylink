package dbdata

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

var networkPolicyMux sync.Mutex

type NetworkPolicy struct {
	Rules     []GroupLinkAcl `json:"rules" xorm:"Text"`
	Id        int            `json:"id" xorm:"pk autoincr not null"`
	Name      string         `json:"name" xorm:"varchar(60) not null unique"`
	Usernames []string       `json:"usernames" xorm:"Text"`
	SourceIPs []string       `json:"source_ips" xorm:"Text"`
	Note      string         `json:"note" xorm:"varchar(255)"`
	CreatedAt time.Time      `json:"created_at" xorm:"DateTime created"`
	UpdatedAt time.Time      `json:"updated_at" xorm:"DateTime updated"`
}

func normalizePolicyValues(values []string) []string {
	result := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			result = append(result, value)
			seen[value] = true
		}
	}
	return result
}

func SetNetworkPolicy(p *NetworkPolicy) error {
	networkPolicyMux.Lock()
	defer networkPolicyMux.Unlock()
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" || len([]rune(p.Name)) > 60 || p.Id < 0 {
		return errors.New("网络策略名称或Id错误")
	}
	p.Usernames = normalizePolicyValues(p.Usernames)
	p.SourceIPs = normalizePolicyValues(p.SourceIPs)
	if len(p.Usernames) == 0 && len(p.SourceIPs) == 0 {
		return errors.New("用户名和来源IP至少填写一项")
	}
	for _, value := range p.SourceIPs {
		if net.ParseIP(value) == nil {
			if _, _, err := net.ParseCIDR(value); err != nil {
				return fmt.Errorf("来源IP或网段错误: %s", value)
			}
		}
	}
	if len(p.Rules) == 0 {
		return errors.New("至少配置一条访问规则")
	}
	if err := ValidateNetworkRules(p.Rules); err != nil {
		return err
	}
	if p.Id > 0 {
		var old NetworkPolicy
		if err := One("Id", p.Id, &old); err != nil {
			return err
		}
		p.CreatedAt = old.CreatedAt
	}
	p.UpdatedAt = time.Now()
	if p.Id > 0 {
		return Set(p)
	}
	return Add(p)
}

func DeleteNetworkPolicy(id int) error {
	networkPolicyMux.Lock()
	defer networkPolicyMux.Unlock()
	if id < 1 {
		return errors.New("Id错误")
	}
	var groups []Group
	if err := Find(&groups, 0, 0); err != nil {
		return err
	}
	for _, group := range groups {
		for _, policyID := range group.NetworkPolicyIDs {
			if policyID == id {
				return fmt.Errorf("网络策略被用户组 %s 引用，请先解除引用", group.Name)
			}
		}
		for _, rule := range group.LinkAcl {
			if rule.NetworkPolicyId == id {
				return fmt.Errorf("网络策略被用户组 %s 引用，请先解除引用", group.Name)
			}
		}
	}
	return Del(&NetworkPolicy{Id: id})
}

func (p *NetworkPolicy) Matches(username, remoteAddr string) bool {
	if len(p.Usernames) == 0 && len(p.SourceIPs) == 0 {
		return false
	}
	if len(p.Usernames) > 0 {
		found := false
		for _, name := range p.Usernames {
			if name == username {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if len(p.SourceIPs) == 0 {
		return true
	}
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, value := range p.SourceIPs {
		if allowed := net.ParseIP(value); allowed != nil {
			if allowed.Equal(ip) {
				return true
			}
		} else if _, network, err := net.ParseCIDR(value); err == nil && network.Contains(ip) {
			return true
		}
	}
	return false
}

// 每次建立隧道加载一次，在线连接保留原有匹配结果。
func MatchNetworkPolicies(group *Group, username, remoteAddr string) map[int]bool {
	matches, _ := LoadNetworkPolicies(group, username, remoteAddr)
	return matches
}

func LoadNetworkPolicies(group *Group, username, remoteAddr string) (map[int]bool, map[int][]GroupLinkAcl) {
	networkPolicyMux.Lock()
	defer networkPolicyMux.Unlock()
	matches := map[int]bool{}
	rules := map[int][]GroupLinkAcl{}
	ids := append([]int{}, group.NetworkPolicyIDs...)
	for _, rule := range group.LinkAcl {
		if rule.NetworkPolicyId != 0 {
			ids = append(ids, rule.NetworkPolicyId)
		}
	}
	for _, id := range ids {
		if _, exists := matches[id]; exists {
			continue
		}
		var policy NetworkPolicy
		if err := One("Id", id, &policy); err != nil {
			return nil, nil
		}
		if len(policy.Rules) > 0 {
			if err := ValidateNetworkRules(policy.Rules); err != nil {
				return nil, nil
			}
		}
		matches[id] = policy.Matches(username, remoteAddr)
		rules[id] = policy.Rules
	}
	return matches, rules
}
