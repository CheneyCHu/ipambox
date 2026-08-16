// Package store — 备份与恢复：SQLite 在线快照导出 / 校验导入 / 热替换。
package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"
	"strings"
)

// ExportSnapshot 用 VACUUM INTO 生成一致性快照（WAL 下安全），返回临时文件路径与清理函数。
func (s *Store) ExportSnapshot() (path string, cleanup func(), err error) {
	tmp := filepath.Join(os.TempDir(), fmt.Sprintf("ipambox-backup-%d-%d.db", os.Getpid(), time.Now().UnixNano()))
	_ = os.Remove(tmp)
	if _, err := s.db.Exec(`VACUUM INTO ?`, tmp); err != nil {
		return "", nil, fmt.Errorf("生成快照失败: %w", err)
	}
	return tmp, func() { _ = os.Remove(tmp) }, nil
}

// ReplaceWith 用上传的备份文件热替换当前数据库。
// 先只读校验文件是合法的 IPAMBox 库（含核心表），再关库、替换文件、重开并迁移。
func (s *Store) ReplaceWith(srcPath string) error {
	if err := validateBackup(srcPath); err != nil {
		return err
	}
	dst := s.DataPath()
	if dst == "" {
		return fmt.Errorf("无法定位当前数据库文件")
	}
	// 关库 → 清 WAL 残余 → 覆盖 → 重开（Open 会顺带执行增量迁移）
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("关闭当前数据库失败: %w", err)
	}
	_ = os.Remove(dst + "-wal")
	_ = os.Remove(dst + "-shm")
	if err := copyFile(srcPath, dst); err != nil {
		// 复制失败时尽量重开旧库（可能已损坏，但比退出强）
		if db, e := Open(dst); e == nil {
			s.db = db.db
		}
		return fmt.Errorf("写入数据库文件失败: %w", err)
	}
	db, err := Open(dst)
	if err != nil {
		return fmt.Errorf("重开数据库失败: %w", err)
	}
	s.db = db.db
	return nil
}

// validateBackup 只读打开备份文件，校验核心表存在。
func validateBackup(path string) error {
	fi, err := os.Stat(path)
	if err != nil || fi.Size() == 0 {
		return fmt.Errorf("备份文件为空或不可读")
	}
	db, err := sql.Open("sqlite3", path+"?mode=ro")
	if err != nil {
		return err
	}
	defer db.Close()
	var tables string
	rows, err := db.Query(`SELECT group_concat(name) FROM sqlite_master WHERE type='table'`)
	if err != nil {
		return fmt.Errorf("不是有效的 SQLite 数据库")
	}
	defer rows.Close()
	if rows.Next() {
		_ = rows.Scan(&tables)
	}
	for _, t := range []string{"subnets", "addresses", "settings"} {
		if !strings.Contains(tables, t) {
			return fmt.Errorf("备份文件缺少核心表 %q，不是 IPAMBox 的备份", t)
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o600)
}
