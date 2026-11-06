package dbdata

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/bjdgyc/anylink/pkg/utils"
	"xorm.io/xorm/schemas"
)

var policyMu sync.Mutex

// Sync2 creates the composite index first. Remove only the old username-only index.
func migratePolicyScope() error {
	tables, err := xdb.DBMetas()
	if err != nil {
		return err
	}
	for _, table := range tables {
		if table.Name != "policy" {
			continue
		}
		for _, index := range table.Indexes {
			if index.Type == schemas.UniqueType && len(index.Cols) == 1 && strings.EqualFold(index.Cols[0], "username") {
				if _, err = xdb.Exec(xdb.Dialect().DropIndexSQL(table.Name, index)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func preparePolicy(p *Policy) error {
	if p.Id < 0 || p.GroupId < 0 || (p.Status != 0 && p.Status != 1) {
		return errors.New("策略参数错误")
	}
	if p.Id > 0 {
		var old Policy
		if err := One("Id", p.Id, &old); err != nil {
			return err
		}
		if normalizeUsername(old.Username) != normalizeUsername(p.Username) {
			return errors.New("不能修改策略所属用户")
		}
		if old.ConfigVersion == 1 && p.ConfigVersion == 0 {
			return errors.New("新版策略不能退回兼容模式")
		}
		p.CreatedAt = old.CreatedAt
	}
	if p.ConfigVersion == 0 {
		if p.GroupId != 0 {
			return errors.New("旧版兼容策略仅适用于所有组，请先转换为新版策略")
		}
		return normalizePolicy(p)
	}
	if p.ConfigVersion != 1 {
		return errors.New("不支持的策略版本")
	}
	p.Username = normalizeUsername(p.Username)
	var user User
	if err := One("Username", p.Username, &user); err != nil {
		return errors.New("请选择已存在的用户")
	}
	for _, mode := range []string{p.LanMode, p.DnsMode, p.RouteMode, p.DomainMode} {
		if mode != "inherit" && mode != "custom" {
			return errors.New("每个模块请选择继承或自定义")
		}
	}
	if p.GroupId > 0 {
		var group Group
		if err := One("Id", p.GroupId, &group); err != nil {
			return errors.New("指定用户组不存在")
		}
		if !utils.InArrStr(user.Groups, group.Name) {
			return errors.New("用户不属于所选用户组")
		}
	}
	if p.LanMode == "inherit" {
		p.AllowLan = false
	}
	if p.DnsMode == "inherit" {
		p.ClientDns = nil
		p.SplitDns = nil
		p.NoGlobalDns = false
	}
	if p.RouteMode == "inherit" {
		p.RouteInclude = nil
		p.RouteExclude = nil
	} else if len(p.RouteInclude) == 0 {
		return errors.New("自定义路由请明确填写网段或 all，空白不代表全隧道")
	}
	if p.DomainMode == "inherit" {
		p.DsIncludeDomains = ""
		p.DsExcludeDomains = ""
	}
	if err := normalizePolicy(p); err != nil {
		return err
	}
	if p.RouteMode == "custom" {
		if len(p.RouteInclude) == 0 {
			return errors.New("自定义路由不能为空")
		}
		for _, route := range p.RouteInclude {
			if route.Val == ALL && len(p.RouteInclude) != 1 {
				return errors.New("all 不能与其他包含路由混用")
			}
		}
	}
	return nil
}

// An enabled group-specific policy replaces the all-groups policy, rather than stacking it.
func ResolveUserPolicy(username string, groupID int) (*Policy, error) {
	return resolvePolicy(username, groupID, nil)
}
func resolvePolicy(username string, groupID int, draft *Policy) (*Policy, error) {
	var policies []Policy
	if err := xdb.Where("LOWER(username) = ? AND status = ?", normalizeUsername(username), 1).Find(&policies); err != nil {
		return nil, err
	}
	if draft != nil {
		kept := policies[:0]
		for _, p := range policies {
			if p.Id != draft.Id {
				kept = append(kept, p)
			}
		}
		policies = kept
		if draft.Status == 1 {
			policies = append(policies, *draft)
		}
	}
	var selected *Policy
	for i := range policies {
		p := &policies[i]
		if p.GroupId != 0 && p.GroupId != groupID {
			continue
		}
		if selected != nil && selected.GroupId == p.GroupId {
			return nil, errors.New("存在重复生效策略，请先处理")
		}
		if selected == nil || p.GroupId == groupID {
			selected = p
		}
	}
	return selected, nil
}

// Shared by tunnel setup and preview. Inherited modules always come directly from the group.
func ApplyUserPolicy(g *Group, p *Policy) map[string]string {
	sources := map[string]string{"lan": "用户组", "dns": "用户组", "split_dns": "用户组", "route": "用户组", "domain": "用户组"}
	CompatGroupDns(g)
	if p == nil {
		return sources
	}
	copyPolicy := *p
	p = &copyPolicy
	CompatPolicyDns(p)
	source := "指定组用户策略"
	if p.GroupId == 0 {
		source = "所有组用户策略"
	}
	legacy := p.ConfigVersion == 0
	if legacy {
		source += "（兼容模式）"
	}
	if legacy || p.LanMode == "custom" {
		g.AllowLan = p.AllowLan
		sources["lan"] = source
	}
	if legacy || p.DnsMode == "custom" {
		g.ClientDns = p.ClientDns
		sources["dns"] = source
		if !legacy || p.NoGlobalDns || len(p.SplitDns) > 0 {
			g.NoGlobalDns = p.NoGlobalDns
			g.SplitDns = p.SplitDns
			sources["split_dns"] = source
		}
	}
	if legacy || p.RouteMode == "custom" {
		g.RouteInclude = p.RouteInclude
		g.RouteExclude = p.RouteExclude
		sources["route"] = source
	}
	if legacy || p.DomainMode == "custom" {
		g.DsIncludeDomains = p.DsIncludeDomains
		g.DsExcludeDomains = p.DsExcludeDomains
		sources["domain"] = source
	}
	return sources
}

func ValidateEffectivePolicy(g *Group) error {
	if (len(g.RouteInclude) == 0 || (len(g.RouteInclude) == 1 && g.RouteInclude[0].Val == ALL)) && len(g.ClientDns) == 0 {
		return errors.New("最终配置为全隧道，必须提供客户端 DNS")
	}
	if g.NoGlobalDns && len(g.SplitDns) == 0 {
		return errors.New("最终配置关闭全局 DNS，必须提供内网域名")
	}
	return nil
}
func validatePolicyGroups(p *Policy) error {
	var user User
	if err := One("Username", p.Username, &user); err != nil {
		return err
	}
	var groups []Group
	if err := xdb.Find(&groups); err != nil {
		return err
	}
	for _, group := range groups {
		if !utils.InArrStr(user.Groups, group.Name) || (p.GroupId != 0 && p.GroupId != group.Id) {
			continue
		}
		selected, err := resolvePolicy(p.Username, group.Id, p)
		if err != nil {
			return err
		}
		ApplyUserPolicy(&group, selected)
		if err := ValidateEffectivePolicy(&group); err != nil {
			return fmt.Errorf("用户组 %s: %w", group.Name, err)
		}
	}
	return nil
}

type PolicyPreview struct {
	Policy           *Policy           `json:"policy"`
	GroupName        string            `json:"group_name"`
	AllowLan         bool              `json:"allow_lan"`
	ClientDns        []ValData         `json:"client_dns"`
	NoGlobalDns      bool              `json:"no_global_dns"`
	SplitDns         []ValData         `json:"split_dns"`
	RouteInclude     []ValData         `json:"route_include"`
	RouteExclude     []ValData         `json:"route_exclude"`
	DsIncludeDomains string            `json:"ds_include_domains"`
	DsExcludeDomains string            `json:"ds_exclude_domains"`
	Sources          map[string]string `json:"sources"`
	NetworkPolicies  []GroupNameId     `json:"network_policies"`
	LinkAcl          []GroupLinkAcl    `json:"link_acl"`
	Warnings         []string          `json:"warnings"`
}

func PreviewPolicy(username string, groupID int, draft *Policy) (*PolicyPreview, error) {
	policyMu.Lock()
	defer policyMu.Unlock()
	username = normalizeUsername(username)
	var user User
	if err := One("Username", username, &user); err != nil {
		return nil, errors.New("请选择已存在的用户")
	}
	var group Group
	if err := One("Id", groupID, &group); err != nil {
		return nil, errors.New("请选择登录用户组")
	}
	if !utils.InArrStr(user.Groups, group.Name) {
		return nil, errors.New("用户不属于所选登录组")
	}
	if draft != nil {
		if normalizeUsername(draft.Username) != username {
			return nil, errors.New("预览用户与策略用户不一致")
		}
		if err := preparePolicy(draft); err != nil {
			return nil, err
		}
	}
	selected, err := resolvePolicy(username, groupID, draft)
	if err != nil {
		return nil, err
	}
	sources := ApplyUserPolicy(&group, selected)
	result := &PolicyPreview{Policy: selected, GroupName: group.Name, AllowLan: group.AllowLan, ClientDns: group.ClientDns, NoGlobalDns: group.NoGlobalDns, SplitDns: group.SplitDns, RouteInclude: group.RouteInclude, RouteExclude: group.RouteExclude, DsIncludeDomains: group.DsIncludeDomains, DsExcludeDomains: group.DsExcludeDomains, Sources: sources, LinkAcl: group.LinkAcl, NetworkPolicies: []GroupNameId{}, Warnings: []string{}}
	if user.Status != 1 || group.Status != 1 {
		result.Warnings = append(result.Warnings, "用户或用户组未启用，预览不代表可登录")
	}
	if err := ValidateEffectivePolicy(&group); err != nil {
		result.Warnings = append(result.Warnings, err.Error())
	}
	ids := append([]int{}, group.NetworkPolicyIDs...)
	for _, rule := range group.LinkAcl {
		if rule.NetworkPolicyId != 0 {
			ids = append(ids, rule.NetworkPolicyId)
		}
	}
	if len(ids) > 0 {
		var policies []NetworkPolicy
		if err := xdb.In("id", ids).Find(&policies); err != nil {
			return nil, err
		}
		for _, policy := range policies {
			result.NetworkPolicies = append(result.NetworkPolicies, GroupNameId{Id: policy.Id, Name: policy.Name})
		}
	}
	return result, nil
}
