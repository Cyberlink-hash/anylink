package dbdata

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/bjdgyc/anylink/base"
	"xorm.io/xorm"
)

const (
	DeviceObserved = "observed"
	DevicePending  = "pending"
	DeviceApproved = "approved"
	DeviceRejected = "rejected"
	DeviceRevoked  = "revoked"
)

// DevicePolicy is separate from User so ordinary edits/imports cannot overwrite it.
type DevicePolicy struct {
	UserId   int  `json:"user_id" xorm:"pk"`
	Enabled  bool `json:"enabled" xorm:"not null default 0"`
	Revision int  `json:"revision" xorm:"not null default 0"`
}

type UserDevice struct {
	Id              int        `json:"id" xorm:"pk autoincr"`
	UserId          int        `json:"user_id" xorm:"not null unique(device_identity) index"`
	KeyKind         string     `json:"key_kind" xorm:"varchar(24) not null unique(device_identity)"`
	KeyHash         string     `json:"key_hash" xorm:"varchar(64) not null unique(device_identity)"`
	State           string     `json:"state" xorm:"varchar(16) not null"`
	Label           string     `json:"label" xorm:"varchar(128)"`
	DeviceType      string     `json:"device_type" xorm:"varchar(128)"`
	PlatformVersion string     `json:"platform_version" xorm:"varchar(128)"`
	UserAgent       string     `json:"user_agent" xorm:"varchar(255)"`
	RemoteAddr      string     `json:"remote_addr" xorm:"varchar(64)"`
	Source          string     `json:"source" xorm:"varchar(16)"`
	FirstSeen       time.Time  `json:"first_seen"`
	LastSeen        time.Time  `json:"last_seen"`
	CreatedAt       time.Time  `json:"created_at" xorm:"created"`
	RequestedAt     *time.Time `json:"requested_at"`
	ReviewedAt      *time.Time `json:"reviewed_at"`
}

type DeviceAudit struct {
	Id        int       `json:"id" xorm:"pk autoincr"`
	UserId    int       `json:"user_id" xorm:"index"`
	DeviceId  int       `json:"device_id"`
	Action    string    `json:"action" xorm:"varchar(32)"`
	Actor     string    `json:"actor" xorm:"varchar(128)"`
	Detail    string    `json:"detail" xorm:"varchar(255)"`
	CreatedAt time.Time `json:"created_at" xorm:"created"`
}

type DeviceIdentity struct {
	UniqueIdHash       string
	UniqueIdGlobalHash string
	DeviceType         string
	PlatformVersion    string
	UserAgent          string
	RemoteAddr         string
}

func (d UserDevice) Matches(identity DeviceIdentity) bool {
	switch d.KeyKind {
	case "unique-id-global":
		return d.KeyHash != "" && d.KeyHash == identity.UniqueIdGlobalHash
	case "unique-id":
		return d.KeyHash != "" && d.KeyHash == identity.UniqueIdHash
	}
	return false
}

func (d DeviceIdentity) key() (string, string) {
	if d.UniqueIdGlobalHash != "" {
		return "unique-id-global", d.UniqueIdGlobalHash
	}
	return "unique-id", d.UniqueIdHash
}

var deviceMu sync.Mutex

type DeviceDeniedError struct{ Message string }

func (e *DeviceDeniedError) Error() string { return e.Message }
func deviceDenied(message string) error    { return &DeviceDeniedError{Message: message} }

func loadDevicePolicy(s *xorm.Session, userID int) (DevicePolicy, error) {
	p := DevicePolicy{UserId: userID}
	_, err := s.ID(userID).Get(&p)
	return p, err
}

// CheckDeviceAccess never registers devices: tunnel reconnects cannot create approvals.
func CheckDeviceAccess(username string, userID int, identity DeviceIdentity) error {
	var user User
	if err := One("Username", normalizeUsername(username), &user); err != nil {
		return err
	}
	if userID != 0 && user.Id != userID {
		return deviceDenied("账户已变更，请重新登录")
	}
	s := xdb.NewSession()
	defer s.Close()
	p, err := loadDevicePolicy(s, user.Id)
	if err != nil {
		return err
	}
	if !p.Enabled {
		return nil
	}
	var devices []UserDevice
	if err := s.Where("user_id = ? AND state = ?", user.Id, DeviceApproved).Find(&devices); err != nil {
		return err
	}
	for _, d := range devices {
		if d.Matches(identity) {
			return nil
		}
	}
	return deviceDenied("此设备未获授权，请联系管理员后重新登录")
}

// RegisterLoginDevice is called only after the complete password/OTP flow.
func RegisterLoginDevice(username string, identity DeviceIdentity) (int, error) {
	deviceMu.Lock()
	defer deviceMu.Unlock()
	var user User
	if err := One("Username", normalizeUsername(username), &user); err != nil {
		return 0, err
	}
	s := xdb.NewSession()
	defer s.Close()
	p, err := loadDevicePolicy(s, user.Id)
	if err != nil {
		return user.Id, err
	}
	kind, hash := identity.key()
	if hash == "" {
		if p.Enabled {
			return user.Id, deviceDenied("客户端未提供设备标识，请使用支持设备标识的客户端，或联系管理员")
		}
		return user.Id, nil
	}
	var devices []UserDevice
	if err := s.Where("user_id = ?", user.Id).Asc("id").Find(&devices); err != nil {
		return user.Id, err
	}
	// Prefer an already approved fixed binding when both identifiers are supplied.
	selected := -1
	for i, d := range devices {
		if d.Matches(identity) && (selected == -1 || d.State == DeviceApproved) {
			selected = i
		}
		if selected >= 0 && devices[selected].State == DeviceApproved {
			break
		}
	}
	now := time.Now()
	d := UserDevice{UserId: user.Id, KeyKind: kind, KeyHash: hash, State: DeviceObserved, Source: "login", FirstSeen: now}
	if selected >= 0 {
		d = devices[selected]
	}
	d.LastSeen = now
	d.DeviceType = substr(identity.DeviceType, 0, 128)
	d.PlatformVersion = substr(identity.PlatformVersion, 0, 128)
	d.UserAgent = substr(identity.UserAgent, 0, 255)
	d.RemoteAddr = deviceRemoteAddr(identity.RemoteAddr)
	if p.Enabled && d.State == DeviceObserved {
		d.State = DevicePending
		d.RequestedAt = &now
	}
	if d.Id == 0 {
		if len(devices) >= 50 {
			if p.Enabled {
				return user.Id, deviceDenied("设备记录已达上限，请联系管理员")
			}
			return user.Id, nil
		}
		if p.Enabled {
			recent := 0
			for _, old := range devices {
				if old.Source == "login" && old.CreatedAt.After(now.Add(-time.Hour)) {
					recent++
				}
			}
			if recent >= 5 {
				return user.Id, deviceDenied("新设备申请过于频繁，请稍后重试或联系管理员")
			}
		}
		_, err = s.Insert(&d)
	} else {
		_, err = s.ID(d.Id).Cols("last_seen", "device_type", "platform_version", "user_agent", "remote_addr", "state", "requested_at").Update(&d)
	}
	if err != nil {
		if !p.Enabled {
			base.Warn("记录候选设备失败", err)
			return user.Id, nil
		}
		return user.Id, err
	}
	if !p.Enabled || d.State == DeviceApproved {
		return user.Id, nil
	}
	if d.State == DeviceRejected || d.State == DeviceRevoked {
		return user.Id, deviceDenied(fmt.Sprintf("设备申请 #%d 已被拒绝或授权已撤销，请联系管理员", d.Id))
	}
	return user.Id, deviceDenied(fmt.Sprintf("此设备尚未授权，已提交申请 #%d，管理员批准后请重新连接", d.Id))
}

func deviceRemoteAddr(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err == nil {
		return host
	}
	return substr(addr, 0, 64)
}

type DeviceManagement struct {
	Policy  DevicePolicy  `json:"policy"`
	Devices []UserDevice  `json:"devices"`
	Audits  []DeviceAudit `json:"audits"`
}

func GetUserDevices(userID int) (*DeviceManagement, error) {
	deviceMu.Lock()
	defer deviceMu.Unlock()
	if err := One("Id", userID, &User{}); err != nil {
		return nil, err
	}
	s := xdb.NewSession()
	defer s.Close()
	p, err := loadDevicePolicy(s, userID)
	if err != nil {
		return nil, err
	}
	result := &DeviceManagement{Policy: p, Devices: []UserDevice{}, Audits: []DeviceAudit{}}
	if err := s.Where("user_id = ?", userID).Desc("last_seen", "id").Find(&result.Devices); err != nil {
		return nil, err
	}
	if err := s.Where("user_id = ?", userID).Desc("id").Limit(50).Find(&result.Audits); err != nil {
		return nil, err
	}
	return result, nil
}

type DeviceChange struct {
	UserId    int    `json:"user_id"`
	Revision  int    `json:"revision"`
	Action    string `json:"action"`
	DeviceId  int    `json:"device_id"`
	DeviceIds []int  `json:"device_ids"`
	ReplaceId int    `json:"replace_id"`
	Label     string `json:"label"`
}

func ChangeUserDevices(change DeviceChange, actor string) error {
	deviceMu.Lock()
	defer deviceMu.Unlock()
	var user User
	if err := One("Id", change.UserId, &user); err != nil {
		return err
	}
	s := xdb.NewSession()
	defer s.Close()
	if err := s.Begin(); err != nil {
		return err
	}
	defer s.Rollback()
	p, err := loadDevicePolicy(s, user.Id)
	if err != nil {
		return err
	}
	if p.Revision != change.Revision {
		return errors.New("设备设置已被修改，请刷新后重试")
	}
	audit := func(id int, action, detail string) error {
		_, err := s.Insert(&DeviceAudit{UserId: user.Id, DeviceId: id, Action: action, Actor: substr(actor, 0, 128), Detail: substr(detail, 0, 255)})
		return err
	}
	update := func(id int, state string) error {
		d := UserDevice{}
		found, err := s.Where("user_id = ? AND id = ?", user.Id, id).Get(&d)
		if err != nil {
			return err
		}
		if !found {
			return errors.New("设备不存在或不属于该用户")
		}
		if state == DeviceRevoked && d.State != DeviceApproved {
			return errors.New("只能撤销已批准的设备")
		}
		if state == DeviceRejected && d.State == DeviceApproved {
			return errors.New("已批准的设备请使用撤销操作")
		}
		now := time.Now()
		d.State = state
		d.ReviewedAt = &now
		if _, err := s.ID(id).Cols("state", "reviewed_at").Update(&d); err != nil {
			return err
		}
		return audit(id, state, "")
	}
	switch change.Action {
	case "enable":
		if len(change.DeviceIds) > 50 {
			return errors.New("选择设备过多")
		}
		for _, id := range change.DeviceIds {
			if err := update(id, DeviceApproved); err != nil {
				return err
			}
		}
		count, err := s.Where("user_id = ? AND state = ?", user.Id, DeviceApproved).Count(&UserDevice{})
		if err != nil {
			return err
		}
		if count == 0 {
			return errors.New("请先选择并确认至少一台设备")
		}
		p.Enabled = true
	case "disable":
		p.Enabled = false
	case "approve":
		if err := update(change.DeviceId, DeviceApproved); err != nil {
			return err
		}
	case "replace":
		if change.DeviceId == change.ReplaceId {
			return errors.New("新旧设备不能相同")
		}
		if err := update(change.ReplaceId, DeviceRevoked); err != nil {
			return err
		}
		if err := update(change.DeviceId, DeviceApproved); err != nil {
			return err
		}
	case "reject":
		if err := update(change.DeviceId, DeviceRejected); err != nil {
			return err
		}
	case "revoke":
		if err := update(change.DeviceId, DeviceRevoked); err != nil {
			return err
		}
	case "delete":
		d := UserDevice{}
		found, err := s.Where("user_id = ? AND id = ?", user.Id, change.DeviceId).Get(&d)
		if err != nil {
			return err
		}
		if !found {
			return errors.New("设备不存在或不属于该用户")
		}
		if d.State == DeviceApproved {
			return errors.New("请先撤销授权，再删除设备记录")
		}
		if _, err := s.ID(d.Id).Delete(&UserDevice{}); err != nil {
			return err
		}
		change.Label = d.KeyKind + ":" + d.KeyHash
	case "rename":
		if len([]rune(change.Label)) > 128 {
			return errors.New("设备备注最多128个字符")
		}
		affected, err := s.Where("user_id = ? AND id = ?", user.Id, change.DeviceId).Cols("label").Update(&UserDevice{Label: strings.TrimSpace(change.Label)})
		if err != nil {
			return err
		}
		if affected == 0 {
			return errors.New("设备不存在或备注未变化")
		}
	case "import":
		if err := importDeviceCandidates(s, user); err != nil {
			return err
		}
	default:
		return errors.New("无效的设备管理操作")
	}
	if err := audit(change.DeviceId, change.Action, change.Label); err != nil {
		return err
	}
	p.Revision++
	if p.Revision == 1 {
		_, err = s.Insert(&p)
	} else {
		_, err = s.ID(user.Id).AllCols().Update(&p)
	}
	if err != nil {
		return err
	}
	return s.Commit()
}

func importDeviceCandidates(s *xorm.Session, user User) error {
	var logs []UserActLog
	since := time.Now().AddDate(0, 0, -30)
	if user.CreatedAt.After(since) {
		since = user.CreatedAt
	}
	if err := s.Where("username = ? AND status = ? AND device_ids_collected = ? AND created_at >= ?", user.Username, UserConnected, true, since).Desc("id").Limit(500).Find(&logs); err != nil {
		return err
	}
	count, err := s.Where("user_id = ?", user.Id).Count(&UserDevice{})
	if err != nil {
		return err
	}
	for _, log := range logs {
		kind, hash := (DeviceIdentity{UniqueIdHash: log.UniqueIdHash, UniqueIdGlobalHash: log.UniqueIdGlobalHash}).key()
		if hash == "" {
			continue
		}
		exists, err := s.Where("user_id = ? AND key_kind = ? AND key_hash = ?", user.Id, kind, hash).Exist(&UserDevice{})
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if count >= 50 {
			break
		}
		d := UserDevice{UserId: user.Id, KeyKind: kind, KeyHash: hash, State: DeviceObserved, Source: "history", FirstSeen: log.CreatedAt, LastSeen: log.CreatedAt, DeviceType: log.DeviceType, PlatformVersion: log.PlatformVersion, RemoteAddr: log.RemoteAddr}
		if _, err := s.Insert(&d); err != nil {
			return err
		}
		count++
	}
	return nil
}

func FillDeviceSummaries(users []User) error {
	if len(users) == 0 {
		return nil
	}
	ids := make([]int, 0, len(users))
	for _, u := range users {
		ids = append(ids, u.Id)
	}
	var policies []DevicePolicy
	if err := xdb.In("user_id", ids).Find(&policies); err != nil {
		return err
	}
	var devices []UserDevice
	if err := xdb.In("user_id", ids).Find(&devices); err != nil {
		return err
	}
	for i := range users {
		for _, p := range policies {
			if p.UserId == users[i].Id {
				users[i].DeviceBindingEnabled = p.Enabled
			}
		}
		for _, d := range devices {
			if d.UserId != users[i].Id {
				continue
			}
			if d.State == DevicePending {
				users[i].PendingDevices++
			}
			if d.State == DeviceApproved {
				users[i].ApprovedDevices++
			}
		}
	}
	return nil
}
