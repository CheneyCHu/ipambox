package store

// 断网续存相关的持久化：外网连通事件 + 待补发通知队列。

// UplinkEvent 一次外网连通状态变化记录。
type UplinkEvent struct {
	ID        int64  `json:"id"`
	Online    bool   `json:"online"`
	Detail    string `json:"detail"`
	CreatedAt string `json:"created_at"`
}

// PendingNotification 推送失败、等待网络恢复后补发的通知。
type PendingNotification struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	Text      string `json:"text"`
	EventType string `json:"event_type"`
	Attempts  int    `json:"attempts"`
	LastError string `json:"last_error"`
	CreatedAt string `json:"created_at"`
}

// RecordUplinkEvent 记录一次连通状态变化（online/offline）。
func (s *Store) RecordUplinkEvent(online bool, detail string) error {
	on := 0
	if online {
		on = 1
	}
	_, err := s.db.Exec(`INSERT INTO uplink_events(online,detail) VALUES(?,?)`, on, detail)
	return err
}

// ListUplinkEvents 返回最近的状态变化记录（新的在前）。
func (s *Store) ListUplinkEvents(limit int) ([]UplinkEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(
		`SELECT id,online,detail,created_at FROM uplink_events ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UplinkEvent{}
	for rows.Next() {
		var e UplinkEvent
		var on int
		if err := rows.Scan(&e.ID, &on, &e.Detail, &e.CreatedAt); err != nil {
			return nil, err
		}
		e.Online = on == 1
		out = append(out, e)
	}
	return out, rows.Err()
}

// EnqueueNotification 推送失败时入队，等待恢复后补发。
func (s *Store) EnqueueNotification(title, text, eventType, errMsg string) error {
	_, err := s.db.Exec(
		`INSERT INTO pending_notifications(title,text,event_type,last_error) VALUES(?,?,?,?)`,
		title, text, eventType, errMsg)
	return err
}

// ListPendingNotifications 返回全部待补发通知（按入队顺序）。
func (s *Store) ListPendingNotifications() ([]PendingNotification, error) {
	rows, err := s.db.Query(
		`SELECT id,title,text,event_type,attempts,last_error,created_at
		 FROM pending_notifications ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PendingNotification{}
	for rows.Next() {
		var p PendingNotification
		if err := rows.Scan(&p.ID, &p.Title, &p.Text, &p.EventType, &p.Attempts, &p.LastError, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// PendingCount 待补发通知数量。
func (s *Store) PendingCount() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM pending_notifications`).Scan(&n)
	return n, err
}

// DeletePending 补发成功（或超过重试上限放弃）后删除。
func (s *Store) DeletePending(id int64) error {
	_, err := s.db.Exec(`DELETE FROM pending_notifications WHERE id=?`, id)
	return err
}

// BumpPendingAttempt 补发失败：重试次数 +1 并记录最新错误。
func (s *Store) BumpPendingAttempt(id int64, errMsg string) error {
	_, err := s.db.Exec(
		`UPDATE pending_notifications SET attempts=attempts+1, last_error=? WHERE id=?`, errMsg, id)
	return err
}
