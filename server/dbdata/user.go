package dbdata

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/bjdgyc/anylink/base"
	"github.com/bjdgyc/anylink/pkg/utils"
	"github.com/xlzd/gotp"
)

// type User struct {
// 	Id       int    `json:"id"  xorm:"pk autoincr not null"`
// 	Username string `json:"username" storm:"not null unique"`
// 	Nickname string `json:"nickname"`
// 	Email    string `json:"email"`
// 	// Password  string    `json:"password"`
// 	PinCode    string    `json:"pin_code"`
// 	OtpSecret  string    `json:"otp_secret"`
// 	DisableOtp bool      `json:"disable_otp"` // 禁用otp
// 	Groups     []string  `json:"groups"`
// 	Status     int8      `json:"status"` // 1正常
// 	SendEmail  bool      `json:"send_email"`
// 	CreatedAt  time.Time `json:"created_at"`
// 	UpdatedAt  time.Time `json:"updated_at"`
// }

func SetUser(v *User) error {
	var err error
	if v.Username == "" || len(v.Groups) == 0 {
		return errors.New("用户名或组错误")
	}
	if v.AuthType == "" {
		v.AuthType = "local"
	}

	planPass := v.PinCode
	// 自动生成密码
	if len(planPass) < 6 {
		planPass = utils.RandomRunes(8)
	}
	v.PinCode = planPass

	if v.OtpSecret == "" {
		v.OtpSecret = gotp.RandomSecret(32)
	}

	// 判断组是否有效
	ng := []string{}
	groups := GetGroupNames()
	for _, g := range v.Groups {
		if utils.InArrStr(groups, g) {
			ng = append(ng, g)
		}
	}
	if len(ng) == 0 {
		return errors.New("用户名或组错误")
	}
	v.Groups = ng

	v.UpdatedAt = time.Now()
	if v.Id > 0 {
		err = Set(v)
	} else {
		err = Add(v)
	}

	return err
}

// 验证用户登录信息
func CheckUser(name, pwd, group string, ext map[string]interface{}) error {
	base.Trace("CheckUser", name, pwd, group, ext)

	// 获取登入的group数据
	groupData := &Group{}
	err := One("Name", group, groupData)
	if err != nil || groupData.Status != 1 {
		return fmt.Errorf("%s - %s", name, "用户组错误")
	}
	// 初始化Auth
	if len(groupData.Auth) == 0 {
		groupData.Auth["type"] = "local"
	}
	authType := groupData.Auth["type"].(string)
	// 本地认证方式
	if authType == "local" {
		return checkLocalUser(name, pwd, group, ext)
	}
	// 其它认证方式, 支持自定义
	_, ok := authRegistry[authType]
	if !ok {
		return fmt.Errorf("%s %s", "未知的认证方式: ", authType)
	}
	auth := makeInstance(authType).(IUserAuth)
	err = auth.checkUser(name, pwd, groupData, ext)
	if err != nil {
		return err
	}
	return SyncExternalUser(name, group, authType)
}

func SyncExternalUser(name, group, authType string) error {
	if name == "" || group == "" || authType == "" || authType == "local" {
		return nil
	}
	v := &User{}
	err := One("Username", name, v)
	if err != nil {
		if !CheckErrNotFound(err) {
			return err
		}
		return Add(&User{
			Username:   name,
			AuthType:   authType,
			Groups:     []string{group},
			Status:     1,
			DisableOtp: true,
		})
	}
	if !utils.InArrStr(v.Groups, group) {
		v.Groups = append(v.Groups, group)
	}
	v.AuthType = authType
	v.UpdatedAt = time.Now()
	return Set(v)
}

func SyncLDAPUsersStatus() (int64, error) {
	var users []User
	err := FindWhere(&users, 0, 0, "auth_type=? AND status=?", "ldap", 1)
	if err != nil {
		return 0, err
	}
	var (
		affected int64
		errMsgs  []string
	)
	for _, user := range users {
		active, checked, err := checkLDAPUserActiveInAnyGroup(user.Username, user.Groups)
		if err != nil {
			errMsgs = append(errMsgs, err.Error())
			continue
		}
		if !checked || active {
			continue
		}
		n, err := xdb.ID(user.Id).Cols("status", "updated_at").Update(&User{
			Status:    0,
			UpdatedAt: time.Now(),
		})
		if err != nil {
			errMsgs = append(errMsgs, fmt.Sprintf("%s LDAP同步停用失败: %s", user.Username, err.Error()))
			continue
		}
		affected += n
	}
	if len(errMsgs) > 0 {
		return affected, errors.New(strings.Join(errMsgs, "; "))
	}
	return affected, nil
}

func checkLDAPUserActiveInAnyGroup(username string, groups []string) (bool, bool, error) {
	var (
		checked bool
		errMsgs []string
	)
	for _, groupName := range groups {
		groupData := &Group{}
		err := One("Name", groupName, groupData)
		if err != nil || groupData.Status != 1 {
			continue
		}
		if len(groupData.Auth) == 0 || groupData.Auth["type"] != "ldap" {
			continue
		}
		checked = true
		ok, err := (AuthLdap{}).checkUserActiveInGroup(username, groupData)
		if err != nil {
			errMsgs = append(errMsgs, fmt.Sprintf("%s/%s: %s", username, groupName, err.Error()))
			continue
		}
		if ok {
			return true, true, nil
		}
	}
	if len(errMsgs) > 0 && !checked {
		return false, checked, errors.New(strings.Join(errMsgs, "; "))
	}
	if len(errMsgs) > 0 {
		return false, checked, errors.New(strings.Join(errMsgs, "; "))
	}
	return false, checked, nil
}

// 验证本地用户登录信息
func checkLocalUser(name, pwd, group string, ext map[string]interface{}) error {
	// TODO 严重问题
	// return nil

	pl := len(pwd)
	if name == "" || pl < 6 {
		return fmt.Errorf("%s %s", name, "密码错误")
	}
	v := &User{}
	err := One("Username", name, v)
	if err != nil || v.Status != 1 {
		switch v.Status {
		case 0:
			return fmt.Errorf("%s %s", name, "用户不存在或用户已停用")
		case 2:
			return fmt.Errorf("%s %s", name, "用户已过期")
		}
	}
	// 判断用户组信息
	if !utils.InArrStr(v.Groups, group) {
		return fmt.Errorf("%s %s", name, "用户组错误")
	}

	pinCode := pwd
	if !base.Cfg.AuthAloneOtp {
		// 判断otp信息
		if !v.DisableOtp {
			pinCode = pwd[:pl-6]
			otp := pwd[pl-6:]
			if !CheckOtp(name, otp, v.OtpSecret) {
				return fmt.Errorf("%s %s", name, "动态码错误")
			}
		}
	}

	// 判断用户密码
	// 兼容明文密码
	if len(v.PinCode) != 60 {
		if pinCode != v.PinCode {
			return fmt.Errorf("%s %s", name, "密码错误")
		}
		return nil
	}
	// 密文密码
	if !utils.PasswordVerify(pinCode, v.PinCode) {
		return fmt.Errorf("%s %s", name, "密码错误")
	}

	return nil
}

// 用户过期时间到达后，更新用户状态，并返回一个状态为过期的用户切片
func CheckUserlimittime() (limitUser []interface{}) {
	if _, err := xdb.Where("limittime <= ?", time.Now()).And("status = ?", 1).Update(&User{Status: 2}); err != nil {
		return
	}
	user := make(map[int64]User)
	if err := xdb.Where("status != ?", 1).Find(user); err != nil {
		return
	}
	for _, v := range user {
		limitUser = append(limitUser, v.Username)
	}
	return
}

var (
	userOtpMux = sync.Mutex{}
	userOtp    = map[string]time.Time{}
)

func init() {
	go func() {
		expire := time.Second * 60

		for range time.Tick(time.Second * 10) {
			tnow := time.Now()
			userOtpMux.Lock()
			for k, v := range userOtp {
				if tnow.After(v.Add(expire)) {
					delete(userOtp, k)
				}
			}
			userOtpMux.Unlock()
		}
	}()
}

// 判断令牌信息
func CheckOtp(name, otp, secret string) bool {
	key := fmt.Sprintf("%s:%s", name, otp)

	userOtpMux.Lock()
	defer userOtpMux.Unlock()

	// 令牌只能使用一次
	if _, ok := userOtp[key]; ok {
		// 已经存在
		return false
	}
	userOtp[key] = time.Now()

	totp := gotp.NewDefaultTOTP(secret)
	unix := time.Now().Unix()
	verify := totp.Verify(otp, unix)

	return verify
}

// 插入数据库前加密密码
func (u *User) BeforeInsert() {
	if base.Cfg.EncryptionPassword {
		hashedPassword, err := utils.PasswordHash(u.PinCode)
		if err != nil {
			base.Error(err)
		}
		u.PinCode = hashedPassword
	}
}

// 更新数据库前加密密码
func (u *User) BeforeUpdate() {
	if len(u.PinCode) != 60 && base.Cfg.EncryptionPassword {
		hashedPassword, err := utils.PasswordHash(u.PinCode)
		if err != nil {
			base.Error(err)
		}
		u.PinCode = hashedPassword
	}
}
