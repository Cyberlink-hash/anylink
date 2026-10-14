package dbdata

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/bjdgyc/anylink/base"
	"github.com/go-ldap/ldap"
)

func setupUserManageDB(t *testing.T) {
	t.Helper()
	oldType, oldSource := base.Cfg.DbType, base.Cfg.DbSource
	base.Cfg.DbType, base.Cfg.DbSource = "sqlite3", filepath.Join(t.TempDir(), "users.db")
	initDb()
	t.Cleanup(func() { xdb.Close(); base.Cfg.DbType, base.Cfg.DbSource = oldType, oldSource })
}

func TestLDAPSyncPreservesLocalSettings(t *testing.T) {
	setupUserManageDB(t)
	entry := ldap.NewEntry("uid=aaron,ou=people,dc=example,dc=com", map[string][]string{"uid": {"aaron"}, "displayName": {"Aaron"}, "mail": {"aaron@example.com"}})
	user, created, err := SyncLDAPEntry(entry, "uid", "product")
	if err != nil || !created {
		t.Fatalf("create: %v %v", created, err)
	}
	if user.DisableOtp || user.OtpSecret == "" || user.Nickname != "Aaron" || user.Email != "aaron@example.com" {
		t.Fatalf("new user defaults incorrect")
	}
	secret := user.OtpSecret
	if _, err := UserBatchAction(user.Id, "disable"); err != nil {
		t.Fatal(err)
	}
	if _, err := UserBatchAction(user.Id, "otp_disable"); err != nil {
		t.Fatal(err)
	}
	entry = ldap.NewEntry(entry.DN, map[string][]string{"UID": {"aaron"}, "DISPLAYNAME": {"New Name"}, "mail": {""}})
	user, created, err = SyncLDAPEntry(entry, "uid", "leaders")
	if err != nil || created {
		t.Fatalf("update: %v %v", created, err)
	}
	if user.Status != 0 || !user.DisableOtp || user.OtpSecret != secret || user.Email != "aaron@example.com" || user.Nickname != "New Name" || len(user.Groups) != 2 {
		t.Fatal("existing settings lost")
	}
	if CheckUserEnabled(user.Username) == nil {
		t.Fatal("disabled LDAP user can login")
	}
	if err := SyncExternalUser("aaron", "product", "ldap"); err != nil {
		t.Fatal(err)
	}
	if CheckUserEnabled("aaron") == nil {
		t.Fatal("login sync re-enabled account")
	}
	entry = ldap.NewEntry(entry.DN, map[string][]string{"uid": {"aaron"}, "displayName": {""}, "mail": {"new@example.com"}})
	user, created, err = SyncLDAPEntry(entry, "uid", "product")
	if err != nil || created || user.Nickname != "aaron" || user.Email != "new@example.com" || len(user.Groups) != 2 {
		t.Fatal("fallback, update or merge incorrect")
	}
	local := User{Username: "local", AuthType: "local", Status: 1}
	if err := Add(&local); err != nil {
		t.Fatal(err)
	}
	if _, _, err := SyncLDAPEntry(ldap.NewEntry("", map[string][]string{"uid": {"local"}}), "uid", "product"); err == nil {
		t.Fatal("local account overwritten")
	}
}

func TestUserBatchAndFilters(t *testing.T) {
	setupUserManageDB(t)
	for _, u := range []User{
		{Username: "alice", Nickname: "Alice", AuthType: "ldap", Groups: []string{"product"}, Status: 1, DisableOtp: true},
		{Username: "alina", Nickname: "Alina", AuthType: "ldap", Groups: []string{"product"}, Status: 1, DisableOtp: true},
		{Username: "albert", AuthType: "ldap", Groups: []string{"product-admin"}, Status: 0},
		{Username: "bob", AuthType: "local", Groups: []string{"product"}, Status: 1},
	} {
		if err := Add(&u); err != nil {
			t.Fatal(err)
		}
	}
	filter := UserFilter{Search: "ali", Group: "product", AuthType: "ldap", Status: "1", OTP: "disabled"}
	users, count, err := ListUsers(filter, 1, 1)
	if err != nil || count != 2 || len(users) != 1 || users[0].Username != "alice" {
		t.Fatalf("first page: %d %v", count, err)
	}
	users, count, err = ListUsers(filter, 2, 1)
	if err != nil || count != 2 || len(users) != 1 || users[0].Username != "alina" {
		t.Fatalf("second page: %d %v", count, err)
	}
	filter.Group = ""
	_, count, err = ListUsers(filter, 1, 10)
	if err != nil || count != 2 {
		t.Fatalf("SQL filters: %d %v", count, err)
	}
	user, err := UserBatchAction(users[0].Id, "otp_enable")
	if err != nil || user.DisableOtp || user.OtpSecret == "" {
		t.Fatal("OTP enable failed")
	}
	secret := user.OtpSecret
	if _, err := UserBatchAction(user.Id, "otp_disable"); err != nil {
		t.Fatal(err)
	}
	user, err = UserBatchAction(user.Id, "otp_enable")
	if err != nil || secret != user.OtpSecret {
		t.Fatal("OTP secret reset")
	}
	if _, err := UserBatchAction(user.Id, "disable"); err != nil {
		t.Fatal(err)
	}
	if CheckUserEnabled(user.Username) == nil {
		t.Fatal("disabled user allowed")
	}
	if _, err := UserBatchAction(user.Id, "enable"); err != nil {
		t.Fatal(err)
	}
	if err := CheckUserEnabled(user.Username); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-time.Hour)
	if _, err := xdb.ID(user.Id).Cols("limittime").Update(&User{LimitTime: &past}); err != nil {
		t.Fatal(err)
	}
	if _, err := UserBatchAction(user.Id, "enable"); err == nil {
		t.Fatal("expired user enabled")
	}
	if _, err := UserBatchAction(user.Id, "delete"); err != nil {
		t.Fatal(err)
	}
	if err := CheckUserEnabled(user.Username); err == nil {
		t.Fatal("deleted user allowed")
	}
}
