package dbdata

import "fmt"

// SQLite 使用现有用户名唯一索引，加上小写校验，防止大小写变体绕过唯一性。
func ensureUsernameConstraints() error {
	if xdb.DriverName() != "sqlite3" {
		return nil
	}
	sess := xdb.NewSession()
	defer sess.Close()
	if err := sess.Begin(); err != nil {
		return err
	}
	defer sess.Rollback()
	var users []User
	if err := sess.Cols("id", "username").Find(&users); err != nil {
		return err
	}
	seen := make(map[string]int)
	for _, user := range users {
		name := normalizeUsername(user.Username)
		if id, ok := seen[name]; ok {
			return fmt.Errorf("用户名忽略大小写后重复（用户ID %d、%d），请先在旧版本手动清理，再升级；未修改用户数据", id, user.Id)
		}
		seen[name] = user.Id
	}
	renamed := make(map[string]bool)
	for _, user := range users {
		name := normalizeUsername(user.Username)
		if name == "" {
			return fmt.Errorf("用户ID %d 的用户名为空，请手动处理；未修改用户数据", user.Id)
		}
		if name == user.Username {
			continue
		}
		// 仅改用户名，避免 ORM 密码钩子或其他账户字段发生变化。
		if _, err := sess.Exec(`UPDATE "user" SET username = ? WHERE id = ?`, name, user.Id); err != nil {
			return err
		}
		for _, table := range []string{"policy", "ip_map"} {
			if _, err := sess.Exec(`UPDATE "`+table+`" SET username = ? WHERE LOWER(TRIM(username)) = ?`, name, name); err != nil {
				return fmt.Errorf("用户ID %d 的关联配置迁移失败，请检查重复用户策略；本次迁移已回滚: %w", user.Id, err)
			}
		}
		renamed[name] = true
	}
	if len(renamed) > 0 {
		var policies []NetworkPolicy
		if err := sess.Find(&policies); err != nil {
			return err
		}
		for _, policy := range policies {
			changed := false
			for i, username := range policy.Usernames {
				name := normalizeUsername(username)
				if renamed[name] && username != name {
					policy.Usernames[i] = name
					changed = true
				}
			}
			if changed {
				policy.Usernames = normalizePolicyValues(policy.Usernames)
				if _, err := sess.ID(policy.Id).Cols("usernames").Update(&policy); err != nil {
					return err
				}
			}
		}
	}

	statements := []string{
		`CREATE TRIGGER IF NOT EXISTS user_username_lower_insert BEFORE INSERT ON "user"
   WHEN NEW.username = '' OR NEW.username != LOWER(TRIM(NEW.username)) COLLATE BINARY
   BEGIN SELECT RAISE(ABORT, 'username must be nonempty lowercase without surrounding spaces'); END`,
		`CREATE TRIGGER IF NOT EXISTS user_username_lower_update BEFORE UPDATE OF username ON "user"
   WHEN NEW.username = '' OR NEW.username != LOWER(TRIM(NEW.username)) COLLATE BINARY
   BEGIN SELECT RAISE(ABORT, 'username must be nonempty lowercase without surrounding spaces'); END`,
	}
	for _, statement := range statements {
		if _, err := sess.Exec(statement); err != nil {
			return fmt.Errorf("创建SQLite用户名约束失败: %w", err)
		}
	}
	return sess.Commit()
}
