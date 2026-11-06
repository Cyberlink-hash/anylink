package dbdata

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/bjdgyc/anylink/pkg/utils"
)

var networkPolicyMux sync.Mutex

const NetworkPolicyGroupMembers = "group_members"

type NetworkPolicy struct {
	UserMode          string               `json:"user_mode" xorm:"varchar(24) not null default ''"`
	ExcludedUsernames []string             `json:"excluded_usernames" xorm:"Text"`
	GroupIds          *[]int               `json:"group_ids,omitempty" xorm:"-"`
	OriginalGroupIds  *[]int               `json:"original_group_ids,omitempty" xorm:"-"`
	Groups            []NetworkPolicyGroup `json:"groups" xorm:"-"`
	Rules             []GroupLinkAcl       `json:"rules" xorm:"Text"`
	Id                int                  `json:"id" xorm:"pk autoincr not null"`
	Name              string               `json:"name" xorm:"varchar(60) not null unique"`
	Usernames         []string             `json:"usernames" xorm:"Text"`
	SourceIPs         []string             `json:"source_ips" xorm:"Text"`
	Note              string               `json:"note" xorm:"varchar(255)"`
	CreatedAt         time.Time            `json:"created_at" xorm:"DateTime created"`
	UpdatedAt         time.Time            `json:"updated_at" xorm:"DateTime updated"`
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
	switch p.UserMode {
	case "":
		if len(p.Usernames) == 0 && len(p.SourceIPs) == 0 {
			return errors.New("用户名和来源IP至少填写一项")
		}
		if len(p.ExcludedUsernames) > 0 {
			return errors.New("旧版用户选择不支持排除名单，请先转换为组成员模式")
		}
	case NetworkPolicyGroupMembers:
		if len(p.Usernames) > 0 {
			return errors.New("组成员模式仅保存排除名单，不接受固定用户名名单")
		}
		names := make([]string, 0, len(p.ExcludedUsernames))
		for _, name := range p.ExcludedUsernames {
			names = append(names, normalizeUsername(name))
		}
		p.ExcludedUsernames = normalizePolicyValues(names)
	default:
		return errors.New("用户适用模式错误")
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
		if old.UserMode == NetworkPolicyGroupMembers && p.UserMode != NetworkPolicyGroupMembers {
			return errors.New("组成员策略不能通过旧版表单保存，请刷新页面")
		}
		p.CreatedAt = old.CreatedAt
	}
	p.UpdatedAt = time.Now()
	return saveNetworkPolicyGroups(p)
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

// Group-member mode requires membership verified by the session loader.
func (p *NetworkPolicy) Matches(username, remoteAddr string, groupMember ...bool) bool {
	if p.UserMode == NetworkPolicyGroupMembers {
		if len(groupMember) == 0 || !groupMember[0] {
			return false
		}
		if utils.InArrStr(p.ExcludedUsernames, normalizeUsername(username)) {
			return false
		}
	} else if p.UserMode != "" {
		return false
	}
	if p.UserMode == "" && len(p.Usernames) == 0 && len(p.SourceIPs) == 0 {
		return false
	}
	if p.UserMode == "" && len(p.Usernames) > 0 {
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
		member := false
		if policy.UserMode == NetworkPolicyGroupMembers {
			var user User
			if err := One("Username", normalizeUsername(username), &user); err != nil {
				return nil, nil
			}
			member = utils.InArrStr(user.Groups, group.Name)
		}
		matches[id] = policy.Matches(username, remoteAddr, member)
		rules[id] = policy.Rules
	}
	return matches, rules
}
