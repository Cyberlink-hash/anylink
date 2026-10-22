package dbdata

import "testing"

func TestUserWriteNormalizesAndRejectsDuplicates(t *testing.T) {
	setupUserManageDB(t)
	if err := Add(&Group{Name: "ops", Status: 1}); err != nil {
		t.Fatal(err)
	}
	user := User{Username: "Bob", Groups: []string{"ops"}, Status: 1, OtpSecret: "KEEP"}
	if err := SetUser(&user); err != nil {
		t.Fatal(err)
	}
	if user.Username != "bob" {
		t.Fatalf("not lowercase: %s", user.Username)
	}
	for _, name := range []string{"bob", "Bob", "BOB"} {
		if err := SetUser(&User{Username: name, Groups: []string{"ops"}}); err == nil {
			t.Fatalf("duplicate %s accepted", name)
		}
		if err := InsertUser(&User{Username: name}); err == nil {
			t.Fatalf("duplicate import %s accepted", name)
		}
	}
	user.Username = "BOB"
	if err := SetUser(&user); err != nil {
		t.Fatal(err)
	}
	if user.Username != "bob" || user.OtpSecret != "KEEP" {
		t.Fatal("edit changed identity settings")
	}
	other := User{Username: "Alice", Groups: []string{"ops"}}
	if err := SetUser(&other); err != nil {
		t.Fatal(err)
	}
	other.Username = "BOB"
	if err := SetUser(&other); err == nil {
		t.Fatal("rename to existing username accepted")
	}
	imported := User{Username: "Carol"}
	if err := InsertUser(&imported); err != nil {
		t.Fatal(err)
	}
	if imported.Username != "carol" {
		t.Fatal("import not normalized")
	}
}

func TestSQLiteUsernameConstraints(t *testing.T) {
	setupUserManageDB(t)
	if err := xdb.Sync2(&User{}); err != nil {
		t.Fatalf("schema sync after constraint installation: %v", err)
	}
	if err := ensureUsernameConstraints(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Bob", "BOB", " bob ", ""} {
		if _, err := xdb.Exec(`INSERT INTO "user" (username) VALUES (?)`, name); err == nil {
			t.Fatalf("raw invalid username %q accepted", name)
		}
	}
	if _, err := xdb.Exec(`INSERT INTO "user" (username) VALUES ('bob')`); err != nil {
		t.Fatal(err)
	}
	if _, err := xdb.Exec(`INSERT INTO "user" (username) VALUES ('bob')`); err == nil {
		t.Fatal("raw duplicate accepted")
	}
	if _, err := xdb.Exec(`UPDATE "user" SET username = 'BOB' WHERE username = 'bob'`); err == nil {
		t.Fatal("raw uppercase update accepted")
	}
	if _, err := xdb.Exec(`UPDATE "user" SET status = 0 WHERE username = 'bob'`); err != nil {
		t.Fatal(err)
	}
}

func TestSQLiteUsernameUpgradeRejectsLegacyWithoutChangingData(t *testing.T) {
	setupUserManageDB(t)
	for _, sql := range []string{`DROP TRIGGER user_username_lower_insert`, `DROP TRIGGER user_username_lower_update`} {
		if _, err := xdb.Exec(sql); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"bob", "Bob"} {
		if _, err := xdb.Exec(`INSERT INTO "user" (username) VALUES (?)`, name); err != nil {
			t.Fatal(err)
		}
	}
	if err := ensureUsernameConstraints(); err == nil {
		t.Fatal("duplicate legacy data accepted")
	}
	var users []User
	if err := xdb.Asc("id").Find(&users); err != nil {
		t.Fatal(err)
	}
	if len(users) != 2 || users[0].Username != "bob" || users[1].Username != "Bob" {
		t.Fatalf("legacy data changed: %+v", users)
	}
	if _, err := xdb.Exec(`DELETE FROM "user" WHERE username = 'bob'`); err != nil {
		t.Fatal(err)
	}
	if err := ensureUsernameConstraints(); err != nil {
		t.Fatal(err)
	}
	if _, err := xdb.Exec(`INSERT INTO "user" (username) VALUES ('BOB')`); err == nil {
		t.Fatal("constraints missing after cleanup")
	}
}

func TestConcurrentUsernameInsert(t *testing.T) {
	setupUserManageDB(t)
	xdb.SetMaxOpenConns(1)
	results := make(chan error, 12)
	start := make(chan struct{})
	for i := 0; i < cap(results); i++ {
		go func() {
			<-start
			_, err := xdb.Exec(`INSERT INTO "user" (username) VALUES ('bob')`)
			results <- err
		}()
	}
	close(start)
	success := 0
	for i := 0; i < cap(results); i++ {
		if <-results == nil {
			success++
		}
	}
	if success != 1 || CountAll(&User{}) != 1 {
		t.Fatalf("concurrent inserts: %d successful", success)
	}
}

func TestSQLiteUsernameUpgradeMigratesReferences(t *testing.T) {
	setupUserManageDB(t)
	for _, sql := range []string{`DROP TRIGGER user_username_lower_insert`, `DROP TRIGGER user_username_lower_update`} {
		if _, err := xdb.Exec(sql); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := xdb.Exec(`INSERT INTO "user" (id, username, auth_type, otp_secret, pin_code, disable_otp, status, groups) VALUES (9, 'Bob', 'ldap', 'KEEP-OTP', 'KEEP-PASSWORD', 0, 0, '["ops"]')`); err != nil {
		t.Fatal(err)
	}
	policy := Policy{Username: "Bob", Status: 1, RouteInclude: []ValData{{Val: "10.30.0.0/24"}}}
	ip := IpMap{Username: "Bob", IpAddr: "10.0.0.2", MacAddr: "00:01:02:03:04:05", Keep: true}
	network := NetworkPolicy{Name: "test", Usernames: []string{"Bob", "bob", "alice"}}
	audit := UserActLog{Username: "Bob"}
	for _, value := range []interface{}{&policy, &ip, &network, &audit} {
		if err := Add(value); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		if err := ensureUsernameConstraints(); err != nil {
			t.Fatal(err)
		}
	}
	var user User
	if err := One("Id", 9, &user); err != nil {
		t.Fatal(err)
	}
	if user.Username != "bob" || user.OtpSecret != "KEEP-OTP" || user.PinCode != "KEEP-PASSWORD" || user.Status != 0 || user.DisableOtp || len(user.Groups) != 1 || user.Groups[0] != "ops" {
		t.Fatalf("account settings changed: %+v", user)
	}
	policy = Policy{Id: policy.Id}
	if err := One("Id", policy.Id, &policy); err != nil {
		t.Fatal(err)
	}
	if policy.Username != "bob" || policy.Status != 1 || policy.RouteInclude[0].Val != "10.30.0.0/24" {
		t.Fatal("user route policy changed")
	}
	ip = IpMap{Id: ip.Id}
	if err := One("Id", ip.Id, &ip); err != nil {
		t.Fatal(err)
	}
	if ip.Username != "bob" || !ip.Keep || ip.IpAddr != "10.0.0.2" {
		t.Fatal("IP mapping changed")
	}
	network = NetworkPolicy{Id: network.Id}
	if err := One("Id", network.Id, &network); err != nil {
		t.Fatal(err)
	}
	if len(network.Usernames) != 2 || network.Usernames[0] != "bob" || network.Usernames[1] != "alice" {
		t.Fatalf("network references: %+v", network.Usernames)
	}
	audit = UserActLog{Id: audit.Id}
	if err := One("Id", audit.Id, &audit); err != nil {
		t.Fatal(err)
	}
	if audit.Username != "Bob" {
		t.Fatal("historical audit was rewritten")
	}
	if err := xdb.Sync2(&User{}); err != nil {
		t.Fatal(err)
	}
}

func TestSQLiteUsernameUpgradeRollsBackPolicyConflict(t *testing.T) {
	setupUserManageDB(t)
	if _, err := xdb.Exec(`DROP TRIGGER user_username_lower_insert`); err != nil {
		t.Fatal(err)
	}
	if _, err := xdb.Exec(`INSERT INTO "user" (username) VALUES ('Bob')`); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Bob", "bob"} {
		if err := Add(&Policy{Username: name}); err != nil {
			t.Fatal(err)
		}
	}
	if err := ensureUsernameConstraints(); err == nil {
		t.Fatal("policy collision accepted")
	}
	var users []User
	if err := xdb.Find(&users); err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 || users[0].Username != "Bob" || CountAll(&Policy{}) != 2 {
		t.Fatal("failed migration was not rolled back")
	}
}
