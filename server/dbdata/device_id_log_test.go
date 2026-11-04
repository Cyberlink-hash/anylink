package dbdata

import (
	"path/filepath"
	"testing"

	"xorm.io/xorm"
)

func TestUserActLogDeviceIds(t *testing.T) {
	const abcHash = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	for _, tt := range []struct {
		name, id, global, idHash, globalHash string
	}{
		{"both", "abc", "abc", abcHash, abcHash},
		{"missing id", "", "abc", "", abcHash},
		{"missing global", "abc", "", abcHash, ""},
		{"both missing", "", "", "", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			log := UserActLog{}
			log.SetDeviceIds(tt.id, tt.global)
			if !log.DeviceIdsCollected || log.UniqueIdHash != tt.idHash || log.UniqueIdGlobalHash != tt.globalHash {
				t.Fatalf("unexpected device summaries: %+v", log)
			}
		})
	}
	if deviceIdHash("ABC") == deviceIdHash("abc") || deviceIdHash(" abc ") == deviceIdHash("abc") {
		t.Fatal("device IDs must be hashed without normalization")
	}
}

func TestUserActLogDeviceIdsSchemaUpgrade(t *testing.T) {
	engine, err := xorm.NewEngine("sqlite3", filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	if _, err := engine.Exec(`CREATE TABLE user_act_log (id INTEGER PRIMARY KEY AUTOINCREMENT, username TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Exec(`INSERT INTO user_act_log (username) VALUES ('legacy')`); err != nil {
		t.Fatal(err)
	}
	if err := engine.Sync2(&UserActLog{}); err != nil {
		t.Fatal(err)
	}
	legacy := UserActLog{}
	if found, err := engine.ID(1).Get(&legacy); err != nil || !found {
		t.Fatalf("read legacy log: found=%v err=%v", found, err)
	}
	if legacy.DeviceIdsCollected || legacy.UniqueIdHash != "" || legacy.UniqueIdGlobalHash != "" {
		t.Fatalf("legacy log must remain uncollected: %+v", legacy)
	}
	for _, ids := range [][2]string{{"abc", "device-global"}, {"", ""}} {
		log := UserActLog{Username: "new"}
		log.SetDeviceIds(ids[0], ids[1])
		if _, err := engine.Insert(&log); err != nil {
			t.Fatal(err)
		}
		stored := UserActLog{}
		if found, err := engine.ID(log.Id).Get(&stored); err != nil || !found {
			t.Fatalf("read new log: found=%v err=%v", found, err)
		}
		if !stored.DeviceIdsCollected || stored.UniqueIdHash != log.UniqueIdHash || stored.UniqueIdGlobalHash != log.UniqueIdGlobalHash {
			t.Fatalf("device summaries not persisted: %+v", stored)
		}
	}
}
