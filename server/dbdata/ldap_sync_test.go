package dbdata

import (
	"errors"
	"path/filepath"
	"reflect"
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

func TestLDAPIdentityCaseInsensitive(t *testing.T) {
	setupUserManageDB(t)
	original := User{Username: "Bob", AuthType: "ldap", Status: 0, DisableOtp: false, OtpSecret: "KEEP", Groups: []string{"ops"}}
	if err := Add(&original); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"bob", "Bob", "BOB"} {
		user, err := FindLDAPUser(name)
		if err != nil || user.Id != original.Id || user.OtpSecret != "KEEP" || user.Status != 0 || user.DisableOtp {
			t.Fatalf("identity %s: %v %+v", name, err, user)
		}
	}
	entry := ldap.NewEntry("uid=bob,dc=example,dc=com", map[string][]string{"uid": {"bob"}, "displayName": {"Bob Name"}, "mail": {"bob@example.com"}})
	user, created, err := SyncLDAPEntry(entry, "uid", "product")
	if err != nil || created || user.Id != original.Id || user.Username != "bob" || user.OtpSecret != "KEEP" || user.Status != 0 {
		t.Fatalf("sync changed identity: %v %v %+v", created, err, user)
	}
	duplicate := User{Username: "bob", AuthType: "ldap", Status: 1, DisableOtp: true}
	if err := Add(&duplicate); err == nil {
		t.Fatal("duplicate identity accepted")
	}
}

func TestLDAPIdentityRejectsLocalCollision(t *testing.T) {
	setupUserManageDB(t)
	if err := Add(&User{Username: "Bob", AuthType: "local", Status: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := FindLDAPUser("bob"); err == nil {
		t.Fatal("local identity accepted as LDAP")
	}
}

func TestUserSearchCaseInsensitive(t *testing.T) {
	setupUserManageDB(t)
	if err := Add(&User{Username: "Bob", Nickname: "Robert", Email: "BOB@example.com"}); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"bob", "BOB", "robert", "ROBERT", "bob@EXAMPLE.COM"} {
		users, count, err := ListUsers(UserFilter{Search: query}, 1, 20)
		if err != nil || count != 1 || len(users) != 1 {
			t.Fatalf("search %s: %d %v", query, count, err)
		}
	}
}

type caseLDAPAuth struct{}

func (caseLDAPAuth) checkData(map[string]interface{}) error { return nil }
func (caseLDAPAuth) checkUser(name, pwd string, g *Group, ext map[string]interface{}) error {
	if name != "bob" || pwd != "correct" {
		return errors.New("authentication failed")
	}
	ext["ldap_username"] = "bob"
	return nil
}

func TestLDAPLoginCaseInsensitive(t *testing.T) {
	base.Test()
	setupUserManageDB(t)
	old := authRegistry["ldap"]
	authRegistry["ldap"] = reflect.TypeOf(caseLDAPAuth{})
	defer func() { authRegistry["ldap"] = old }()
	if err := Add(&Group{Name: "ops", Status: 1, Auth: map[string]interface{}{"type": "ldap"}}); err != nil {
		t.Fatal(err)
	}
	if err := CheckUser("BOB", "correct", "ops", nil); err == nil {
		t.Fatal("unsynced LDAP login accepted")
	}
	if err := SyncExternalUser("BOB", "ops", "ldap"); err == nil {
		t.Fatal("LDAP auto provisioning accepted")
	}
	if CountAll(&User{}) != 0 {
		t.Fatal("unsynced login created user")
	}
	user := User{Username: "bob", AuthType: "ldap", Status: 1, OtpSecret: "KEEP", DisableOtp: false, Groups: []string{"ops"}}
	if err := Add(&user); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"bob", "Bob", "BOB"} {
		ext := map[string]interface{}{}
		if err := CheckUser(name, "correct", "ops", ext); err != nil {
			t.Fatal(err)
		}
		if ext["authenticated_username"] != "bob" {
			t.Fatalf("session identity not canonical: %+v", ext)
		}
	}
	if CountAll(&User{}) != 1 {
		t.Fatal("duplicate created")
	}
	stored, err := FindLDAPUser("BOB")
	if err != nil || stored.OtpSecret != "KEEP" || stored.DisableOtp {
		t.Fatalf("OTP changed: %+v %v", stored, err)
	}
	if err := CheckUser("BOB", "wrong", "ops", nil); err == nil {
		t.Fatal("incorrect password accepted")
	}
	if _, err := UserBatchAction(user.Id, "disable"); err != nil {
		t.Fatal(err)
	}
	if err := CheckUser("BOB", "correct", "ops", nil); err == nil {
		t.Fatal("disabled user accepted")
	}
}

func TestLDAPSyncNormalizesDirectoryUsername(t *testing.T) {
	setupUserManageDB(t)
	entry := ldap.NewEntry("uid=BOB,dc=example,dc=com", map[string][]string{"uid": {"BOB"}, "displayName": {"Bob"}, "mail": {"bob@example.com"}})
	user, created, err := SyncLDAPEntry(entry, "uid", "ops")
	if err != nil || !created || user.Username != "bob" || user.DisableOtp {
		t.Fatalf("LDAP create: %v %v %+v", err, created, user)
	}
	secret, id := user.OtpSecret, user.Id
	user, created, err = SyncLDAPEntry(entry, "uid", "product")
	if err != nil || created || user.Id != id || user.OtpSecret != secret || CountAll(&User{}) != 1 {
		t.Fatalf("LDAP repeat sync: %v %v %+v", err, created, user)
	}
}
