package handler

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bjdgyc/anylink/base"
	"github.com/bjdgyc/anylink/dbdata"
	"github.com/bjdgyc/anylink/sessdata"
	"github.com/xlzd/gotp"
)

func TestDeviceBindingRequiresCompleteAuthentication(t *testing.T) {
	base.Test()
	oldCfg := *base.Cfg
	defer func() { *base.Cfg = oldCfg }()
	base.Cfg.DbType = "sqlite3"
	base.Cfg.DbSource = filepath.Join(t.TempDir(), "devices.db")
	base.Cfg.AuthAloneOtp = true
	base.Cfg.AntiBruteForce = false
	dbdata.Start()
	defer func() {
		done := make(chan struct{})
		dbdata.UserActLogIns.Pool.JobQueue <- func() { close(done) }
		<-done
		_ = dbdata.Stop()
	}()
	group := dbdata.Group{Name: "device-test", Status: 1, Auth: map[string]interface{}{"type": "local"}}
	if err := dbdata.Add(&group); err != nil {
		t.Fatal(err)
	}
	user := dbdata.User{Username: "device-auth", PinCode: "test-password", OtpSecret: "JBSWY3DPEHPK3PXP", Status: 1, Groups: []string{group.Name}}
	if err := dbdata.Add(&user); err != nil {
		t.Fatal(err)
	}
	_, err := dbdata.RegisterLoginDevice(user.Username, dbdata.DeviceIdentity{UniqueIdGlobalHash: "existing-device"})
	if err != nil {
		t.Fatal(err)
	}
	m, err := dbdata.GetUserDevices(user.Id)
	if err != nil {
		t.Fatal(err)
	}
	if err = dbdata.ChangeUserDevices(dbdata.DeviceChange{UserId: user.Id, Action: "enable", DeviceIds: []int{m.Devices[0].Id}}, "admin"); err != nil {
		t.Fatal(err)
	}
	oldSession := sessdata.NewSession("")
	oldSession.UserId = user.Id
	oldSession.Username = user.Username
	defer sessdata.CloseSess(oldSession.Token)
	reconnect := httptest.NewRequest(http.MethodConnect, "/CSCOSSLC/tunnel", nil)
	reconnect.AddCookie(&http.Cookie{Name: "webvpn", Value: "test@" + oldSession.Token})
	reconnectResult := httptest.NewRecorder()
	LinkTunnel(reconnectResult, reconnect)
	if reconnectResult.Code != http.StatusForbidden {
		t.Fatal("old cookie without an approved identity bypassed binding")
	}
	checkCount := func(want int) {
		t.Helper()
		m, err := dbdata.GetUserDevices(user.Id)
		if err != nil || len(m.Devices) != want {
			t.Fatalf("devices=%+v err=%v", m, err)
		}
	}
	login := func(password string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`<config-auth type="auth-reply"><group-select>device-test</group-select><device-id unique-id-global="new-device"/><auth><username>device-auth</username><password>`+password+`</password></auth></config-auth>`))
		r.Header.Set("User-Agent", "AnyConnect")
		r.Header.Set("X-Aggregate-Auth", "1")
		r.Header.Set("X-Transcend-Version", "1")
		w := httptest.NewRecorder()
		LinkAuth(w, r)
		return w
	}
	login("wrong-password")
	checkCount(1)
	w := login("test-password")
	checkCount(1)
	var cookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == "auth-session-id" {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatalf("OTP challenge missing: %s", w.Body.String())
	}
	otp := func(value string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/otp-verification", strings.NewReader(`<config-auth><auth><secondary_password>`+value+`</secondary_password></auth></config-auth>`))
		r.AddCookie(cookie)
		result := httptest.NewRecorder()
		LinkAuth_otp(result, r)
		return result
	}
	otp("invalid")
	checkCount(1)
	w = otp(gotp.NewDefaultTOTP(user.OtpSecret).Now())
	checkCount(2)
	if !strings.Contains(w.Body.String(), "已提交申请") || strings.Contains(w.Body.String(), "<session-token>") {
		t.Fatalf("unexpected pending response: %s", w.Body.String())
	}
	m, err = dbdata.GetUserDevices(user.Id)
	if err != nil {
		t.Fatal(err)
	}
	pending := 0
	for _, d := range m.Devices {
		if d.State == dbdata.DevicePending {
			pending++
		}
	}
	if pending != 1 {
		t.Fatal("authenticated device not pending")
	}
}
