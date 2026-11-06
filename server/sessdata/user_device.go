package sessdata

import (
	"sync"

	"github.com/bjdgyc/anylink/dbdata"
)

// DeviceAccessMu serializes admission/session registration with administrative revocation.
// Never acquire it while holding sessMux or a Session mutex.
var DeviceAccessMu sync.Mutex

func (s *Session) DeviceIdentity() dbdata.DeviceIdentity {
	return dbdata.DeviceIdentity{UniqueIdHash: s.UniqueIdHash, UniqueIdGlobalHash: s.UniqueIdGlobalHash}
}

// ReconcileUserDevices must be called with DeviceAccessMu held, after committing changes.
func ReconcileUserDevices(userID int, username string, blockedIDs []int) (int, error) {
	management, err := dbdata.GetUserDevices(userID)
	if err != nil {
		return 0, err
	}
	tokens := []string{}
	sessMux.RLock()
	for token, s := range sessions {
		if s.Username != username {
			continue
		}
		identity := s.DeviceIdentity()
		allowed := !management.Policy.Enabled
		blocked := s.UserId != 0 && s.UserId != userID
		for _, d := range management.Devices {
			if !d.Matches(identity) {
				continue
			}
			if d.State == dbdata.DeviceApproved {
				allowed = true
			}
			for _, id := range blockedIDs {
				if d.Id == id {
					blocked = true
				}
			}
		}
		if !allowed || blocked {
			tokens = append(tokens, token)
		}
	}
	sessMux.RUnlock()
	for _, token := range tokens {
		CloseSess(token, dbdata.UserLogoutDevice)
	}
	return len(tokens), nil
}

func (s *Session) CheckDeviceAccess() error {
	return dbdata.CheckDeviceAccess(s.Username, s.UserId, s.DeviceIdentity())
}

func (cs *ConnSession) IsCurrent() bool {
	if Token2Sess(cs.Sess.Token) != cs.Sess {
		return false
	}
	cs.Sess.mux.RLock()
	defer cs.Sess.mux.RUnlock()
	return cs.Sess.IsActive && cs.Sess.CSess == cs
}
