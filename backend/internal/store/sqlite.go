// Package store 封装 SQLite 持久层（WAL 模式，免运维）。
package store

import (
	"database/sql"
	"fmt"
	"net"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
	"github.com/ipambox/ipambox/internal/models"
)

const schema = `
PRAGMA journal_mode=WAL;
CREATE TABLE IF NOT EXISTS subnets (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  cidr TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  vlan INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS addresses (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  subnet_id INTEGER NOT NULL REFERENCES subnets(id) ON DELETE CASCADE,
  ip TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'free',
  mac TEXT NOT NULL DEFAULT '',
  vendor TEXT NOT NULL DEFAULT '',
  hostname TEXT NOT NULL DEFAULT '',
  label TEXT NOT NULL DEFAULT '',
  owner TEXT NOT NULL DEFAULT '',
  dev_type TEXT NOT NULL DEFAULT '',
  first_seen DATETIME,
  last_seen DATETIME,
  UNIQUE(subnet_id, ip)
);
CREATE TABLE IF NOT EXISTS scan_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ip TEXT NOT NULL,
  mac TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL,
  seen_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_events_ip ON scan_events(ip, seen_at);
CREATE TABLE IF NOT EXISTS alerts (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  type TEXT NOT NULL,
  level TEXT NOT NULL DEFAULT 'info',
  message TEXT NOT NULL,
  params TEXT NOT NULL DEFAULT '',
  ip TEXT NOT NULL DEFAULT '',
  read INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS uplink_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  online INTEGER NOT NULL,
  detail TEXT NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS pending_notifications (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  title TEXT NOT NULL,
  text TEXT NOT NULL,
  event_type TEXT NOT NULL DEFAULT '',
  attempts INTEGER NOT NULL DEFAULT 0,
  last_error TEXT NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`

// Store 是持久层句柄。
type Store struct {
	db   *sql.DB
	path string
}

// Open 打开（必要时创建）数据库并执行迁移；自动创建父目录。
func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}
	db, err := sql.Open("sqlite3", path+"?_busy_timeout=5000")
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	// 增量迁移：为 subnets 增加 iface 列（已存在则忽略）
	var hasIface bool
	rows, err := db.Query(`PRAGMA table_info(subnets)`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err == nil && name == "iface" {
			hasIface = true
		}
	}
	rows.Close()
	if !hasIface {
		if _, err := db.Exec(`ALTER TABLE subnets ADD COLUMN iface TEXT NOT NULL DEFAULT ''`); err != nil {
			return nil, fmt.Errorf("migrate iface: %w", err)
		}
	}
	// 增量迁移：为 alerts 增加 params 列（已存在则忽略）
	var hasParams bool
	rows2, err := db.Query(`PRAGMA table_info(alerts)`)
	if err != nil {
		return nil, err
	}
	for rows2.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows2.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err == nil && name == "params" {
			hasParams = true
		}
	}
	rows2.Close()
	if !hasParams {
		if _, err := db.Exec(`ALTER TABLE alerts ADD COLUMN params TEXT NOT NULL DEFAULT ''`); err != nil {
			return nil, fmt.Errorf("migrate alerts params: %w", err)
		}
	}
	return &Store{db: db, path: path}, nil
}

// Close 关闭底层连接。
func (s *Store) Close() error { return s.db.Close() }

// DataPath 返回当前数据库文件路径。
func (s *Store) DataPath() string { return s.path }

// ---- 设置（KV） ----

func (s *Store) GetSetting(key string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}

func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
		key, value)
	return err
}

// ---- 子网 ----

func (s *Store) CreateSubnet(sub *models.Subnet) error {
	res, err := s.db.Exec(
		`INSERT INTO subnets(cidr,name,description,vlan,iface) VALUES(?,?,?,?,?)`,
		sub.CIDR, sub.Name, sub.Description, sub.VLAN, sub.Iface)
	if err != nil {
		return err
	}
	sub.ID, _ = res.LastInsertId()
	return nil
}

func (s *Store) ListSubnets() ([]models.Subnet, error) {
	rows, err := s.db.Query(`SELECT id,cidr,name,description,vlan,iface,created_at FROM subnets ORDER BY cidr`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Subnet{}
	for rows.Next() {
		var sub models.Subnet
		if err := rows.Scan(&sub.ID, &sub.CIDR, &sub.Name, &sub.Description, &sub.VLAN, &sub.Iface, &sub.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, sub)
	}
	return out, rows.Err()
}

func (s *Store) GetSubnet(id int64) (*models.Subnet, error) {
	var sub models.Subnet
	err := s.db.QueryRow(`SELECT id,cidr,name,description,vlan,iface,created_at FROM subnets WHERE id=?`, id).
		Scan(&sub.ID, &sub.CIDR, &sub.Name, &sub.Description, &sub.VLAN, &sub.Iface, &sub.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &sub, nil
}

func (s *Store) DeleteSubnet(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// 级联清除该子网的地址记录，避免残留孤儿数据
	if _, err := tx.Exec(`DELETE FROM addresses WHERE subnet_id=?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM subnets WHERE id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// UpdateSubnet 更新子网的名称/描述/VLAN/接入网卡（CIDR 创建后不可改）。
func (s *Store) UpdateSubnet(sub *models.Subnet) error {
	_, err := s.db.Exec(
		`UPDATE subnets SET name=?, description=?, vlan=?, iface=? WHERE id=?`,
		sub.Name, sub.Description, sub.VLAN, sub.Iface, sub.ID)
	return err
}

// CreateAddress 人工登记一条地址记录（保留/规划预留等），已存在则报错。
func (s *Store) CreateAddress(a *models.IPAddress) error {
	res, err := s.db.Exec(
		`INSERT INTO addresses(subnet_id,ip,status,mac,hostname,label,owner,dev_type)
		 VALUES(?,?,?,?,?,?,?,?)`,
		a.SubnetID, a.IP, a.Status, a.MAC, a.Hostname, a.Label, a.Owner, a.DevType)
	if err != nil {
		return err
	}
	a.ID, _ = res.LastInsertId()
	return nil
}

// DeleteAddress 删除一条地址记录（回收台账条目）。
func (s *Store) DeleteAddress(id int64) error {
	_, err := s.db.Exec(`DELETE FROM addresses WHERE id=?`, id)
	return err
}

func (s *Store) MarkAlertRead(id int64) error {
	_, err := s.db.Exec(`UPDATE alerts SET read=1 WHERE id=?`, id)
	return err
}

// ResetAll 清空全部业务数据并解除初始化标记（保留 AI 配置）。
// 用于"重新运行初始化向导"：执行后 setup/status 将返回未初始化。
func (s *Store) ResetAll() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmts := []string{
		`DELETE FROM scan_events`,
		`DELETE FROM alerts`,
		`DELETE FROM addresses`,
		`DELETE FROM subnets`,
		`DELETE FROM uplink_events`,
		`DELETE FROM pending_notifications`,
		`DELETE FROM settings WHERE key = 'password_hash'`,
	}
	for _, q := range stmts {
		if _, err := tx.Exec(q); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GlobalStats 汇总所有子网的仪表盘数据。
// capacityOf 计算网段可用主机数（/24 → 254；/31、/32 按实际计）。
func capacityOf(cidr string) int {
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return 0
	}
	ones, bits := ipnet.Mask.Size()
	n := 1 << (bits - ones)
	if n > 2 {
		n -= 2 // 去掉网络地址与广播地址
	}
	return n
}

func (s *Store) GlobalStats() (map[string]int, error) {
	out := map[string]int{"total": 0, "online": 0, "offline": 0, "free": 0, "conflict": 0, "rogue": 0, "reserved": 0}
	rows, err := s.db.Query(`SELECT status, COUNT(*) FROM addresses GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	used := 0
	for rows.Next() {
		var st string
		var n int
		if err := rows.Scan(&st, &n); err != nil {
			return nil, err
		}
		out[st] += n
		if st != "free" {
			used += n
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// 总量按全部受管网段的容量计算，而不是已观测记录数
	subs, err := s.ListSubnets()
	if err != nil {
		return nil, err
	}
	cap := 0
	for _, sub := range subs {
		cap += capacityOf(sub.CIDR)
	}
	out["total"] = cap
	out["free"] = cap - used
	if out["free"] < 0 {
		out["free"] = 0
	}
	return out, nil
}

// ListAddresses 返回子网内全部地址记录；空切片表示尚未初始化网格。
func (s *Store) ListAddresses(subnetID int64) ([]models.IPAddress, error) {
	rows, err := s.db.Query(
		`SELECT id,subnet_id,ip,status,mac,vendor,hostname,label,owner,dev_type,first_seen,last_seen
		 FROM addresses WHERE subnet_id=? ORDER BY length(ip), ip`, subnetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.IPAddress{}
	for rows.Next() {
		var a models.IPAddress
		var first, last sql.NullTime
		err := rows.Scan(&a.ID, &a.SubnetID, &a.IP, &a.Status, &a.MAC, &a.Vendor,
			&a.Hostname, &a.Label, &a.Owner, &a.DevType, &first, &last)
		if err != nil {
			return nil, err
		}
		a.FirstSeen, a.LastSeen = first.Time, last.Time
		out = append(out, a)
	}
	// 修正 IP 排序：按长度+字典序可正确处理点分十进制
	return out, rows.Err()
}

// DeviceRow 设备台账视图：地址记录 + 所属子网信息。
type DeviceRow struct {
	models.IPAddress
	SubnetCIDR string `json:"subnet_cidr"`
	SubnetName string `json:"subnet_name"`
}

// ListAllDevices 跨子网列出全部地址记录，可按关键词/状态过滤。
func (s *Store) ListAllDevices() ([]DeviceRow, error) {
	rows, err := s.db.Query(`
		SELECT a.id,a.subnet_id,a.ip,a.status,a.mac,a.vendor,a.hostname,a.label,a.owner,a.dev_type,
		       a.first_seen,a.last_seen, s.cidr, s.name
		FROM addresses a JOIN subnets s ON s.id=a.subnet_id
		ORDER BY s.cidr, length(a.ip), a.ip`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DeviceRow{}
	for rows.Next() {
		var d DeviceRow
		var first, last sql.NullTime
		err := rows.Scan(&d.ID, &d.SubnetID, &d.IP, &d.Status, &d.MAC, &d.Vendor,
			&d.Hostname, &d.Label, &d.Owner, &d.DevType, &first, &last, &d.SubnetCIDR, &d.SubnetName)
		if err != nil {
			return nil, err
		}
		d.FirstSeen, d.LastSeen = first.Time, last.Time
		out = append(out, d)
	}
	return out, rows.Err()
}

// MarkOfflineMissing 一轮扫描结束后调用：
// 子网内状态为 online 但本轮未再出现的地址降级为 offline。返回被降级的 IP 列表。
func (s *Store) MarkOfflineMissing(subnetID int64, seen map[string]bool) ([]string, error) {
	if seen == nil {
		seen = map[string]bool{}
	}
	addrs, err := s.ListAddresses(subnetID)
	if err != nil {
		return nil, err
	}
	missing := []string{}
	for _, a := range addrs {
		if a.Status == models.StatusOnline && !seen[a.IP] {
			missing = append(missing, a.IP)
		}
	}
	if len(missing) == 0 {
		return nil, nil
	}
	// 逐个更新（数量通常很少；SQLite 不支持 IN 数组绑定）
	var gone []string
	for _, ip := range missing {
		res, err := s.db.Exec(
			`UPDATE addresses SET status='offline' WHERE subnet_id=? AND ip=? AND status='online'`,
			subnetID, ip)
		if err != nil {
			return gone, err
		}
		if c, _ := res.RowsAffected(); c > 0 {
			gone = append(gone, ip)
		}
	}
	return gone, nil
}
func (s *Store) UpdateAnnotation(id int64, label, owner, devType string) error {
	_, err := s.db.Exec(`UPDATE addresses SET label=?,owner=?,dev_type=? WHERE id=?`,
		label, owner, devType, id)
	return err
}

// RenameAnnotationRefs 级联重命名台账引用：kind 为 dev_types 或 owners，返回更新行数。
func (s *Store) RenameAnnotationRefs(kind, from, to string) (int64, error) {
	col := "dev_type"
	if kind == "owners" {
		col = "owner"
	}
	res, err := s.db.Exec(`UPDATE addresses SET `+col+`=? WHERE `+col+`=?`, to, from)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// UpdateAnnotationByIP 按 (子网, IP) 更新标注，供 CSV 导入使用；返回是否命中已有记录。
func (s *Store) UpdateAnnotationByIP(subnetID int64, ip, label, owner, devType string) (bool, error) {
	res, err := s.db.Exec(
		`UPDATE addresses SET label=?,owner=?,dev_type=? WHERE subnet_id=? AND ip=?`,
		label, owner, devType, subnetID, ip)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// SubnetIDForIP 在 Go 侧做 CIDR 包含判断，返回该 IP 所属子网。
func (s *Store) SubnetIDForIP(ipStr string) (int64, bool, error) {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return 0, false, nil
	}
	subs, err := s.ListSubnets()
	if err != nil {
		return 0, false, err
	}
	for _, sub := range subs {
		_, ipnet, err := net.ParseCIDR(sub.CIDR)
		if err == nil && ipnet.Contains(ip) {
			return sub.ID, true, nil
		}
	}
	return 0, false, nil
}

// MarkSeen 由扫描引擎调用：插入事件并刷新地址状态。
// mac/hostname 为空时保留已有值，避免后续观测把已获取的信息清空。
func (s *Store) MarkSeen(ip, mac, hostname, source string) error {
	subnetID, ok, err := s.SubnetIDForIP(ip)
	if err != nil {
		return err
	}
	if !ok {
		return nil // 未受管网段的观测直接忽略
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO scan_events(ip,mac,source) VALUES(?,?,?)`, ip, mac, source); err != nil {
		return err
	}
	_, err = tx.Exec(`
		INSERT INTO addresses(subnet_id,ip,status,mac,hostname,first_seen,last_seen)
		VALUES(?, ?, 'online', ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(subnet_id,ip) DO UPDATE SET
		  status='online', last_seen=CURRENT_TIMESTAMP,
		  mac=CASE WHEN excluded.mac='' THEN addresses.mac ELSE excluded.mac END,
		  hostname=CASE WHEN excluded.hostname='' THEN addresses.hostname ELSE excluded.hostname END`,
		subnetID, ip, mac, hostname)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// ---- 告警 ----

func (s *Store) CreateAlert(a *models.Alert) error {
	_, err := s.db.Exec(`INSERT INTO alerts(type,level,message,params,ip) VALUES(?,?,?,?,?)`,
		a.Type, a.Level, a.Message, a.Params, a.IP)
	return err
}

func (s *Store) ListAlerts(unreadOnly bool) ([]models.Alert, error) {
	q := `SELECT id,type,level,message,params,ip,read,created_at FROM alerts`
	if unreadOnly {
		q += ` WHERE read=0`
	}
	q += ` ORDER BY id DESC LIMIT 200`
	rows, err := s.db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Alert{}
	for rows.Next() {
		var a models.Alert
		if err := rows.Scan(&a.ID, &a.Type, &a.Level, &a.Message, &a.Params, &a.IP, &a.Read, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// Stats 计算单个子网的聚合统计。
// Total 取网段容量（而非已观测记录数），使用率 = 非闲置观测数 / 容量。
func (s *Store) Stats(subnetID int64) (*models.SubnetStats, error) {
	st := &models.SubnetStats{SubnetID: subnetID}
	sub, err := s.GetSubnet(subnetID)
	if err != nil {
		return nil, err
	}
	err = s.db.QueryRow(`
		SELECT COUNT(*),
		  COALESCE(SUM(status='online'),0),
		  COALESCE(SUM(status='offline'),0),
		  COALESCE(SUM(status='free'),0),
		  COALESCE(SUM(status='conflict'),0)
		FROM addresses WHERE subnet_id=?`, subnetID).
		Scan(&st.Total, &st.Online, &st.Offline, &st.Free, &st.Conflict)
	if err != nil {
		return nil, err
	}
	observed := st.Total - st.Free // 表中 free 记录不算占用
	st.Total = capacityOf(sub.CIDR)
	used := observed
	st.Free = st.Total - used
	if st.Free < 0 {
		st.Free = 0
	}
	if st.Total > 0 {
		st.UsagePct = float64(used) / float64(st.Total) * 100
	}
	return st, nil
}
