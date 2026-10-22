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
	if err := ensureUsernameConstraints(); err == nil {
		t.Fatal("uppercase legacy data accepted")
	}
	if _, err := xdb.Exec(`UPDATE "user" SET username = 'bob'`); err != nil {
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
