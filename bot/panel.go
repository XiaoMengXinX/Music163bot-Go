package bot

import (
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sirupsen/logrus"
)

//go:embed panel.html
var panelHTML []byte

// 面板认证方式
const (
	panelAuthToken = "token" // 默认：管理员私聊 bot 发 /panel 申请一次性令牌
	panelAuthNone  = "none"  // 内网免密（面板只应绑定在可信网络）
)

// 面板会话 Cookie 名
const panelCookieName = "music163bot_panel"

var (
	panelListen     string
	panelEnabled    bool
	panelAuthMode   = panelAuthToken
	panelPublicURL  string
	panelTokenTTL   = 5 * time.Minute
	panelSessionTTL = 12 * time.Hour
	panelMaxTokens  = 32

	panelMu       sync.Mutex
	panelTokens   = map[string]time.Time{} // 一次性令牌 -> 过期时间
	panelSessions = map[string]time.Time{} // 登录会话 -> 过期时间
)

// statsPanelStart 启动统计面板（由 bot.Start 调用）
func statsPanelStart(conf map[string]string) {
	if v, ok := conf["PanelEnable"]; ok && strings.EqualFold(strings.TrimSpace(v), "false") {
		logrus.Infoln("统计面板已按配置关闭 (PanelEnable = false)")
		return
	}
	panelEnabled = true

	panelListen = strings.TrimSpace(conf["PanelListen"])
	if panelListen == "" {
		panelListen = "0.0.0.0:8099"
	}
	switch v := strings.ToLower(strings.TrimSpace(conf["PanelAuth"])); v {
	case panelAuthToken, panelAuthNone:
		panelAuthMode = v
	case "":
		panelAuthMode = panelAuthToken
	default:
		logrus.Warnf("PanelAuth 只能取 token 或 none（当前 %q），已按 token 处理", v)
		panelAuthMode = panelAuthToken
	}
	if v := strings.TrimSpace(conf["PanelPublicURL"]); v != "" {
		panelPublicURL = strings.TrimRight(v, "/")
	}
	if v, err := strconv.Atoi(strings.TrimSpace(conf["PanelTokenTTL"])); err == nil && v > 0 {
		panelTokenTTL = time.Duration(v) * time.Second
	}

	mux := panelRoutes()

	// 首次启动且没有任何点播数据时，自动从 cache.db 回填历史
	go func() {
		var n int64
		if StatsDB != nil {
			StatsDB.Model(&PlayRecord{}).Count(&n)
		}
		if n == 0 {
			if res, err := backfillFromCache(config); err != nil {
				logrus.Errorln("历史回填失败:", err)
			} else {
				logrus.Infoln("历史回填:", res.Message)
			}
		}
	}()

	server := &http.Server{
		Addr:              panelListen,
		Handler:           panelRecover(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		defer func() {
			if e := recover(); e != nil {
				logrus.Errorln("统计面板异常退出:", e)
			}
		}()
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logrus.Errorln("统计面板启动失败:", err)
			return
		}
	}()

	if panelAuthMode == panelAuthNone {
		logrus.Warnf("统计面板已启用: http://%s/  (统计库 %s, 认证: 免密)", panelListen, statsDBFile)
		logrus.Warnln("PanelAuth = none 时任何人都能访问面板，请确保面板只监听在可信网络内")
	} else {
		logrus.Infof("统计面板已启用: http://%s/  (统计库 %s, 认证: 一次性令牌, 有效期 %v)", panelListen, statsDBFile, panelTokenTTL)
		logrus.Infoln("在 Telegram 里用 BotAdmin 账号私聊 bot 发送 /panel 即可获取访问令牌")
	}
}

// panelRoutes 注册所有面板路由（单独抽出便于测试）
func panelRoutes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/", panelIndex)
	mux.HandleFunc("/api/health", panelHealth)
	mux.HandleFunc("/api/session", panelSession)
	mux.HandleFunc("/api/logout", panelLogout)
	mux.HandleFunc("/api/summary", panelRequire(panelSummary))
	mux.HandleFunc("/api/weeks", panelRequire(panelWeeks))
	mux.HandleFunc("/api/top", panelRequire(panelTop))
	mux.HandleFunc("/api/chats", panelRequire(panelChats))
	mux.HandleFunc("/api/track", panelRequire(panelTrack))
	mux.HandleFunc("/api/tracks", panelRequire(panelTracks))
	mux.HandleFunc("/api/sync", panelRequire(panelSync))
	mux.HandleFunc("/api/sync/status", panelRequire(panelSyncStatus))
	mux.HandleFunc("/api/backfill", panelRequire(panelBackfill))
	return mux
}

// panelRecover 保证面板内部错误不会导致 bot 进程崩溃
func panelRecover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if e := recover(); e != nil {
				logrus.Errorln("面板处理请求时发生错误:", e)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("%v", e)})
			}
		}()
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// ---------- 认证 ----------

// panelIssueToken 生成一次性访问令牌（只应由 bot 管理员通过 /panel 触发）
func panelIssueToken() (token string, ttl time.Duration) {
	panelMu.Lock()
	defer panelMu.Unlock()
	now := time.Now()
	for t, exp := range panelTokens { // 清理过期的
		if now.After(exp) {
			delete(panelTokens, t)
		}
	}
	for len(panelTokens) >= panelMaxTokens { // 超量时淘汰最早到期的
		var oldest string
		var oldestExp time.Time
		for t, exp := range panelTokens {
			if oldest == "" || exp.Before(oldestExp) {
				oldest, oldestExp = t, exp
			}
		}
		if oldest == "" {
			break
		}
		delete(panelTokens, oldest)
	}
	token = randomHex(16)
	panelTokens[token] = now.Add(panelTokenTTL)
	return token, panelTokenTTL
}

// panelConsumeToken 校验并消费一次性令牌
func panelConsumeToken(token string) bool {
	token = strings.TrimSpace(token)
	if token == "" {
		return false
	}
	panelMu.Lock()
	defer panelMu.Unlock()
	exp, ok := panelTokens[token]
	if !ok {
		return false
	}
	delete(panelTokens, token) // 一次性：不管是否过期都作废
	return time.Now().Before(exp)
}

func panelNewSession() string {
	session := randomHex(32)
	panelMu.Lock()
	panelSessions[session] = time.Now().Add(panelSessionTTL)
	panelMu.Unlock()
	return session
}

// panelAuthed 判断请求是否已通过认证
func panelAuthed(r *http.Request) bool {
	if panelAuthMode == panelAuthNone {
		return true
	}
	c, err := r.Cookie(panelCookieName)
	if err != nil || c.Value == "" {
		return false
	}
	panelMu.Lock()
	defer panelMu.Unlock()
	exp, ok := panelSessions[c.Value]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(panelSessions, c.Value)
		return false
	}
	panelSessions[c.Value] = time.Now().Add(panelSessionTTL) // 滑动续期
	return true
}

// panelRequire 包裹需要认证的接口
func panelRequire(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !panelAuthed(r) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "未授权或登录已过期，请用 BotAdmin 账号私聊 bot 发送 /panel 获取一次性令牌"})
			return
		}
		next(w, r)
	}
}

// ---------- bot 侧：申请令牌 ----------

const (
	panelTokenText    = "统计面板一次性访问令牌（%d 秒内有效，仅可使用一次）:\n%s\n\n打开面板地址后粘贴该令牌即可；过期或已使用请重新发送 /panel。"
	panelTokenTextURL = "统计面板一次性访问令牌（%d 秒内有效，仅可使用一次）:\n%s\n\n一键登录: %s/?token=%s\n\n链接用过即失效，需要新链接请重新发送 /panel。"
	panelNoPerm       = "你没有申请面板访问令牌的权限（需要在 config.ini 的 BotAdmin 中）。"
	panelDisabledMsg  = "统计面板未启用（config.ini 里 PanelEnable = true 可开启）。"
)

// processPanelCommand 处理管理员的 /panel 指令：签发一次性访问令牌
func processPanelCommand(message tgbotapi.Message, bot *tgbotapi.BotAPI) (err error) {
	if message.From == nil || !message.Chat.IsPrivate() {
		return nil // 只在私聊里回应，避免令牌被其他人看到
	}
	if !panelEnabled {
		return sendPanelReply(bot, message, panelDisabledMsg)
	}
	if !in(fmt.Sprintf("%d", message.From.ID), botAdminStr) {
		logrus.Warnf("非管理员 %d 尝试申请面板令牌", message.From.ID)
		return sendPanelReply(bot, message, panelNoPerm)
	}
	token, ttl := panelIssueToken()
	text := fmt.Sprintf(panelTokenText, int(ttl.Seconds()), token)
	if panelPublicURL != "" {
		text = fmt.Sprintf(panelTokenTextURL, int(ttl.Seconds()), token, panelPublicURL, token)
	}
	logrus.Infof("已为管理员 %d 签发面板令牌（%v 内有效）", message.From.ID, ttl)
	return sendPanelReply(bot, message, text)
}

func sendPanelReply(bot *tgbotapi.BotAPI, message tgbotapi.Message, text string) error {
	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	msg.ReplyToMessageID = message.MessageID
	_, err := bot.Send(msg)
	return err
}

// ---------- 基础接口 ----------

func panelIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/index.html" {
		if r.URL.Path == "/favicon.ico" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(panelHTML)
}

func panelHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"panel":   "music163bot-stats",
		"auth":    panelAuthMode,
		"time":    time.Now().In(panelLoc).Format("2006-01-02 15:04:05"),
		"statsDB": statsDBFile,
	})
}

// panelSession GET 查询登录状态；POST 用一次性令牌换取会话 Cookie
func panelSession(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, map[string]interface{}{"authed": panelAuthed(r), "auth": panelAuthMode})
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "请使用 GET 或 POST"})
		return
	}
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求格式错误"})
		return
	}
	if !panelConsumeToken(body.Token) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "令牌无效、已使用或已过期，请重新发送 /panel 获取"})
		return
	}
	session := panelNewSession()
	http.SetCookie(w, &http.Cookie{
		Name:     panelCookieName,
		Value:    session,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(panelSessionTTL.Seconds()),
	})
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "ttl": int(panelSessionTTL.Seconds())})
}

func panelLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(panelCookieName); err == nil {
		panelMu.Lock()
		delete(panelSessions, c.Value)
		panelMu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: panelCookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---------- 数据接口 ----------

func panelSummary(w http.ResponseWriter, r *http.Request) {
	s, err := statsSummary()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	top, _ := statsTopTracks(s.WeekKey, 0, "all", 10)
	weeks, _ := statsWeeks(12)
	chats, _ := statsChats("group", false)
	recent := make([]ChatRow, 0, 10)
	for _, c := range chats {
		if c.LastPlay == "" {
			continue
		}
		recent = append(recent, c)
		if len(recent) >= 10 {
			break
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"summary":      s,
		"topThisWeek":  top,
		"weeks":        weeks,
		"recentGroups": recent,
	})
}

func panelWeeks(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 52
	}
	weeks, err := statsWeeks(limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"weeks": weeks, "current": currentWeekKey()})
}

func panelTop(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	week := strings.TrimSpace(q.Get("week"))
	scope := q.Get("scope")
	if scope == "" {
		scope = "all"
	}
	chatID, _ := strconv.ParseInt(q.Get("chatId"), 10, 64)
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	if week == "current" || week == "" {
		week = currentWeekKey()
	}
	if week == "all" {
		week = ""
	}
	rows, err := statsTopTracks(week, chatID, scope, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	rangeInfo := map[string]string{"week": week}
	if week != "" {
		if s, e, err := parseWeekKey(week); err == nil {
			rangeInfo["start"] = s.Format("2006-01-02")
			rangeInfo["end"] = e.AddDate(0, 0, -1).Format("2006-01-02")
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"range": rangeInfo, "rows": rows, "scope": scope, "chatId": chatID})
}

func panelChats(w http.ResponseWriter, r *http.Request) {
	only := r.URL.Query().Get("only")
	withPlays := r.URL.Query().Get("withPlays") == "1"
	rows, err := statsChats(only, withPlays)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"rows": rows, "total": len(rows)})
}

func panelTrack(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.URL.Query().Get("id"))
	if id == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "缺少 id 参数"})
		return
	}
	d, err := statsTrackDetail(id, 0)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func panelTracks(w http.ResponseWriter, r *http.Request) {
	kw := strings.TrimSpace(r.URL.Query().Get("q"))
	if kw == "" {
		rows, err := statsTopTracks("", 0, "all", 50)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"rows": rows})
		return
	}
	rows, err := statsSearchTracks(kw, 50)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"rows": rows})
}

func panelSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "请使用 POST"})
		return
	}
	if bot == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "bot 尚未就绪"})
		return
	}
	syncMu.Lock()
	running := syncRun
	syncMu.Unlock()
	if running {
		writeJSON(w, http.StatusOK, map[string]interface{}{"started": false, "message": "校验正在进行中"})
		return
	}
	go verifyChats(bot)
	writeJSON(w, http.StatusOK, map[string]interface{}{"started": true})
}

func panelSyncStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, syncStatus())
}

func panelBackfill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "请使用 POST"})
		return
	}
	res, err := backfillFromCache(config)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}
