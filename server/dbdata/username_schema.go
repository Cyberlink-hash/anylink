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
	for _, user := range users {
		if user.Username == "" || user.Username != normalizeUsername(user.Username) {
			return fmt.Errorf("用户ID %d 的用户名不是规范小写，请先在旧版本手动清理为小写，再升级；未修改用户数据", user.Id)
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
