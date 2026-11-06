package dbdata

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func deviceTestUser(t *testing.T, name string) User {
	t.Helper()
	u := User{Username: name, Status: 1, DisableOtp: true}
	if err := Add(&u); err != nil {
		t.Fatal(err)
	}
	return u
}
func deviceTestIdentity(name string) DeviceIdentity {
	return DeviceIdentity{UniqueIdHash: deviceIdHash("id:" + name), UniqueIdGlobalHash: deviceIdHash("global:" + name), DeviceType: "Mac", RemoteAddr: "192.0.2.1:443"}
}
func deviceTestList(t *testing.T, id int) *DeviceManagement {
	t.Helper()
	m, err := GetUserDevices(id)
	if err != nil {
		t.Fatal(err)
	}
	return m
}
func deviceTestChange(t *testing.T, c DeviceChange) {
	t.Helper()
	c.Revision = deviceTestList(t, c.UserId).Policy.Revision
	if err := ChangeUserDevices(c, "admin"); err != nil {
		t.Fatal(err)
	}
}

func TestDeviceBindingLifecycleAndFixedField(t *testing.T) {
	setupUserManageDB(t)
	u := deviceTestUser(t, "alice")
	identity := deviceTestIdentity("computer")
	if _, err := RegisterLoginDevice(u.Username, DeviceIdentity{}); err != nil {
		t.Fatal("disabled missing ID rejected", err)
	}
	if _, err := RegisterLoginDevice(u.Username, identity); err != nil {
		t.Fatal(err)
	}
	m := deviceTestList(t, u.Id)
	if m.Policy.Enabled || len(m.Devices) != 1 || m.Devices[0].State != DeviceObserved || m.Devices[0].KeyKind != "unique-id-global" {
		t.Fatalf("unexpected candidate: %+v", m)
	}
	id := m.Devices[0].Id
	deviceTestChange(t, DeviceChange{UserId: u.Id, Action: "enable", DeviceIds: []int{id}})
	if err := CheckDeviceAccess(u.Username, u.Id, identity); err != nil {
		t.Fatal(err)
	}
	moved := identity
	moved.RemoteAddr = "203.0.113.2:1234"
	moved.PlatformVersion = "new"
	moved.UniqueIdHash = deviceIdHash("changed-secondary")
	if _, err := RegisterLoginDevice(u.Username, moved); err != nil {
		t.Fatal("network/version/secondary change broke binding", err)
	}
	if err := CheckDeviceAccess(u.Username, u.Id, DeviceIdentity{UniqueIdHash: identity.UniqueIdHash}); err == nil {
		t.Fatal("global binding downgraded to unique-id")
	}
	if _, err := RegisterLoginDevice(u.Username, DeviceIdentity{}); err == nil {
		t.Fatal("enabled missing ID admitted")
	}
	newIdentity := deviceTestIdentity("phone")
	for i := 0; i < 3; i++ {
		if _, err := RegisterLoginDevice(u.Username, newIdentity); err == nil || !strings.Contains(err.Error(), "已提交申请") {
			t.Fatalf("pending admission: %v", err)
		}
	}
	m = deviceTestList(t, u.Id)
	if len(m.Devices) != 2 {
		t.Fatal("duplicate requests created")
	}
	var pending int
	for _, d := range m.Devices {
		if d.State == DevicePending {
			pending = d.Id
		}
	}
	deviceTestChange(t, DeviceChange{UserId: u.Id, Action: "replace", DeviceId: pending, ReplaceId: id})
	if err := CheckDeviceAccess(u.Username, u.Id, identity); err == nil {
		t.Fatal("replaced device still admitted")
	}
	if err := CheckDeviceAccess(u.Username, u.Id, newIdentity); err != nil {
		t.Fatal(err)
	}
	if _, err := RegisterLoginDevice(u.Username, identity); err == nil || !strings.Contains(err.Error(), "撤销") {
		t.Fatal("revoked device restored", err)
	}
	deviceTestChange(t, DeviceChange{UserId: u.Id, Action: "disable"})
	if _, err := RegisterLoginDevice(u.Username, DeviceIdentity{}); err != nil {
		t.Fatal(err)
	}
	if len(deviceTestList(t, u.Id).Audits) == 0 {
		t.Fatal("no audit trail")
	}
}

func TestDeviceBindingUniqueOnlyDoesNotChangeKey(t *testing.T) {
	setupUserManageDB(t)
	u := deviceTestUser(t, "bob")
	identity := deviceTestIdentity("phone")
	identity.UniqueIdGlobalHash = ""
	if _, err := RegisterLoginDevice(u.Username, identity); err != nil {
		t.Fatal(err)
	}
	id := deviceTestList(t, u.Id).Devices[0].Id
	deviceTestChange(t, DeviceChange{UserId: u.Id, Action: "enable", DeviceIds: []int{id}})
	identity.UniqueIdGlobalHash = deviceIdHash("new-global")
	if _, err := RegisterLoginDevice(u.Username, identity); err != nil {
		t.Fatal(err)
	}
	m := deviceTestList(t, u.Id)
	if len(m.Devices) != 1 || m.Devices[0].KeyKind != "unique-id" {
		t.Fatal("binding changed")
	}
}

func TestDeviceBindingTransactionsOwnershipAndOrdinaryUserWrites(t *testing.T) {
	setupUserManageDB(t)
	alice := deviceTestUser(t, "alice")
	bob := deviceTestUser(t, "bob")
	RegisterLoginDevice(alice.Username, deviceTestIdentity("a"))
	RegisterLoginDevice(bob.Username, deviceTestIdentity("b"))
	aID := deviceTestList(t, alice.Id).Devices[0].Id
	bID := deviceTestList(t, bob.Id).Devices[0].Id
	if err := ChangeUserDevices(DeviceChange{UserId: alice.Id, Action: "enable", DeviceIds: []int{aID, bID}}, "admin"); err == nil {
		t.Fatal("cross-user device accepted")
	}
	m := deviceTestList(t, alice.Id)
	if m.Policy.Enabled || m.Devices[0].State != DeviceObserved || len(m.Audits) != 0 {
		t.Fatal("partial enable committed")
	}
	deviceTestChange(t, DeviceChange{UserId: alice.Id, Action: "enable", DeviceIds: []int{aID}})
	if err := ChangeUserDevices(DeviceChange{UserId: alice.Id, Action: "disable", Revision: 0}, "admin"); err == nil {
		t.Fatal("stale write accepted")
	}
	rev := deviceTestList(t, alice.Id).Policy.Revision
	if err := ChangeUserDevices(DeviceChange{UserId: alice.Id, Action: "replace", DeviceId: bID, ReplaceId: aID, Revision: rev}, "admin"); err == nil {
		t.Fatal("cross-user replacement accepted")
	}
	if err := CheckDeviceAccess(alice.Username, alice.Id, deviceTestIdentity("a")); err != nil {
		t.Fatal("failed replacement revoked original", err)
	}
	alice.Nickname = "Edited"
	if err := Set(&alice); err != nil {
		t.Fatal(err)
	}
	if !deviceTestList(t, alice.Id).Policy.Enabled {
		t.Fatal("ordinary user update disabled binding")
	}
	if err := CheckDeviceAccess(alice.Username, bob.Id, deviceTestIdentity("a")); err == nil {
		t.Fatal("stale user ID admitted")
	}
}

func TestDeviceBindingHistoryOnlyImportsSuccessfulConnections(t *testing.T) {
	setupUserManageDB(t)
	u := deviceTestUser(t, "history")
	// Give the account a creation time before the test logs (SQLite timestamp precision).
	_, err := xdb.Exec("UPDATE user SET created_at = ? WHERE id = ?", time.Now().Add(-time.Hour), u.Id)
	if err != nil {
		t.Fatal(err)
	}
	for i, status := range []uint8{UserAuthFail, UserAuthSuccess, UserConnected, UserDeviceDenied} {
		log := UserActLog{Username: u.Username, Status: status}
		log.SetDeviceIds(fmt.Sprint(i), fmt.Sprint(i))
		if err := Add(&log); err != nil {
			t.Fatal(err)
		}
	}
	deviceTestChange(t, DeviceChange{UserId: u.Id, Action: "import"})
	m := deviceTestList(t, u.Id)
	if len(m.Devices) != 1 || m.Devices[0].KeyHash != deviceIdHash("2") || m.Devices[0].State != DeviceObserved {
		t.Fatalf("unsafe history import: %+v", m.Devices)
	}
	deviceTestChange(t, DeviceChange{UserId: u.Id, Action: "reject", DeviceId: m.Devices[0].Id})
	deviceTestChange(t, DeviceChange{UserId: u.Id, Action: "import"})
	if deviceTestList(t, u.Id).Devices[0].State != DeviceRejected {
		t.Fatal("import reset rejection")
	}
}

func TestDeviceBindingPendingRateLimitAndRejection(t *testing.T) {
	setupUserManageDB(t)
	u := deviceTestUser(t, "limits")
	RegisterLoginDevice(u.Username, deviceTestIdentity("first"))
	id := deviceTestList(t, u.Id).Devices[0].Id
	deviceTestChange(t, DeviceChange{UserId: u.Id, Action: "enable", DeviceIds: []int{id}})
	for i := 0; i < 4; i++ {
		RegisterLoginDevice(u.Username, deviceTestIdentity(fmt.Sprint(i)))
	}
	if _, err := RegisterLoginDevice(u.Username, deviceTestIdentity("overflow")); err == nil || !strings.Contains(err.Error(), "频繁") {
		t.Fatal("request rate limit not enforced", err)
	}
	m := deviceTestList(t, u.Id)
	var pending UserDevice
	for _, d := range m.Devices {
		if d.State == DevicePending {
			pending = d
			break
		}
	}
	deviceTestChange(t, DeviceChange{UserId: u.Id, Action: "reject", DeviceId: pending.Id})
	identity := DeviceIdentity{UniqueIdGlobalHash: pending.KeyHash}
	if _, err := RegisterLoginDevice(u.Username, identity); err == nil || !strings.Contains(err.Error(), "拒绝") {
		t.Fatal("rejected device reapplied", err)
	}
}
