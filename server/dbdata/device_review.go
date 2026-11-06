package dbdata

import (
	"errors"
	"strings"

	"xorm.io/xorm"
)

type DeviceReview struct {
	UserDevice     `xorm:"extends"`
	Username       string `json:"username"`
	BindingEnabled bool   `json:"binding_enabled"`
	Revision       int    `json:"revision"`
}

func deviceReviewQuery() *xorm.Session {
	return xdb.Table("user_device").Alias("d").Join("INNER", []string{"user", "u"}, "u.id = d.user_id").Join("LEFT", []string{"device_policy", "p"}, "p.user_id = d.user_id")
}

func CountPendingDevices() (int64, error) {
	s := deviceReviewQuery()
	defer s.Close()
	return s.Where("d.state = ?", DevicePending).Count(&UserDevice{})
}

func ListDeviceReviews(search, state string, page, pageSize int) ([]DeviceReview, int64, error) {
	switch state {
	case "", DevicePending, DeviceObserved, DeviceApproved, DeviceRejected, DeviceRevoked:
	default:
		return nil, 0, errors.New("设备状态筛选错误")
	}
	if page < 1 || pageSize < 1 {
		return nil, 0, errors.New("分页参数错误")
	}
	s := deviceReviewQuery()
	defer s.Close()
	if state != "" {
		s.And("d.state = ?", state)
	}
	if search = strings.TrimSpace(search); search != "" {
		s.And("LOWER(u.username) LIKE ?", "%"+strings.ToLower(search)+"%")
	}
	rows := []DeviceReview{}
	count, err := s.Select("d.*, u.username, COALESCE(p.enabled, false) AS binding_enabled, COALESCE(p.revision, 0) AS revision").Desc("d.created_at", "d.id").Limit(pageSize, (page-1)*pageSize).FindAndCount(&rows)
	return rows, count, err
}
