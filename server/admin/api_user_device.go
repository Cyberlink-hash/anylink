package admin

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/bjdgyc/anylink/base"
	"github.com/bjdgyc/anylink/dbdata"
	"github.com/bjdgyc/anylink/sessdata"
)

func UserDevices(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.URL.Query().Get("user_id"))
	if err != nil || id < 1 {
		RespError(w, RespParamErr, "用户ID错误")
		return
	}
	data, err := dbdata.GetUserDevices(id)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	RespSucess(w, data)
}

func UserDevicesChange(w http.ResponseWriter, r *http.Request) {
	var change dbdata.DeviceChange
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(&change); err != nil || change.UserId < 1 {
		RespError(w, RespParamErr, "设备管理参数错误")
		return
	}
	sessdata.DeviceAccessMu.Lock()
	defer sessdata.DeviceAccessMu.Unlock()
	var user dbdata.User
	if err := dbdata.One("Id", change.UserId, &user); err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	if err := dbdata.ChangeUserDevices(change, base.Cfg.AdminUser); err != nil {
		RespError(w, RespParamErr, err)
		return
	}
	var blocked []int
	switch change.Action {
	case "revoke", "reject":
		blocked = []int{change.DeviceId}
	case "replace":
		blocked = []int{change.ReplaceId}
	}
	closed, err := sessdata.ReconcileUserDevices(user.Id, user.Username, blocked)
	if err != nil {
		// Fail closed if the committed policy cannot be read back for selective eviction.
		sessdata.CloseUserSessions(user.Username)
		RespError(w, RespInternalErr, "设置已保存，但会话检查失败，已关闭该用户连接，请刷新确认")
		return
	}
	RespSucess(w, map[string]int{"closed_sessions": closed})
}

func DeviceReviews(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	rows, count, err := dbdata.ListDeviceReviews(r.URL.Query().Get("search"), r.URL.Query().Get("state"), page, dbdata.PageSize)
	if err != nil {
		RespError(w, RespParamErr, err)
		return
	}
	RespSucess(w, map[string]interface{}{"datas": rows, "count": count, "page_size": dbdata.PageSize})
}

func DevicePendingCount(w http.ResponseWriter, r *http.Request) {
	count, err := dbdata.CountPendingDevices()
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	RespSucess(w, map[string]int64{"count": count})
}
