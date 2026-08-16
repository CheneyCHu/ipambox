package alert

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/ipambox/ipambox/internal/store"
)

type received struct {
	Body string
}

// 端到端：冲突告警 → 落库 + Webhook 推送
func TestConflictNotifiesWebhook(t *testing.T) {
	var got []received
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = append(got, received{Body: string(b)})
		w.WriteHeader(200)
	}))
	defer srv.Close()

	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for k, v := range map[string]string{
		"notify_enabled": "1", "notify_channel": "webhook",
		"notify_webhook": srv.URL, "notify_events": "conflict",
	} {
		if err := db.SetSetting(k, v); err != nil {
			t.Fatal(err)
		}
	}

	a := New(db)
	a.CheckConflict("10.0.0.5", "aa:aa:aa:aa:aa:01") // 首次：基线，不告警
	a.CheckConflict("10.0.0.5", "aa:aa:aa:aa:aa:02") // MAC 变化 → 告警

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && len(got) == 0 {
		time.Sleep(20 * time.Millisecond)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 webhook call, got %d", len(got))
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(got[0].Body), &payload); err != nil {
		t.Fatalf("bad payload: %v", err)
	}
	if payload["title"] != "严重告警" {
		t.Fatalf("unexpected title: %v", payload["title"])
	}

	// 落库确认
	alerts, err := db.ListAlerts(false)
	if err != nil || len(alerts) != 1 || alerts[0].Type != "conflict" {
		t.Fatalf("alert not persisted: %v %v", alerts, err)
	}
}

// 事件过滤：只订阅 offline 时，conflict 不推送
func TestEventFilter(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer srv.Close()

	db, _ := store.Open(filepath.Join(t.TempDir(), "t.db"))
	defer db.Close()
	db.SetSetting("notify_enabled", "1")
	db.SetSetting("notify_channel", "webhook")
	db.SetSetting("notify_webhook", srv.URL)
	db.SetSetting("notify_events", "offline")

	a := New(db)
	a.CheckConflict("10.0.0.6", "mac1")
	a.CheckConflict("10.0.0.6", "mac2")
	time.Sleep(200 * time.Millisecond)
	if called {
		t.Fatal("conflict event should not be pushed when only offline subscribed")
	}
}

// 钉钉加签 URL 生成
func TestDingTalkSign(t *testing.T) {
	u := signDingTalk("https://oapi.dingtalk.com/robot/send?access_token=abc", "SEC123")
	if u == "" || !containsAll(u, []string{"timestamp=", "sign="}) {
		t.Fatalf("bad signed url: %s", u)
	}
}

func containsAll(s string, subs []string) bool {
	for _, x := range subs {
		if !bytes.Contains([]byte(s), []byte(x)) {
			return false
		}
	}
	return true
}
