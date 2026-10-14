package admin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/mail"
	"sort"
	"sync"

	"github.com/bjdgyc/anylink/base"
	"github.com/bjdgyc/anylink/dbdata"
	"github.com/bjdgyc/anylink/sessdata"
	"github.com/go-ldap/ldap"
	simplemail "github.com/xhit/go-simple-mail/v2"
)

var ldapSyncMu sync.Mutex

type userOperationResult struct {
	Username string `json:"username"`
	Group    string `json:"group,omitempty"`
	Result   string `json:"result"`
	Detail   string `json:"detail,omitempty"`
}

func UserBatch(w http.ResponseWriter, r *http.Request) {
	var request struct {
		IDs    []int  `json:"ids"`
		Action string `json:"action"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536)).Decode(&request); err != nil || len(request.IDs) == 0 || len(request.IDs) > dbdata.PageSize {
		RespError(w, RespParamErr, "请选择当前页用户，最多10人")
		return
	}
	switch request.Action {
	case "enable", "disable", "otp_enable", "otp_disable", "delete":
	default:
		RespError(w, RespParamErr, "操作错误")
		return
	}
	results := []userOperationResult{}
	seen := map[int]bool{}
	for _, id := range request.IDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		item := userOperationResult{Username: fmt.Sprint(id), Result: "成功"}
		user, err := dbdata.UserBatchAction(id, request.Action)
		if err != nil {
			item.Result = "失败"
			item.Detail = err.Error()
		} else {
			item.Username = user.Username
			if request.Action == "disable" || request.Action == "delete" {
				sessdata.CloseUserSessions(user.Username)
			}
		}
		results = append(results, item)
	}
	RespSucess(w, results)
}

func LDAPSyncGroups(w http.ResponseWriter, r *http.Request) {
	var groups []dbdata.Group
	if err := dbdata.Find(&groups, 0, 0); err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	result := []dbdata.GroupNameId{}
	for _, group := range groups {
		if group.Status == 1 && group.Auth["type"] == "ldap" {
			result = append(result, dbdata.GroupNameId{Id: group.Id, Name: group.Name})
		}
	}
	RespSucess(w, result)
}

func UserSyncLDAP(w http.ResponseWriter, r *http.Request) {
	var request struct {
		GroupIDs []int `json:"group_ids"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536)).Decode(&request); err != nil || len(request.GroupIDs) == 0 {
		RespError(w, RespParamErr, "请选择LDAP用户组")
		return
	}
	if !ldapSyncMu.TryLock() {
		RespError(w, RespParamErr, "已有LDAP同步正在执行，请稍后查看用户列表")
		return
	}
	defer ldapSyncMu.Unlock()
	RespSucess(w, syncLDAPGroups(request.GroupIDs, dbdata.LDAPUsers, ldapWelcomeMail))
}

func syncLDAPGroups(groupIDs []int, load func(*dbdata.Group) ([]*ldap.Entry, dbdata.AuthLdap, error), notify func(*dbdata.User) error) map[string]interface{} {
	results := []userOperationResult{}
	created := map[string]*dbdata.User{}
	updated := map[string]bool{}
	seen := map[int]bool{}
	for _, id := range groupIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		var group dbdata.Group
		if id < 1 {
			results = append(results, userOperationResult{Group: fmt.Sprint(id), Result: "失败", Detail: "用户组ID错误"})
			continue
		}
		if err := dbdata.One("Id", id, &group); err != nil {
			results = append(results, userOperationResult{Group: fmt.Sprint(id), Result: "失败", Detail: "用户组不存在"})
			continue
		}
		entries, auth, err := load(&group)
		if err != nil {
			results = append(results, userOperationResult{Group: group.Name, Result: "失败", Detail: err.Error()})
			continue
		}
		for _, entry := range entries {
			name := dbdata.LDAPValue(entry, auth.SearchAttr)
			user, isNew, err := dbdata.SyncLDAPEntry(entry, auth.SearchAttr, group.Name)
			if err != nil {
				results = append(results, userOperationResult{Username: name, Group: group.Name, Result: "跳过/失败", Detail: err.Error()})
				continue
			}
			if isNew || created[user.Username] != nil {
				created[user.Username] = user
			} else {
				updated[user.Username] = true
			}
		}
	}
	names := []string{}
	for name := range created {
		names = append(names, name)
	}
	sort.Strings(names)
	sent, failed := 0, 0
	for _, name := range names {
		user := created[name]
		item := userOperationResult{Username: name, Result: "新增", Detail: "OTP已开启，邮件已发送"}
		if err := notify(user); err != nil {
			failed++
			item.Detail = "OTP已开启，邮件发送失败：" + err.Error() + "；请在用户编辑页补发邮件"
		} else {
			sent++
		}
		results = append(results, item)
	}
	return map[string]interface{}{"created": len(created), "updated": len(updated), "mail_sent": sent, "mail_failed": failed, "results": results}
}

func ldapWelcomeMail(user *dbdata.User) error {
	if _, err := mail.ParseAddress(user.Email); err != nil {
		return fmt.Errorf("邮箱为空或格式无效")
	}
	image, err := userOtpQr(user.Id, false)
	if err != nil {
		return err
	}
	setting := &dbdata.SettingOther{}
	if err := dbdata.SettingGet(setting); err != nil {
		return err
	}
	data := struct{ Username, Nickname, Address string }{user.Username, user.Nickname, setting.LinkAddr}
	const body = `<p>{{.Nickname}}，您好：</p><p>您的 VPN 账号已开通，并已启用 OTP 二次认证。</p><p>登录地址：{{.Address}}<br>用户名：{{.Username}}<br>密码：您现有的 LDAP 密码。</p><p>请用认证器扫描附件中的二维码，登录时输入认证器生成的动态码。请妥善保管二维码。</p>`
	var html bytes.Buffer
	if err := template.Must(template.New("ldap-welcome").Parse(body)).Execute(&html, data); err != nil {
		return err
	}
	return SendMail(base.Cfg.Issuer+" VPN账号与OTP通知", user.Email, html.String(), &simplemail.File{MimeType: "image/png", Name: "userOtpQr.png", Data: []byte(image), Inline: false})
}
