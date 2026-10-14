package admin

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/bjdgyc/anylink/base"
	"github.com/bjdgyc/anylink/dbdata"
	"github.com/go-ldap/ldap"
)

func TestLDAPSyncMailOnceAndFailure(t *testing.T) {
	oldType, oldSource := base.Cfg.DbType, base.Cfg.DbSource
	base.Cfg.DbType, base.Cfg.DbSource = "sqlite3", filepath.Join(t.TempDir(), "users.db")
	dbdata.Start()
	t.Cleanup(func() { dbdata.Stop(); base.Cfg.DbType, base.Cfg.DbSource = oldType, oldSource })
	g1, g2 := dbdata.Group{Name: "sync-product"}, dbdata.Group{Name: "sync-leaders"}
	if err := dbdata.Add(&g1); err != nil {
		t.Fatal(err)
	}
	if err := dbdata.Add(&g2); err != nil {
		t.Fatal(err)
	}
	calls := 0
	load := func(g *dbdata.Group) ([]*ldap.Entry, dbdata.AuthLdap, error) {
		return []*ldap.Entry{ldap.NewEntry("uid=aaron", map[string][]string{"uid": {"aaron"}, "mail": {"aaron@example.com"}})}, dbdata.AuthLdap{SearchAttr: "uid"}, nil
	}
	notify := func(u *dbdata.User) error {
		calls++
		if u.DisableOtp || u.OtpSecret == "" || len(u.Groups) != 2 {
			t.Fatal("email before full user creation")
		}
		return errors.New("simulated SMTP failure")
	}
	result := syncLDAPGroups([]int{g1.Id, g2.Id, g1.Id}, load, notify)
	if result["created"] != 1 || result["mail_failed"] != 1 || calls != 1 {
		t.Fatalf("result %v calls %d", result, calls)
	}
	result = syncLDAPGroups([]int{g1.Id, g2.Id}, load, notify)
	if result["created"] != 0 || result["updated"] != 1 || calls != 1 {
		t.Fatal("repeat sync sent mail again")
	}
	var user dbdata.User
	if err := dbdata.One("Username", "aaron", &user); err != nil || user.DisableOtp {
		t.Fatal("mail failure lost account or disabled OTP")
	}
	result = syncLDAPGroups([]int{g1.Id}, func(g *dbdata.Group) ([]*ldap.Entry, dbdata.AuthLdap, error) {
		return nil, dbdata.AuthLdap{}, errors.New("LDAP unavailable")
	}, notify)
	if result["created"] != 0 || calls != 1 {
		t.Fatal("LDAP failure created user or sent mail")
	}
	if err := dbdata.One("Username", "aaron", &user); err != nil || len(user.Groups) != 2 {
		t.Fatal("failed sync removed groups")
	}
}
