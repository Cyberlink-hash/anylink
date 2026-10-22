package dbdata

import (
	"errors"
	"strings"

	"github.com/bjdgyc/anylink/pkg/utils"
)

type UserFilter struct{ Search, Group, AuthType, Status, OTP string }

func ListUsers(filter UserFilter, page, pageSize int) ([]User, int, error) {
	session := xdb.NewSession()
	defer session.Close()
	if query := strings.TrimSpace(filter.Search); query != "" {
		query = "%" + strings.ToLower(query) + "%"
		session.And("(LOWER(username) LIKE ? OR LOWER(nickname) LIKE ? OR LOWER(email) LIKE ?)", query, query, query)
	}
	if filter.AuthType != "" {
		if filter.AuthType == "local" {
			session.And("(auth_type = ? OR auth_type = ?)", "local", "")
		} else {
			session.And("auth_type = ?", filter.AuthType)
		}
	}
	switch filter.Status {
	case "":
	case "0", "1", "2":
		session.And("status = ?", filter.Status)
	default:
		return nil, 0, errors.New("状态筛选错误")
	}
	switch filter.OTP {
	case "":
	case "enabled":
		session.And("disable_otp = ?", false)
	case "disabled":
		session.And("disable_otp = ?", true)
	default:
		return nil, 0, errors.New("OTP筛选错误")
	}
	var users []User
	// Groups 是JSON字段，在Go中精确筛选，避免模糊匹配到相似组名。
	if filter.Group != "" {
		if err := session.Asc("id").Find(&users); err != nil {
			return nil, 0, err
		}
		matches := []User{}
		for _, user := range users {
			if utils.InArrStr(user.Groups, filter.Group) {
				matches = append(matches, user)
			}
		}
		count := len(matches)
		start := (page - 1) * pageSize
		if start >= count {
			return []User{}, count, nil
		}
		end := start + pageSize
		if end > count {
			end = count
		}
		return matches[start:end], count, nil
	}
	count, err := session.Asc("id").Limit(pageSize, (page-1)*pageSize).FindAndCount(&users)
	return users, int(count), err
}
