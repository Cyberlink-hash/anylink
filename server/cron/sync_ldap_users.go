package cron

import (
	"github.com/bjdgyc/anylink/base"
	"github.com/bjdgyc/anylink/dbdata"
)

func SyncLDAPUsersStatus() {
	affected, err := dbdata.SyncLDAPUsersStatus()
	base.Info("Cron SyncLDAPUsersStatus: ", affected, err)
}
