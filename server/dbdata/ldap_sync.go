package dbdata

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/bjdgyc/anylink/pkg/utils"
	"github.com/go-ldap/ldap"
	"github.com/xlzd/gotp"
)

var userWriteMu sync.Mutex
var ldapAttributeName = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9-]*$`)

// LDAPUsers 只读取符合当前用户组认证范围的用户，不修改目录。
func LDAPUsers(g *Group) ([]*ldap.Entry, AuthLdap, error) {
	var auth AuthLdap
	if g.Status != 1 || g.Auth["type"] != "ldap" {
		return nil, auth, errors.New("请选择已启用的LDAP用户组")
	}
	auth, err := auth.getLDAPAuth(g, "")
	if err != nil {
		return nil, auth, err
	}
	if err = auth.checkData(g.Auth); err != nil {
		return nil, auth, err
	}
	l, err := auth.bindLDAPAdmin("")
	if err != nil {
		return nil, auth, err
	}
	defer l.Close()
	l.SetTimeout(15 * time.Second)
	// 用户标识属性是配置中的LDAP属性名，不允许作为过滤表达式使用。
	if !ldapAttributeName.MatchString(auth.SearchAttr) {
		return nil, auth, errors.New("LDAP用户唯一ID属性名无效")
	}
	filter := fmt.Sprintf("(&(objectClass=%s)(%s=*))", ldap.EscapeFilter(auth.ObjectClass), auth.SearchAttr)
	sr, err := l.Search(ldap.NewSearchRequest(auth.BaseDn, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 15, false, filter, []string{auth.SearchAttr, "displayName", "mail", "shadowExpire"}, nil))
	if err != nil {
		return nil, auth, fmt.Errorf("LDAP用户查询失败: %w", err)
	}
	if auth.MemberOf == "" {
		return sr.Entries, auth, nil
	}
	gr, err := l.Search(ldap.NewSearchRequest(auth.MemberOf, ldap.ScopeBaseObject, ldap.NeverDerefAliases, 0, 15, false, "(objectClass=*)", []string{"objectClass", "uniqueMember", "member"}, nil))
	if err != nil {
		return nil, auth, fmt.Errorf("LDAP用户组查询失败: %w", err)
	}
	if len(gr.Entries) != 1 {
		return nil, auth, errors.New("LDAP受限用户组不存在或不唯一")
	}
	group := gr.Entries[0]
	if !hasLDAPObjectClass(group, "groupOfUniqueNames") && !hasLDAPObjectClass(group, "groupOfNames") {
		return nil, auth, errors.New("LDAP受限用户组类型不支持")
	}
	entries := []*ldap.Entry{}
	for _, entry := range sr.Entries {
		if hasLDAPAttributeValue(group, "uniqueMember", entry.DN) || hasLDAPAttributeValue(group, "member", entry.DN) {
			entries = append(entries, entry)
		}
	}
	return entries, auth, nil
}

func SyncLDAPEntry(entry *ldap.Entry, attr, group string) (*User, bool, error) {
	userWriteMu.Lock()
	defer userWriteMu.Unlock()
	username := strings.TrimSpace(LDAPValue(entry, attr))
	if username == "" {
		return nil, false, errors.New("LDAP用户名为空")
	}
	if err := parseEntries(&ldap.SearchResult{Entries: []*ldap.Entry{entry}}); err != nil {
		return nil, false, err
	}
	nickname := strings.TrimSpace(LDAPValue(entry, "displayName"))
	if nickname == "" {
		nickname = username
	}
	email := strings.TrimSpace(LDAPValue(entry, "mail"))
	var user User
	err := One("Username", username, &user)
	if err != nil && !CheckErrNotFound(err) {
		return nil, false, err
	}
	if CheckErrNotFound(err) {
		user = User{Username: username, AuthType: "ldap", Nickname: nickname, Email: email, Groups: []string{group}, Status: 1, OtpSecret: gotp.RandomSecret(32), DisableOtp: false}
		if err := Add(&user); err != nil {
			return nil, false, err
		}
		return &user, true, nil
	}
	if user.AuthType != "ldap" {
		return nil, false, errors.New("同名用户不是LDAP用户，已跳过")
	}
	user.Nickname = nickname
	if email != "" {
		user.Email = email
	}
	if !utils.InArrStr(user.Groups, group) {
		user.Groups = append(user.Groups, group)
	}
	user.UpdatedAt = time.Now()
	_, err = xdb.ID(user.Id).Cols("nickname", "email", "groups", "updated_at").Update(&user)
	return &user, false, err
}

func CheckUserEnabled(username string) error {
	var user User
	err := One("Username", username, &user)
	if err != nil {
		return err
	}
	if user.Status != 1 {
		return errors.New("用户已停用或过期")
	}
	if user.LimitTime != nil && !user.LimitTime.After(time.Now()) {
		return errors.New("用户已过期")
	}
	return nil
}

func UserBatchAction(id int, action string) (*User, error) {
	userWriteMu.Lock()
	defer userWriteMu.Unlock()
	var user User
	if id < 1 {
		return nil, errors.New("用户ID错误")
	}
	if err := One("Id", id, &user); err != nil {
		return nil, err
	}
	cols := []string{"updated_at"}
	switch action {
	case "enable":
		if user.LimitTime != nil && !user.LimitTime.After(time.Now()) {
			return nil, errors.New("用户已过期，请先修改过期时间")
		}
		user.Status = 1
		cols = append(cols, "status")
	case "disable":
		user.Status = 0
		cols = append(cols, "status")
	case "otp_enable":
		user.DisableOtp = false
		if user.OtpSecret == "" {
			user.OtpSecret = gotp.RandomSecret(32)
		}
		cols = append(cols, "disable_otp", "otp_secret")
	case "otp_disable":
		user.DisableOtp = true
		cols = append(cols, "disable_otp")
	case "delete":
		return &user, Del(&User{Id: id})
	default:
		return nil, errors.New("不支持的批量操作")
	}
	user.UpdatedAt = time.Now()
	_, err := xdb.ID(id).Cols(cols...).Update(&user)
	return &user, err
}

func LDAPValue(entry *ldap.Entry, name string) string {
	for _, attr := range entry.Attributes {
		if strings.EqualFold(attr.Name, name) && len(attr.Values) > 0 {
			return attr.Values[0]
		}
	}
	return ""
}
