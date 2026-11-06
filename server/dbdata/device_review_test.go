package dbdata

import "testing"

func TestDeviceReviewListAndUserBindingFilter(t *testing.T) {
	setupUserManageDB(t)
	alice := deviceTestUser(t, "review-alice")
	bob := deviceTestUser(t, "review-bob")
	carol := deviceTestUser(t, "review-carol")
	alice.Groups = []string{"ops"}
	bob.Groups = []string{"ops"}
	if err := Set(&alice); err != nil {
		t.Fatal(err)
	}
	if err := Set(&bob); err != nil {
		t.Fatal(err)
	}
	deviceTestChange(t, DeviceChange{UserId: bob.Id, Action: "disable"})
	// Alice enabled, Bob explicitly disabled, Carol has no policy (new/synced account).
	if _, err := RegisterLoginDevice(alice.Username, deviceTestIdentity("trusted")); err != nil {
		t.Fatal(err)
	}
	trusted := deviceTestList(t, alice.Id).Devices[0].Id
	deviceTestChange(t, DeviceChange{UserId: alice.Id, Action: "enable", DeviceIds: []int{trusted}})
	if _, err := RegisterLoginDevice(alice.Username, deviceTestIdentity("new")); err == nil {
		t.Fatal("new device admitted")
	}
	rows, count, err := ListDeviceReviews("REVIEW-ALICE", DevicePending, 1, 1)
	if err != nil || count != 1 || len(rows) != 1 {
		t.Fatalf("pending list count=%d rows=%+v err=%v", count, rows, err)
	}
	row := rows[0]
	if row.Username != alice.Username || row.UserId != alice.Id || !row.BindingEnabled || row.Revision != 1 || row.RequestedAt == nil {
		t.Fatalf("bad joined metadata: %+v", row)
	}
	pending, err := CountPendingDevices()
	if err != nil || pending != 1 {
		t.Fatalf("badge=%d err=%v", pending, err)
	}
	_, count, err = ListDeviceReviews("review-alice", "", 2, 1)
	if err != nil || count != 2 {
		t.Fatalf("pagination count=%d err=%v", count, err)
	}
	users, n, err := ListUsers(UserFilter{DeviceBinding: "enabled"}, 1, 10)
	if err != nil || n != 1 || len(users) != 1 || users[0].Id != alice.Id {
		t.Fatalf("enabled filter: %+v n=%d err=%v", users, n, err)
	}
	users, n, err = ListUsers(UserFilter{DeviceBinding: "disabled"}, 1, 10)
	if err != nil || n != 2 || len(users) != 2 || users[0].Id != bob.Id || users[1].Id != carol.Id {
		t.Fatalf("disabled filter: %+v n=%d err=%v", users, n, err)
	}
	users, n, err = ListUsers(UserFilter{DeviceBinding: "disabled", Group: "ops"}, 1, 1)
	if err != nil || n != 1 || len(users) != 1 || users[0].Id != bob.Id {
		t.Fatalf("combined group filter: %+v n=%d err=%v", users, n, err)
	}
	deviceTestChange(t, DeviceChange{UserId: alice.Id, Action: "approve", DeviceId: row.Id})
	if err := ChangeUserDevices(DeviceChange{UserId: alice.Id, Revision: row.Revision, Action: "reject", DeviceId: row.Id}, "admin"); err == nil {
		t.Fatal("stale review overwrote approval")
	}
	pending, err = CountPendingDevices()
	if err != nil || pending != 0 {
		t.Fatalf("badge did not clear: %d %v", pending, err)
	}
	_, count, err = ListDeviceReviews("", DeviceApproved, 1, 10)
	if err != nil || count != 2 {
		t.Fatalf("approved filter: %d %v", count, err)
	}
	if _, _, err = ListDeviceReviews("", "invalid", 1, 10); err == nil {
		t.Fatal("invalid state allowed")
	}
	if _, _, err = ListUsers(UserFilter{DeviceBinding: "invalid"}, 1, 10); err == nil {
		t.Fatal("invalid binding filter allowed")
	}
	// Removed users must not leave visible orphan requests or inflate the badge.
	if err = Add(&UserDevice{UserId: 999999, KeyKind: "unique-id", KeyHash: "orphan", State: DevicePending}); err != nil {
		t.Fatal(err)
	}
	pending, err = CountPendingDevices()
	if err != nil || pending != 0 {
		t.Fatalf("orphan counted: %d %v", pending, err)
	}
}
