package dbdata

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

type NetworkPolicyGroup struct {
	Id     int    `json:"id"`
	Name   string `json:"name"`
	Order  int    `json:"order"`
	Legacy bool   `json:"legacy"`
}

func samePolicyOrder(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i, id := range a {
		if id != b[i] {
			return false
		}
	}
	return true
}
func sameGroupIds(a, b []int) bool {
	a = append([]int{}, a...)
	b = append([]int{}, b...)
	sort.Ints(a)
	sort.Ints(b)
	return samePolicyOrder(a, b)
}

// Group.NetworkPolicyIDs remains the only source of direct association and order.
func FillNetworkPolicyGroups(policies []NetworkPolicy) error {
	var groups []Group
	if err := xdb.Asc("id").Find(&groups); err != nil {
		return err
	}
	for i := range policies {
		ids := []int{}
		policies[i].Groups = []NetworkPolicyGroup{}
		for _, g := range groups {
			association := NetworkPolicyGroup{Id: g.Id, Name: g.Name}
			for index, id := range g.NetworkPolicyIDs {
				if id == policies[i].Id {
					association.Order = index + 1
					ids = append(ids, g.Id)
					break
				}
			}
			for _, rule := range g.LinkAcl {
				if rule.NetworkPolicyId == policies[i].Id {
					association.Legacy = true
					break
				}
			}
			if association.Order > 0 || association.Legacy {
				policies[i].Groups = append(policies[i].Groups, association)
			}
		}
		policies[i].GroupIds = &ids
	}
	return nil
}

// Called under networkPolicyMux, shared with group edits and policy deletion.
func saveNetworkPolicyGroups(p *NetworkPolicy) error {
	s := xdb.NewSession()
	defer s.Close()
	if err := s.Begin(); err != nil {
		return err
	}
	defer s.Rollback()
	var groups []Group
	selected := map[int]bool{}
	if p.GroupIds != nil {
		if err := s.Find(&groups); err != nil {
			return err
		}
		available := map[int]bool{}
		current := []int{}
		for _, g := range groups {
			available[g.Id] = true
			for _, id := range g.NetworkPolicyIDs {
				if id == p.Id {
					current = append(current, g.Id)
					break
				}
			}
		}
		if p.Id > 0 && p.OriginalGroupIds != nil && !sameGroupIds(current, *p.OriginalGroupIds) {
			return errors.New("此策略的用户组关联已变更，请刷新后重试")
		}
		for _, id := range *p.GroupIds {
			if id <= 0 || selected[id] {
				return errors.New("关联用户组无效或重复")
			}
			if !available[id] {
				return fmt.Errorf("关联用户组 #%d 不存在，请刷新后重试", id)
			}
			selected[id] = true
		}
	}
	if p.Id > 0 {
		if _, err := s.ID(p.Id).AllCols().Update(p); err != nil {
			return err
		}
	} else {
		if _, err := s.Insert(p); err != nil {
			return err
		}
	}
	for _, g := range groups {
		updated := make([]int, 0, len(g.NetworkPolicyIDs)+1)
		found := false
		for _, id := range g.NetworkPolicyIDs {
			if id == p.Id {
				found = true
				if !selected[g.Id] {
					continue
				}
			}
			updated = append(updated, id)
		}
		if selected[g.Id] && !found {
			updated = append(updated, p.Id)
		}
		if samePolicyOrder(updated, g.NetworkPolicyIDs) {
			continue
		}
		g.NetworkPolicyIDs = updated
		g.UpdatedAt = time.Now()
		if _, err := s.ID(g.Id).Cols(xdb.GetColumnMapper().Obj2Table("NetworkPolicyIDs"), "updated_at").Update(&g); err != nil {
			return err
		}
	}
	return s.Commit()
}
