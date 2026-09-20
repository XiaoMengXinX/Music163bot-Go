package bot

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/glebarez/sqlite"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

// StatsDB 点播统计数据库入口（独立于歌曲缓存库 cache.db，执行 /rmcache 不会影响统计数据）
var StatsDB *gorm.DB

// statsDBFile 统计数据库文件名（用于面板展示）
var statsDBFile = "stats.db"

// panelLoc 统计所用时区，「周」的边界由它决定（默认东八区）
var panelLoc = time.FixedZone("CST", 8*3600)

const (
	sourceLive     = "live"     // 实时收到的点播
	sourceBackfill = "backfill" // 从 cache.db 历史缓存回填
	sourceInline   = "inline"   // inline 模式选用（拿不到群信息）
)

// ChatRecord 会话（群/私聊）记录
// bot 能收到某个群的消息，说明 bot 在该群内，因此以此判断「在哪些群」
type ChatRecord struct {
	ID          int64      `gorm:"primaryKey;autoIncrement:false"` // Telegram ChatID（群为负数）
	Type        string     `gorm:"size:16;index"`                  // private|group|supergroup|channel
	Title       string     // 群名（私聊为空）
	UserName    string     // @username
	MsgCount    int64      // 收到的消息条数
	MemberCount int        // 群成员数（主动校验时获取）
	InChat      bool       `gorm:"index"`   // 是否仍在群内
	Source      string     `gorm:"size:12"` // live|backfill
	FirstSeen   time.Time  // 首次见到
	LastSeen    time.Time  `gorm:"index"` // 最近一次见到
	VerifiedAt  *time.Time `gorm:"index"` // 最近一次确认在群状态的时间（NULL = 从未确认）
}

// PlayRecord 点播记录（一次「点歌并成功发出音频」＝一条记录）
type PlayRecord struct {
	ID        uint      `gorm:"primaryKey"`
	PlayedAt  time.Time `gorm:"index;uniqueIndex:idx_play_dedupe"`
	WeekKey   string    `gorm:"size:8;index"` // 2026-W38（按 panelLoc 计算的 ISO 周）
	ChatID    int64     `gorm:"index;uniqueIndex:idx_play_dedupe"`
	ChatTitle string
	ChatType  string `gorm:"size:16;index"`
	UserID    int64  `gorm:"index"`
	UserName  string
	MusicID   int `gorm:"index;uniqueIndex:idx_play_dedupe"`
	SongName  string
	Artists   string
	Album     string
	Source    string `gorm:"size:12;index;uniqueIndex:idx_play_dedupe"`
}

const (
	evChatSeen   = iota // 收到消息：更新标题/最后活跃/在群状态
	evChatStatus        // my_chat_member：更新在群状态
	evPlay              // 点播记录
	evFlush             // 等待队列排空（测试用）
)

type statsEvent struct {
	kind   int
	chat   *tgbotapi.Chat
	inChat bool
	play   *PlayRecord
	done   chan struct{}
}

var statsCh = make(chan statsEvent, 4096)
var statsWorkerOnce sync.Once

// initStats 初始化统计数据库并启动单写者协程
func initStats(conf map[string]string) (err error) {
	if conf["StatsDB"] != "" {
		statsDBFile = conf["StatsDB"]
	}
	if conf["StatsTimezone"] != "" {
		if loc, e := time.LoadLocation(conf["StatsTimezone"]); e == nil {
			panelLoc = loc
		} else {
			logrus.Warnf("无法加载时区 %v（%v），统计将使用 UTC+8", conf["StatsTimezone"], e)
		}
	}
	dsn := fmt.Sprintf("%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)", statsDBFile)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger:      NewLogger(logger.Silent),
		PrepareStmt: true,
	})
	if err != nil {
		return err
	}
	if err = db.AutoMigrate(&ChatRecord{}, &PlayRecord{}); err != nil {
		return err
	}
	// 旧版本把「未确认」写成零值时间而非 NULL，这里统一归一化（本项目的群记录都在 2023 年之后）
	if res := db.Model(&ChatRecord{}).Where("verified_at is not null and verified_at < ?", "2000-01-01").Update("verified_at", nil); res.Error != nil {
		logrus.Debugln("归一化 verified_at 失败:", res.Error)
	} else if res.RowsAffected > 0 {
		logrus.Infof("已将 %d 条未确认的会话记录归一化为 NULL", res.RowsAffected)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	sqlDB.SetMaxOpenConns(1)
	StatsDB = db

	statsWorkerOnce.Do(func() {
		go statsWorker()
		go statsCheckpointer()
	})
	logrus.Infof("统计数据库已就绪: %s (时区 %s)", statsDBFile, panelLoc.String())
	return nil
}

// statsCheckpointer 定期把 WAL 合并回主库，使 stats.db 可以单独备份/检查
func statsCheckpointer() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		checkpointStats()
	}
}

// checkpointStats 把 WAL 内容合并回主库（批量写入后调用，避免 stats.db 看起来是空的）
func checkpointStats() {
	if StatsDB == nil {
		return
	}
	if err := StatsDB.Exec("PRAGMA wal_checkpoint(PASSIVE)").Error; err != nil {
		logrus.Debugln("wal checkpoint 失败:", err)
	}
}

func statsWorker() {
	for ev := range statsCh {
		var err error
		switch ev.kind {
		case evChatSeen:
			err = applyChatSeen(ev.chat)
		case evChatStatus:
			err = applyChatStatus(ev.chat, ev.inChat)
		case evPlay:
			err = applyPlay(ev.play)
		case evFlush:
			if ev.done != nil {
				close(ev.done)
			}
		}
		if err != nil {
			logrus.Errorln("写入统计数据失败:", err)
		}
	}
}

// flushStats 等待队列里已入队的事件全部落库（测试与优雅退出用）
func flushStats(timeout time.Duration) bool {
	if StatsDB == nil {
		return false
	}
	done := make(chan struct{})
	select {
	case statsCh <- statsEvent{kind: evFlush, done: done}:
	case <-time.After(timeout):
		return false
	}
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

func chatTypeOf(chat *tgbotapi.Chat) string {
	if chat == nil || chat.Type == "" {
		return "unknown"
	}
	return chat.Type
}

// chatIDOf 取会话 ID（nil 安全）
func chatIDOf(chat *tgbotapi.Chat) int64 {
	if chat == nil {
		return 0
	}
	return chat.ID
}

func isGroupChat(t string) bool {
	return t == "group" || t == "supergroup" || t == "channel"
}

// recordChatSeen 收到消息时调用：消息本身即证明 bot 在该会话内
func recordChatSeen(chat *tgbotapi.Chat) {
	if StatsDB == nil || chat == nil {
		return
	}
	select {
	case statsCh <- statsEvent{kind: evChatSeen, chat: chat}:
	default:
	}
}

// recordChatStatus my_chat_member 事件：bot 被加入/移出群时调用
func recordChatStatus(chat *tgbotapi.Chat, inChat bool) {
	if StatsDB == nil || chat == nil {
		return
	}
	select {
	case statsCh <- statsEvent{kind: evChatStatus, chat: chat, inChat: inChat}:
	default:
	}
}

// recordPlay 记录一次点播（按统计时区计算周）
func recordPlay(song SongInfo, message tgbotapi.Message, source string) {
	if StatsDB == nil || song.MusicID == 0 {
		return
	}
	now := time.Now().UTC()
	rec := &PlayRecord{
		PlayedAt:  now,
		WeekKey:   weekKeyOf(now),
		ChatID:    chatIDOf(message.Chat),
		ChatTitle: chatTitleOf(message.Chat),
		ChatType:  chatTypeOf(message.Chat),
		MusicID:   song.MusicID,
		SongName:  song.SongName,
		Artists:   song.SongArtists,
		Album:     song.SongAlbum,
		Source:    source,
	}
	if message.From != nil {
		rec.UserID = message.From.ID
		rec.UserName = message.From.UserName
		if rec.UserName == "" {
			rec.UserName = message.From.FirstName
		}
	}
	select {
	case statsCh <- statsEvent{kind: evPlay, play: rec}:
	default:
	}
}

// recordInlinePlay inline 模式下用户选用了某首歌：拿不到所在群，只能计入曲目总量
func recordInlinePlay(song SongInfo, user *tgbotapi.User) {
	if StatsDB == nil || song.MusicID == 0 || user == nil {
		return
	}
	now := time.Now().UTC()
	userName := user.UserName
	if userName == "" {
		userName = user.FirstName
	}
	rec := &PlayRecord{
		PlayedAt:  now,
		WeekKey:   weekKeyOf(now),
		ChatID:    0,
		ChatTitle: "（inline 选用，未知会话）",
		ChatType:  "inline",
		UserID:    user.ID,
		UserName:  userName,
		MusicID:   song.MusicID,
		SongName:  song.SongName,
		Artists:   song.SongArtists,
		Album:     song.SongAlbum,
		Source:    sourceInline,
	}
	select {
	case statsCh <- statsEvent{kind: evPlay, play: rec}:
	default:
	}
}

// recordChosenInline inline 结果被选用时调用：用 file_id 反查曲目（群信息拿不到）
func recordChosenInline(fileID string, user *tgbotapi.User) {
	if StatsDB == nil || MusicDB == nil || fileID == "" || user == nil {
		return
	}
	var song SongInfo
	err := MusicDB.Session(&gorm.Session{}).Where("file_id = ?", fileID).First(&song).Error
	if err != nil {
		return
	}
	recordInlinePlay(song, user)
}

func chatTitleOf(chat *tgbotapi.Chat) string {
	if chat == nil {
		return ""
	}
	if chat.Title != "" {
		return chat.Title
	}
	if chat.UserName != "" {
		return chat.UserName
	}
	if chat.FirstName != "" {
		return chat.FirstName
	}
	return fmt.Sprintf("%d", chat.ID)
}

func applyChatSeen(c *tgbotapi.Chat) error {
	if c == nil {
		return nil
	}
	now := time.Now().UTC()
	updates := map[string]interface{}{
		"type":        chatTypeOf(c),
		"last_seen":   now,
		"verified_at": &now,
		"msg_count":   gorm.Expr("msg_count + 1"),
	}
	if c.Title != "" {
		updates["title"] = c.Title
	}
	if c.UserName != "" {
		updates["user_name"] = c.UserName
	}
	if isGroupChat(chatTypeOf(c)) {
		// 能收到群消息 => bot 在该群内
		updates["in_chat"] = true
	}
	res := StatsDB.Model(&ChatRecord{}).Where("id = ?", c.ID).Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		rec := ChatRecord{
			ID:         c.ID,
			Type:       chatTypeOf(c),
			Title:      c.Title,
			UserName:   c.UserName,
			MsgCount:   1,
			InChat:     isGroupChat(chatTypeOf(c)),
			Source:     sourceLive,
			FirstSeen:  now,
			LastSeen:   now,
			VerifiedAt: &now,
		}
		return StatsDB.Clauses(clause.OnConflict{DoNothing: true}).Create(&rec).Error
	}
	return nil
}

func applyChatStatus(c *tgbotapi.Chat, inChat bool) error {
	if c == nil {
		return nil
	}
	now := time.Now().UTC()
	updates := map[string]interface{}{
		"in_chat":     inChat,
		"type":        chatTypeOf(c),
		"verified_at": &now,
		"last_seen":   now,
	}
	if c.Title != "" {
		updates["title"] = c.Title
	}
	if c.UserName != "" {
		updates["user_name"] = c.UserName
	}
	res := StatsDB.Model(&ChatRecord{}).Where("id = ?", c.ID).Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		rec := ChatRecord{
			ID: c.ID, Type: chatTypeOf(c), Title: c.Title, UserName: c.UserName,
			InChat: inChat, Source: sourceLive, FirstSeen: now, LastSeen: now, VerifiedAt: &now,
		}
		return StatsDB.Clauses(clause.OnConflict{DoNothing: true}).Create(&rec).Error
	}
	return nil
}

func applyPlay(p *PlayRecord) error {
	if p == nil || p.MusicID == 0 {
		return nil
	}
	return StatsDB.Clauses(clause.OnConflict{DoNothing: true}).Create(p).Error
}

// ---------- 周计算 ----------

// weekKeyOf 返回 t 所在 ISO 周，形如 2026-W38（按 panelLoc 计算）
func weekKeyOf(t time.Time) string {
	y, w := t.In(panelLoc).ISOWeek()
	return fmt.Sprintf("%04d-W%02d", y, w)
}

// isoWeekStart 返回某 ISO 年第 week 周的周一 00:00（panelLoc）
func isoWeekStart(year, week int) time.Time {
	jan4 := time.Date(year, 1, 4, 0, 0, 0, 0, panelLoc)
	wd := int(jan4.Weekday())
	if wd == 0 {
		wd = 7 // 周日算第 7 天
	}
	return jan4.AddDate(0, 0, -(wd-1)+(week-1)*7)
}

// parseWeekKey 解析 2026-W38
func parseWeekKey(key string) (start, end time.Time, err error) {
	key = strings.TrimSpace(strings.ToUpper(key))
	var year, week int
	if _, err = fmt.Sscanf(key, "%d-W%d", &year, &week); err != nil {
		return start, end, fmt.Errorf("周格式应为 2026-W38")
	}
	if week < 1 || week > 53 || year < 2000 {
		return start, end, errors.New("周超出范围")
	}
	start = isoWeekStart(year, week)
	end = start.AddDate(0, 0, 7)
	return start, end, nil
}

// currentWeekKey 当前周
func currentWeekKey() string { return weekKeyOf(time.Now()) }

// parseDBTime 兼容驱动返回的时间文本（如 2026-09-19 15:04:05.123456789+00:00）
func parseDBTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	layouts := []string{
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05-07:00",
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// fmtLocal 把数据库里的时间文本转成统计时区的展示格式
func fmtLocal(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if t, ok := parseDBTime(s); ok {
		return t.In(panelLoc).Format("2006-01-02 15:04")
	}
	// 兼容被存成 unix 时间戳（秒/毫秒/纳秒）的情况
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		switch {
		case n > 1e18:
			return time.Unix(0, n).In(panelLoc).Format("2006-01-02 15:04")
		case n > 1e12:
			return time.UnixMilli(n).In(panelLoc).Format("2006-01-02 15:04")
		case n > 1e9:
			return time.Unix(n, 0).In(panelLoc).Format("2006-01-02 15:04")
		}
	}
	if len(s) >= 16 {
		return s[:16]
	}
	return s
}

// ---------- 查询（供面板使用）----------

// Summary 概览数据
type Summary struct {
	Groups        int64    `json:"groups"`        // 见过的群总数
	GroupsIn      int64    `json:"groupsIn"`      // 确认仍在群
	GroupsLeft    int64    `json:"groupsLeft"`    // 确认已退出
	GroupsUnknown int64    `json:"groupsUnknown"` // 状态未知（仅历史回填）
	MembersSum    int64    `json:"membersSum"`    // 已获取到的群人数之和
	Privates      int64    `json:"privates"`      // 私聊会话数
	PlaysTotal    int64    `json:"playsTotal"`
	PlaysWeek     int64    `json:"playsWeek"`
	PlaysInline   int64    `json:"playsInline"`
	PlaysBackfill int64    `json:"playsBackfill"`
	SongsTotal    int64    `json:"songsTotal"` // 曲目去重数
	WeekKey       string   `json:"weekKey"`
	WeekStart     string   `json:"weekStart"`
	WeekEnd       string   `json:"weekEnd"`
	DayLabels     []string `json:"dayLabels"`
	DayPlays      []int64  `json:"dayPlays"`
	LastPlay      string   `json:"lastPlay"`
	DBFile        string   `json:"dbFile"`
	DBBytes       int64    `json:"dbBytes"`
	StatsTimeZone string   `json:"statsTimeZone"`
	Now           string   `json:"now"`
}

func statsSummary() (Summary, error) {
	var s Summary
	s.StatsTimeZone = panelLoc.String()
	s.DBFile = statsDBFile
	if fi, err := os.Stat(statsDBFile); err == nil {
		s.DBBytes = fi.Size()
		if w, err := os.Stat(statsDBFile + "-wal"); err == nil { // WAL 里可能还有未合并的数据
			s.DBBytes += w.Size()
		}
	}
	const groupExpr = "type in ('group','supergroup','channel')"
	StatsDB.Model(&ChatRecord{}).Where(groupExpr).Count(&s.Groups)
	StatsDB.Model(&ChatRecord{}).Where(groupExpr + " and verified_at is not null and in_chat = 1").Count(&s.GroupsIn)
	StatsDB.Model(&ChatRecord{}).Where(groupExpr + " and verified_at is not null and in_chat = 0").Count(&s.GroupsLeft)
	StatsDB.Model(&ChatRecord{}).Where(groupExpr + " and verified_at is null").Count(&s.GroupsUnknown)
	StatsDB.Model(&ChatRecord{}).Where("type = 'private'").Count(&s.Privates)

	if row := StatsDB.Model(&ChatRecord{}).Select("coalesce(sum(member_count),0)").
		Where(groupExpr + " and verified_at is not null and in_chat = 1").Row(); row != nil {
		_ = row.Scan(&s.MembersSum)
	}
	StatsDB.Model(&PlayRecord{}).Count(&s.PlaysTotal)
	StatsDB.Model(&PlayRecord{}).Where("source = ?", sourceInline).Count(&s.PlaysInline)
	StatsDB.Model(&PlayRecord{}).Where("source = ?", sourceBackfill).Count(&s.PlaysBackfill)
	if row := StatsDB.Model(&PlayRecord{}).Select("count(distinct music_id)").Row(); row != nil {
		_ = row.Scan(&s.SongsTotal)
	}

	s.WeekKey = currentWeekKey()
	start, end, _ := parseWeekKey(s.WeekKey)
	s.WeekStart = start.Format("2006-01-02")
	s.WeekEnd = end.AddDate(0, 0, -1).Format("2006-01-02")
	StatsDB.Model(&PlayRecord{}).Where("week_key = ?", s.WeekKey).Count(&s.PlaysWeek)

	// 本周每天的点播数（用类型化的时间字段分桶，避免驱动返回格式差异）
	weekPlays := make([]PlayRecord, 0)
	StatsDB.Select("played_at").Where("week_key = ?", s.WeekKey).Find(&weekPlays)
	counts := map[string]int64{}
	for _, p := range weekPlays {
		if p.PlayedAt.IsZero() {
			continue
		}
		counts[p.PlayedAt.In(panelLoc).Format("2006-01-02")]++
	}
	for i := 0; i < 7; i++ {
		d := start.AddDate(0, 0, i)
		s.DayLabels = append(s.DayLabels, d.Format("01-02"))
		s.DayPlays = append(s.DayPlays, counts[d.Format("2006-01-02")])
	}

	var lastRec PlayRecord
	if err := StatsDB.Order("played_at desc").Limit(1).First(&lastRec).Error; err == nil && !lastRec.PlayedAt.IsZero() {
		s.LastPlay = lastRec.PlayedAt.In(panelLoc).Format("2006-01-02 15:04")
	}
	s.Now = time.Now().In(panelLoc).Format("2006-01-02 15:04:05")
	return s, nil
}

// WeekRow 每周点播数
type WeekRow struct {
	WeekKey   string `json:"week"`
	Plays     int64  `json:"plays"`
	WeekStart string `json:"weekStart"`
	WeekEnd   string `json:"weekEnd"`
	IsCurrent bool   `json:"isCurrent"`
}

func statsWeeks(limit int) ([]WeekRow, error) {
	rows := make([]WeekRow, 0)
	err := StatsDB.Model(&PlayRecord{}).
		Select("week_key, count(*) as plays").
		Group("week_key").Order("week_key desc").Limit(limit).Scan(&rows).Error
	if err != nil {
		return rows, err
	}
	cur := currentWeekKey()
	for i := range rows {
		rows[i].IsCurrent = rows[i].WeekKey == cur
		if s, e, err := parseWeekKey(rows[i].WeekKey); err == nil {
			rows[i].WeekStart = s.Format("2006-01-02")
			rows[i].WeekEnd = e.AddDate(0, 0, -1).Format("2006-01-02")
		}
	}
	return rows, nil
}

// TrackRow 曲目排行行
type TrackRow struct {
	Rank       int    `json:"rank"`
	MusicID    int    `json:"musicId"`
	SongName   string `json:"songName"`
	Artists    string `json:"artists"`
	Album      string `json:"album"`
	Plays      int64  `json:"plays"`
	Chats      int64  `json:"chats"`
	Users      int64  `json:"users"`
	LastPlayed string `json:"lastPlayed"`
}

// statsTopTracks scope: all|group|private|inline；chatID 非 0 时按会话过滤
func statsTopTracks(weekKey string, chatID int64, scope string, limit int) ([]TrackRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 20
	}
	q := StatsDB.Model(&PlayRecord{}).Select(
		"music_id, max(song_name) as song_name, max(artists) as artists, max(album) as album, count(*) as plays, count(distinct chat_id) as chats, count(distinct user_id) as users, max(played_at) as last_played")
	if weekKey != "" {
		q = q.Where("week_key = ?", weekKey)
	}
	if chatID != 0 {
		q = q.Where("chat_id = ?", chatID)
	} else {
		switch scope {
		case "group":
			q = q.Where("chat_type in ('group','supergroup','channel')")
		case "private":
			q = q.Where("chat_type = 'private'")
		case "inline":
			q = q.Where("chat_type = 'inline'")
		}
	}
	rows := make([]TrackRow, 0)
	if err := q.Group("music_id").Order("plays desc, last_played desc").Limit(limit).Scan(&rows).Error; err != nil {
		return rows, err
	}
	for i := range rows {
		rows[i].Rank = i + 1
		rows[i].LastPlayed = fmtLocal(rows[i].LastPlayed)
	}
	return rows, nil
}

// ChatRow 群/会话行
type ChatRow struct {
	ID          int64  `json:"id"`
	Type        string `json:"type"`
	Title       string `json:"title"`
	UserName    string `json:"userName"`
	MsgCount    int64  `json:"msgCount"`
	MemberCount int    `json:"memberCount"`
	InChat      bool   `json:"inChat"`
	Verified    bool   `json:"verified"`
	FirstSeen   string `json:"firstSeen"`
	LastSeen    string `json:"lastSeen"`
	VerifiedAt  string `json:"verifiedAt"`
	Plays       int64  `json:"plays"`
	PlaysWeek   int64  `json:"playsWeek"`
	LastPlay    string `json:"lastPlay"`
	Source      string `json:"source"`
}

func statsChats(only string, withPlays bool) ([]ChatRow, error) {
	q := StatsDB.Model(&ChatRecord{})
	switch only {
	case "group":
		q = q.Where("type in ('group','supergroup','channel')")
	case "private":
		q = q.Where("type = 'private'")
	}
	recs := make([]ChatRecord, 0)
	if err := q.Order("last_seen desc").Find(&recs).Error; err != nil {
		return nil, err
	}
	type agg struct {
		ChatID    int64
		Plays     int64
		PlaysWeek int64
		LastPlay  string
	}
	aggs := make([]agg, 0)
	StatsDB.Model(&PlayRecord{}).
		Select("chat_id, count(*) as plays, sum(case when week_key = ? then 1 else 0 end) as plays_week, max(played_at) as last_play", currentWeekKey()).
		Group("chat_id").Scan(&aggs)
	byChat := map[int64]agg{}
	for _, a := range aggs {
		byChat[a.ChatID] = a
	}
	rows := make([]ChatRow, 0, len(recs))
	for _, r := range recs {
		row := ChatRow{
			ID: r.ID, Type: r.Type, Title: r.Title, UserName: r.UserName,
			MsgCount: r.MsgCount, MemberCount: r.MemberCount, InChat: r.InChat,
			Verified: r.VerifiedAt != nil, Source: r.Source,
		}
		if !r.FirstSeen.IsZero() {
			row.FirstSeen = r.FirstSeen.In(panelLoc).Format("2006-01-02 15:04")
		}
		if !r.LastSeen.IsZero() {
			row.LastSeen = r.LastSeen.In(panelLoc).Format("2006-01-02 15:04")
		}
		if r.VerifiedAt != nil {
			row.VerifiedAt = r.VerifiedAt.In(panelLoc).Format("2006-01-02 15:04")
		}
		if a, ok := byChat[r.ID]; ok {
			row.Plays = a.Plays
			row.PlaysWeek = a.PlaysWeek
			row.LastPlay = fmtLocal(a.LastPlay)
		}
		if withPlays && row.Plays == 0 {
			continue
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// NamePlayRow 会话/用户的点播数
type NamePlayRow struct {
	ID    int64  `json:"chatId"`
	Name  string `json:"chatTitle"`
	Type  string `json:"chatType"`
	Plays int64  `json:"plays"`
}

// TrackDetail 单曲详情
type TrackDetail struct {
	MusicID    int           `json:"musicId"`
	SongName   string        `json:"songName"`
	Artists    string        `json:"artists"`
	Album      string        `json:"album"`
	PlaysTotal int64         `json:"playsTotal"`
	LastPlayed string        `json:"lastPlayed"`
	Weeks      []WeekRow     `json:"weeks"`
	Chats      []NamePlayRow `json:"chats"`
	Users      []NamePlayRow `json:"users"`
}

func statsTrackDetail(musicID int, limit int) (TrackDetail, error) {
	var d TrackDetail
	d.MusicID = musicID
	if limit <= 0 {
		limit = 30
	}
	type head struct {
		SongName   string
		Artists    string
		Album      string
		PlaysTotal int64
		LastPlayed string
	}
	h := head{}
	if err := StatsDB.Model(&PlayRecord{}).Select(
		"max(song_name) as song_name, max(artists) as artists, max(album) as album, count(*) as plays_total, max(played_at) as last_played").
		Where("music_id = ?", musicID).Scan(&h).Error; err != nil {
		return d, err
	}
	d.SongName, d.Artists, d.Album, d.PlaysTotal = h.SongName, h.Artists, h.Album, h.PlaysTotal
	d.LastPlayed = fmtLocal(h.LastPlayed)

	weeks := make([]WeekRow, 0)
	StatsDB.Model(&PlayRecord{}).Select("week_key, count(*) as plays").
		Where("music_id = ?", musicID).Group("week_key").Order("week_key desc").Limit(26).Scan(&weeks)
	cur := currentWeekKey()
	for i := range weeks {
		weeks[i].IsCurrent = weeks[i].WeekKey == cur
		if s, e, err := parseWeekKey(weeks[i].WeekKey); err == nil {
			weeks[i].WeekStart = s.Format("2006-01-02")
			weeks[i].WeekEnd = e.AddDate(0, 0, -1).Format("2006-01-02")
		}
	}
	d.Weeks = weeks

	chats := make([]NamePlayRow, 0)
	StatsDB.Model(&PlayRecord{}).
		Select("chat_id as id, max(chat_title) as name, max(chat_type) as type, count(*) as plays").
		Where("music_id = ?", musicID).Group("chat_id").Order("plays desc").Limit(limit).Scan(&chats)
	d.Chats = chats

	users := make([]NamePlayRow, 0)
	StatsDB.Model(&PlayRecord{}).
		Select("user_id as id, max(user_name) as name, count(*) as plays").
		Where("music_id = ?", musicID).Group("user_id").Order("plays desc").Limit(limit).Scan(&users)
	d.Users = users
	return d, nil
}

// SearchTrackRow 曲目搜索结果
type SearchTrackRow struct {
	MusicID    int    `json:"musicId"`
	SongName   string `json:"songName"`
	Artists    string `json:"artists"`
	Plays      int64  `json:"plays"`
	LastPlayed string `json:"lastPlayed"`
}

func statsSearchTracks(keyword string, limit int) ([]SearchTrackRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows := make([]SearchTrackRow, 0)
	q := StatsDB.Model(&PlayRecord{}).
		Select("music_id, max(song_name) as song_name, max(artists) as artists, count(*) as plays, max(played_at) as last_played").
		Group("music_id").Order("plays desc").Limit(limit)
	if id, err := strconv.Atoi(strings.TrimSpace(keyword)); err == nil && id != 0 {
		q = q.Where("music_id = ?", id)
	} else {
		kw := "%" + keyword + "%"
		q = q.Where("song_name like ? or artists like ?", kw, kw)
	}
	if err := q.Scan(&rows).Error; err != nil {
		return rows, err
	}
	for i := range rows {
		rows[i].LastPlayed = fmtLocal(rows[i].LastPlayed)
	}
	return rows, nil
}

// ---------- 群状态校验 ----------

// SyncResult 群状态校验结果
type SyncResult struct {
	Running   bool     `json:"running"`
	Total     int      `json:"total"`
	Done      int      `json:"done"`
	InChat    int      `json:"inChat"`
	Left      int      `json:"left"`
	StartedAt string   `json:"startedAt"`
	EndedAt   string   `json:"endedAt"`
	Errors    []string `json:"errors"`
}

var (
	syncMu    sync.Mutex
	syncState = SyncResult{Errors: []string{}}
	syncRun   bool
)

// verifyChats 主动向 Telegram 查询每个群的当前信息（标题/类型/人数/是否仍在群）
func verifyChats(api *tgbotapi.BotAPI) {
	syncMu.Lock()
	if syncRun {
		syncMu.Unlock()
		return
	}
	syncRun = true
	syncState = SyncResult{Running: true, Errors: []string{}, StartedAt: time.Now().In(panelLoc).Format("2006-01-02 15:04:05")}
	recs := make([]ChatRecord, 0)
	StatsDB.Where("type in ('group','supergroup','channel')").Order("last_seen desc").Find(&recs)
	syncState.Total = len(recs)
	syncMu.Unlock()

	logrus.Infof("开始校验 %d 个会话的在群状态", len(recs))
	for i := range recs {
		rec := recs[i]
		inChat, memberCount, title, ctype, err := probeChat(api, rec.ID)
		syncMu.Lock()
		if err != nil {
			syncState.Left++ // getChat 失败通常意味着 bot 已不在该群
			if len(syncState.Errors) < 20 {
				syncState.Errors = append(syncState.Errors, fmt.Sprintf("%d(%s): %v", rec.ID, rec.Title, err))
			}
		} else if inChat {
			syncState.InChat++
		} else {
			syncState.Left++
		}
		syncMu.Unlock()

		now := time.Now().UTC()
		updates := map[string]interface{}{
			"in_chat":      inChat,
			"verified_at":  &now,
			"member_count": memberCount,
		}
		if title != "" {
			updates["title"] = title
		}
		if ctype != "" {
			updates["type"] = ctype
		}
		if err := StatsDB.Model(&ChatRecord{}).Where("id = ?", rec.ID).Updates(updates).Error; err != nil {
			logrus.Errorln(err)
		}
		syncMu.Lock()
		syncState.Done++
		syncMu.Unlock()
		time.Sleep(350 * time.Millisecond) // 控制请求速率，避免触发限流
	}

	syncMu.Lock()
	if syncState.Errors == nil {
		syncState.Errors = []string{}
	}
	syncState.Running = false
	syncState.EndedAt = time.Now().In(panelLoc).Format("2006-01-02 15:04:05")
	syncRun = false
	result := syncState
	syncMu.Unlock()
	logrus.Infof("会话校验完成: 共 %d, 在群 %d, 已退出 %d", result.Total, result.InChat, result.Left)
}

func probeChat(api *tgbotapi.BotAPI, chatID int64) (inChat bool, memberCount int, title, ctype string, err error) {
	cfg := tgbotapi.ChatInfoConfig{ChatConfig: tgbotapi.ChatConfig{ChatID: chatID}}
	chat, err := api.GetChat(cfg)
	if err != nil {
		return false, 0, "", "", err
	}
	title = chat.Title
	ctype = chat.Type
	inChat = true
	if cnt, e := api.GetChatMembersCount(tgbotapi.ChatMemberCountConfig{ChatConfig: tgbotapi.ChatConfig{ChatID: chatID}}); e == nil {
		memberCount = cnt
	}
	return inChat, memberCount, title, ctype, nil
}

func syncStatus() SyncResult {
	syncMu.Lock()
	defer syncMu.Unlock()
	r := syncState
	if r.Errors == nil {
		r.Errors = []string{}
	}
	return r
}

// ---------- 历史回填 ----------

// BackfillResult 回填结果
type BackfillResult struct {
	Songs   int    `json:"songs"`
	Chats   int    `json:"chats"`
	Message string `json:"message"`
}

// backfillFromCache 从歌曲缓存库 cache.db 导入历史记录（仅作补充：缓存库只保留每首歌首次缓存的时间）
func backfillFromCache(conf map[string]string) (res BackfillResult, err error) {
	cacheFile := "cache.db"
	if conf["Database"] != "" {
		cacheFile = conf["Database"]
	}
	if _, err = os.Stat(cacheFile); err != nil {
		return res, fmt.Errorf("找不到缓存库 %s: %v", cacheFile, err)
	}
	dsn := fmt.Sprintf("file:%s?mode=ro&_pragma=busy_timeout(5000)", cacheFile)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: NewLogger(logger.Silent)})
	if err != nil {
		return res, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return res, err
	}
	defer sqlDB.Close()

	songs := make([]SongInfo, 0)
	if err = db.Table("song_infos").Order("created_at asc").Find(&songs).Error; err != nil {
		return res, err
	}
	if len(songs) == 0 {
		res.Message = "缓存库里没有历史记录"
		return res, nil
	}
	// 回填可重复执行：先清掉上次回填的数据，再写入
	if err = StatsDB.Where("source = ?", sourceBackfill).Delete(&PlayRecord{}).Error; err != nil {
		return res, err
	}

	chatSeen := map[int64][2]time.Time{} // chatID -> [首次, 最近]
	chatMeta := map[int64]SongInfo{}
	plays := make([]PlayRecord, 0, len(songs))
	for _, s := range songs {
		if s.FromChatID == 0 {
			continue
		}
		t := s.CreatedAt.UTC()
		ctype := "private"
		if s.FromChatID < 0 {
			ctype = "supergroup"
		}
		plays = append(plays, PlayRecord{
			PlayedAt:  t,
			WeekKey:   weekKeyOf(t),
			ChatID:    s.FromChatID,
			ChatTitle: s.FromChatName,
			ChatType:  ctype,
			UserID:    s.FromUserID,
			UserName:  s.FromUserName,
			MusicID:   s.MusicID,
			SongName:  s.SongName,
			Artists:   s.SongArtists,
			Album:     s.SongAlbum,
			Source:    sourceBackfill,
		})
		if v, ok := chatSeen[s.FromChatID]; ok {
			if t.Before(v[0]) {
				v[0] = t
			}
			if t.After(v[1]) {
				v[1] = t
			}
			chatSeen[s.FromChatID] = v
		} else {
			chatSeen[s.FromChatID] = [2]time.Time{t, t}
			chatMeta[s.FromChatID] = s
		}
	}
	if len(plays) > 0 {
		if err = StatsDB.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(plays, 200).Error; err != nil {
			return res, err
		}
	}
	// 补全会话表（in_chat / verified_at 保持未校验状态）
	for chatID, span := range chatSeen {
		s := chatMeta[chatID]
		ctype := "private"
		if chatID < 0 {
			ctype = "supergroup"
		}
		var exist ChatRecord
		e := StatsDB.Where("id = ?", chatID).First(&exist).Error
		if e == nil {
			continue
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return res, e
		}
		rec := ChatRecord{
			ID: chatID, Type: ctype, Title: s.FromChatName, Source: sourceBackfill,
			FirstSeen: span[0], LastSeen: span[1],
		}
		if err = StatsDB.Clauses(clause.OnConflict{DoNothing: true}).Create(&rec).Error; err != nil {
			return res, err
		}
	}
	res.Songs = len(plays)
	res.Chats = len(chatSeen)
	checkpointStats() // 批量写入后合并 WAL
	res.Message = fmt.Sprintf("已从缓存库回填 %d 条历史点播记录，涉及 %d 个会话（缓存库只保留每首歌首次缓存的时间，重复点播无法还原）", len(plays), len(chatSeen))
	return res, nil
}
