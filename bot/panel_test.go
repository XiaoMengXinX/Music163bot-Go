package bot

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// setupPanel 起一个只包含面板路由的测试服务器，并重置认证相关全局状态
func setupPanel(t *testing.T, mode string) *httptest.Server {
	t.Helper()
	old := struct {
		mode     string
		ttl      time.Duration
		sessTTL  time.Duration
		listen   string
		enabled  bool
		maxToks  int
		tokens   map[string]time.Time
		sessions map[string]time.Time
	}{panelAuthMode, panelTokenTTL, panelSessionTTL, panelListen, panelEnabled, panelMaxTokens, panelTokens, panelSessions}

	panelAuthMode = mode
	panelTokenTTL = 2 * time.Second
	panelSessionTTL = time.Hour
	panelMaxTokens = 32
	panelListen = "127.0.0.1:0"
	panelEnabled = true
	panelTokens = map[string]time.Time{}
	panelSessions = map[string]time.Time{}

	srv := httptest.NewServer(panelRoutes())
	t.Cleanup(func() {
		srv.Close()
		panelAuthMode, panelTokenTTL, panelSessionTTL = old.mode, old.ttl, old.sessTTL
		panelListen, panelEnabled, panelMaxTokens = old.listen, old.enabled, old.maxToks
		panelTokens, panelSessions = old.tokens, old.sessions
	})
	return srv
}

func panelPost(t *testing.T, srv *httptest.Server, path, body string, cookie *http.Cookie) (*http.Response, map[string]interface{}) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("构造请求失败: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer res.Body.Close()
	var data map[string]interface{}
	raw, _ := io.ReadAll(res.Body)
	_ = json.Unmarshal(raw, &data)
	return res, data
}

func panelGet(t *testing.T, srv *httptest.Server, path string, cookie *http.Cookie) (*http.Response, []byte) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, srv.URL+path, nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	return res, raw
}

// TestPanelTokenFlow 一次性令牌换取会话的完整流程
func TestPanelTokenFlow(t *testing.T) {
	cleanupStats := newTestStatsDB(t)
	defer cleanupStats()
	srv := setupPanel(t, panelAuthToken)

	res, raw := panelGet(t, srv, "/api/session", nil)
	var sess map[string]interface{}
	_ = json.Unmarshal(raw, &sess)
	if res.StatusCode != 200 || sess["authed"] != false || sess["auth"] != panelAuthToken {
		t.Fatalf("未登录时 /api/session 应返回 authed=false，实际 %d %s", res.StatusCode, raw)
	}

	if res, _ := panelGet(t, srv, "/api/summary", nil); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("未登录访问 /api/summary 应 401，实际 %d", res.StatusCode)
	}
	if res, _ := panelGet(t, srv, "/api/chats", nil); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("未登录访问 /api/chats 应 401，实际 %d", res.StatusCode)
	}
	if res, _ := panelGet(t, srv, "/", nil); res.StatusCode != 200 {
		t.Errorf("首页应可访问（页面内部再判断登录），实际 %d", res.StatusCode)
	}
	if res, _ := panelGet(t, srv, "/api/health", nil); res.StatusCode != 200 {
		t.Errorf("/api/health 应可匿名访问，实际 %d", res.StatusCode)
	}

	token, ttl := panelIssueToken()
	if len(token) != 32 || ttl <= 0 {
		t.Fatalf("令牌格式异常: %q %v", token, ttl)
	}

	if res, _ := panelPost(t, srv, "/api/session", `{"token":"deadbeef"}`, nil); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("错误令牌应 401，实际 %d", res.StatusCode)
	}

	res, body := panelPost(t, srv, "/api/session", `{"token":"`+token+`"}`, nil)
	if res.StatusCode != 200 || body["ok"] != true {
		t.Fatalf("正确令牌应换取会话，实际 %d %v", res.StatusCode, body)
	}
	cookies := res.Cookies()
	if len(cookies) == 0 {
		t.Fatal("响应未下发会话 Cookie")
	}
	session := cookies[0]
	if !session.HttpOnly {
		t.Errorf("会话 Cookie 应设置 HttpOnly")
	}

	if res, raw := panelGet(t, srv, "/api/summary", session); res.StatusCode != 200 || !strings.Contains(string(raw), "\"summary\"") {
		t.Errorf("带会话访问 /api/summary 应成功，实际 %d %s", res.StatusCode, raw)
	}

	// 一次性：同一令牌不能再次换取会话
	if res, _ := panelPost(t, srv, "/api/session", `{"token":"`+token+`"}`, nil); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("一次性令牌重复使用应 401，实际 %d", res.StatusCode)
	}

	// 退出后原会话失效
	panelPost(t, srv, "/api/logout", `{}`, session)
	if res, _ := panelGet(t, srv, "/api/summary", session); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("退出后旧会话应 401，实际 %d", res.StatusCode)
	}
	token2, _ := panelIssueToken()
	if res, _ := panelPost(t, srv, "/api/session", `{"token":"`+token2+`"}`, nil); res.StatusCode != 200 {
		t.Errorf("退出后应可再次用新令牌登录，实际 %d", res.StatusCode)
	}
}

// TestPanelTokenExpiryAndCap 令牌过期与数量上限
func TestPanelTokenExpiryAndCap(t *testing.T) {
	srv := setupPanel(t, panelAuthToken)
	_ = srv

	panelTokenTTL = 60 * time.Millisecond
	expired, _ := panelIssueToken()
	time.Sleep(90 * time.Millisecond)
	if panelConsumeToken(expired) {
		t.Errorf("过期令牌不应通过校验")
	}
	if panelConsumeToken(expired) {
		t.Errorf("过期令牌被消费后也不应通过")
	}
	panelTokenTTL = 5 * time.Second
	fresh, _ := panelIssueToken()
	if !panelConsumeToken(fresh) {
		t.Errorf("新令牌应通过校验")
	}
	if panelConsumeToken("") {
		t.Errorf("空令牌不应通过")
	}

	panelMaxTokens = 4
	panelTokens = map[string]time.Time{}
	for i := 0; i < 10; i++ {
		panelIssueToken()
	}
	if len(panelTokens) > panelMaxTokens {
		t.Errorf("令牌数量应被限制在 %d 以内，实际 %d", panelMaxTokens, len(panelTokens))
	}
}

// TestPanelAuthNone 内网免密模式
func TestPanelAuthNone(t *testing.T) {
	cleanupStats := newTestStatsDB(t)
	defer cleanupStats()
	srv := setupPanel(t, panelAuthNone)

	res, raw := panelGet(t, srv, "/api/session", nil)
	var sess map[string]interface{}
	_ = json.Unmarshal(raw, &sess)
	if res.StatusCode != 200 || sess["authed"] != true || sess["auth"] != panelAuthNone {
		t.Fatalf("免密模式 /api/session 应返回 authed=true，实际 %d %s", res.StatusCode, raw)
	}
	if res, raw := panelGet(t, srv, "/api/summary", nil); res.StatusCode != 200 || !strings.Contains(string(raw), "\"summary\"") {
		t.Errorf("免密模式下应可直接读取数据，实际 %d %s", res.StatusCode, raw)
	}
}

// fakeBotAPI 起一个假的 Telegram Bot API，记录发出的消息
func fakeBotAPI(t *testing.T, sent *[]string) *tgbotapi.BotAPI {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/sendMessage"):
			*sent = append(*sent, string(raw))
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1,"date":0,"chat":{"id":1,"type":"private"},"text":"ok"}}`))
		default: // getMe 等
			_, _ = w.Write([]byte(`{"ok":true,"result":{"id":6599925531,"is_bot":true,"first_name":"stub","username":"stub_bot"}}`))
		}
	}))
	t.Cleanup(srv.Close)
	api, err := tgbotapi.NewBotAPIWithAPIEndpoint("6599925531:TESTTOKEN", srv.URL+"/bot%s/%s")
	if err != nil {
		t.Fatalf("构造测试 BotAPI 失败: %v", err)
	}
	return api
}

// firstBody 便于在断言失败时看清假 API 收到的报文
func firstBody(sent []string) string {
	if len(sent) == 0 {
		return "(空)"
	}
	b := sent[0]
	if len(b) > 400 {
		b = b[:400] + "..."
	}
	return b
}

// sentText 从假 API 收到的报文里取出 text 字段
// tgbotapi 会用 application/x-www-form-urlencoded 发送（中文是百分号转义），也可能用 JSON
func sentText(t *testing.T, body string) string {
	t.Helper()
	trimmed := strings.TrimSpace(body)
	if strings.HasPrefix(trimmed, "{") {
		var payload struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
			t.Fatalf("JSON 报文解析失败: %v", err)
		}
		return payload.Text
	}
	values, err := url.ParseQuery(trimmed)
	if err != nil {
		t.Fatalf("表单报文解析失败: %v", err)
	}
	return values.Get("text")
}

// TestPanelCommandAdminOnly /panel 指令只对管理员生效，且只在私聊回复
func TestPanelCommandAdminOnly(t *testing.T) {
	srv := setupPanel(t, panelAuthToken)
	_ = srv

	oldAdminStr := botAdminStr
	oldTokens := panelTokens
	botAdminStr = []string{"12345"}
	panelTokens = map[string]time.Time{}
	defer func() { botAdminStr, panelTokens = oldAdminStr, oldTokens }()

	sent := []string{}
	api := fakeBotAPI(t, &sent)

	// 管理员私聊：应返回包含令牌的消息
	panelTokens = map[string]time.Time{}
	if err := processPanelCommand(tgbotapi.Message{
		MessageID: 1,
		Chat:      &tgbotapi.Chat{ID: 12345, Type: "private"},
		From:      &tgbotapi.User{ID: 12345, UserName: "admin"},
	}, api); err != nil {
		t.Fatalf("管理员申请令牌出错: %v", err)
	}
	if len(sent) != 1 || len(panelTokens) != 1 {
		t.Fatalf("管理员应收到 1 条回复并生成 1 个令牌，实际 sent=%d body=%s tokens=%d", len(sent), firstBody(sent), len(panelTokens))
	}
	replyText := sentText(t, sent[0])
	if !strings.Contains(replyText, "令牌") {
		t.Errorf("管理员回复应包含「令牌」，实际: %s", replyText)
	}
	for tk := range panelTokens {
		if !strings.Contains(replyText, tk) {
			t.Errorf("回复里应包含令牌明文 %s，实际: %s", tk, replyText)
		}
	}

	// 非管理员私聊：应拒绝且不签发令牌
	panelTokens = map[string]time.Time{}
	sent = sent[:0]
	if err := processPanelCommand(tgbotapi.Message{
		MessageID: 2,
		Chat:      &tgbotapi.Chat{ID: 999, Type: "private"},
		From:      &tgbotapi.User{ID: 999, UserName: "someone"},
	}, api); err != nil {
		t.Fatalf("非管理员申请令牌出错: %v", err)
	}
	if len(sent) != 1 || len(panelTokens) != 0 {
		t.Fatalf("非管理员应收到 1 条拒绝回复且不签发令牌，实际 sent=%d body=%s tokens=%d", len(sent), firstBody(sent), len(panelTokens))
	}
	if permText := sentText(t, sent[0]); !strings.Contains(permText, "没有") || !strings.Contains(permText, "权限") {
		t.Errorf("非管理员回复应说明无权限，实际: %s", permText)
	}

	// 管理员在群里：静默不回复（避免令牌泄露）
	sent = sent[:0]
	if err := processPanelCommand(tgbotapi.Message{
		MessageID: 3,
		Chat:      &tgbotapi.Chat{ID: -100123, Type: "supergroup", Title: "group"},
		From:      &tgbotapi.User{ID: 12345, UserName: "admin"},
	}, api); err != nil {
		t.Fatalf("群内指令出错: %v", err)
	}
	if len(sent) != 0 {
		t.Errorf("群内不应回复令牌，实际发了 %d 条", len(sent))
	}
	if len(panelTokens) != 0 {
		t.Errorf("群内不应签发令牌")
	}
}
