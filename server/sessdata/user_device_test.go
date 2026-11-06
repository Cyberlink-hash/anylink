package sessdata

import (
	"github.com/bjdgyc/anylink/dbdata"
	"testing"
)

func TestDeviceBindingEvictsActiveAndSleepingSessions(t *testing.T) {
	preData(t.TempDir())
	defer func() {
		for token := range sessions {
			CloseSess(token)
		}
		done := make(chan struct{})
		dbdata.UserActLogIns.Pool.JobQueue <- func() { close(done) }
		<-done
		_ = dbdata.Stop()
	}()
	user := dbdata.User{Username: "device-session-test", Status: 1}
	if err := dbdata.Add(&user); err != nil {
		t.Fatal(err)
	}
	identity := dbdata.DeviceIdentity{UniqueIdGlobalHash: "approved-device"}
	if _, err := dbdata.RegisterLoginDevice(user.Username, identity); err != nil {
		t.Fatal(err)
	}
	management, err := dbdata.GetUserDevices(user.Id)
	if err != nil {
		t.Fatal(err)
	}
	deviceID := management.Devices[0].Id
	makeSession := func(hash string, active bool) *Session {
		s := NewSession("")
		s.UserId = user.Id
		s.Username = user.Username
		s.Group = "group1"
		s.UniqueIdGlobalHash = hash
		if active && s.NewConn() == nil {
			t.Fatal("connection creation failed")
		}
		return s
	}
	approved := makeSession(identity.UniqueIdGlobalHash, true)
	denied := makeSession("unknown-device", true)
	sleeping := makeSession("", false)
	deniedConn := denied.CSess
	DeviceAccessMu.Lock()
	err = dbdata.ChangeUserDevices(dbdata.DeviceChange{UserId: user.Id, Action: "enable", DeviceIds: []int{deviceID}}, "admin")
	if err != nil {
		DeviceAccessMu.Unlock()
		t.Fatal(err)
	}
	count, err := ReconcileUserDevices(user.Id, user.Username, nil)
	DeviceAccessMu.Unlock()
	if err != nil || count != 2 {
		t.Fatalf("eviction count=%d err=%v", count, err)
	}
	if Token2Sess(approved.Token) != approved || !approved.CSess.IsCurrent() {
		t.Fatal("approved connection interrupted")
	}
	if Token2Sess(denied.Token) != nil || Token2Sess(sleeping.Token) != nil || deniedConn.IsCurrent() {
		t.Fatal("revoked session still usable")
	}
	select {
	case <-deniedConn.CloseChan:
	default:
		t.Fatal("active transport not closed")
	}
	if sleeping.CheckDeviceAccess() == nil {
		t.Fatal("missing identity accepted after enable")
	}
	DeviceAccessMu.Lock()
	err = dbdata.ChangeUserDevices(dbdata.DeviceChange{UserId: user.Id, Revision: 1, Action: "revoke", DeviceId: deviceID}, "admin")
	if err != nil {
		DeviceAccessMu.Unlock()
		t.Fatal(err)
	}
	count, err = ReconcileUserDevices(user.Id, user.Username, []int{deviceID})
	DeviceAccessMu.Unlock()
	if err != nil || count != 1 || Token2Sess(approved.Token) != nil {
		t.Fatalf("revoke failed count=%d err=%v", count, err)
	}
	if approved.CheckDeviceAccess() == nil {
		t.Fatal("old identity allowed after revoke")
	}
}
