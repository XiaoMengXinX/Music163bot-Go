package bot

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// newTestStatsDB 用临时文件初始化统计库，返回清理函数
func newTestStatsDB(t *testing.T) func() {
	t.Helper()
	dir := t.TempDir()
	oldFile, oldLoc := statsDBFile, panelLoc
	statsDBFile = filepath.Join(dir, "stats.db")
	panelLoc = time.FixedZone("CST", 8*3600)
	if err := initStats(map[string]string{"StatsDB": statsDBFile}); err != nil {
		t.Fatalf("initStats 失败: %v", err)
	}
	return func() {
		if StatsDB != nil {
			if db, err := StatsDB.DB(); err == nil {
				_ = db.Close()
			}
		}
		StatsDB = nil
		statsDBFile, panelLoc = oldFile, oldLoc
	}
}

// drainStats 等待写入协程把队列处理完
func drainStats(t *testing.T) {
	t.Helper()
	if !flushStats(5 * time.Second) {
		t.Fatal("等待统计队列排空超时")
	}
}

// TestLiveWritePath 覆盖 bot 运行时真正走的写入路径（入队 -> 单写者落库）
func TestLiveWritePath(t *testing.T) {
	cleanup := newTestStatsDB(t)
	defer cleanup()

	group := tgbotapi.Chat{ID: -1001234567890, Type: "supergroup", Title: "测试群"}
	priv := tgbotapi.Chat{ID: 123456, Type: "private", UserName: "someone"}
	recordChatSeen(&group)
	recordChatSeen(&group)
	recordChatSeen(&priv)

	msg := tgbotapi.Message{
		MessageID: 7,
		Chat:      &group,
		From:      &tgbotapi.User{ID: 42, UserName: "tester"},
	}
	recordPlay(SongInfo{MusicID: 999, SongName: "夜曲", SongArtists: "周杰伦", SongAlbum: "十一月的萧邦"}, msg, sourceLive)
	recordInlinePlay(SongInfo{MusicID: 888, SongName: "稻香", SongArtists: "周杰伦"}, &tgbotapi.User{ID: 43, UserName: "inline_user"})
	recordChatStatus(&group, false)

	drainStats(t)

	var g ChatRecord
	if err := StatsDB.Where("id = ?", group.ID).First(&g).Error; err != nil {
		t.Fatalf("群记录未写入: %v", err)
	}
	if g.Type != "supergroup" || g.Title != "测试群" {
		t.Errorf("群记录字段错误: %+v", g)
	}
	if g.MsgCount != 2 {
		t.Errorf("消息计数应为 2，实际 %d", g.MsgCount)
	}
	if g.InChat {
		t.Errorf("my_chat_member 已把 bot 标记为离开，InChat 应为 false")
	}
	if g.VerifiedAt == nil {
		t.Errorf("收到消息后 VerifiedAt 不应为 NULL")
	}

	var p PlayRecord
	if err := StatsDB.Where("music_id = ?", 999).First(&p).Error; err != nil {
		t.Fatalf("点播记录未写入: %v", err)
	}
	if p.ChatID != group.ID || p.ChatType != "supergroup" || p.ChatTitle != "测试群" {
		t.Errorf("点播记录会话信息错误: %+v", p)
	}
	if p.UserID != 42 || p.UserName != "tester" {
		t.Errorf("点播记录用户信息错误: %+v", p)
	}
	if p.WeekKey != weekKeyOf(p.PlayedAt) || p.SongName != "夜曲" {
		t.Errorf("点播记录周/曲目错误: %+v", p)
	}

	var ip PlayRecord
	if err := StatsDB.Where("music_id = ?", 888).First(&ip).Error; err != nil {
		t.Fatalf("inline 点播记录未写入: %v", err)
	}
	if ip.ChatType != "inline" || ip.Source != sourceInline {
		t.Errorf("inline 记录标记错误: %+v", ip)
	}

	// 重复点播应计为多次（而不是被唯一索引吞掉）
	recordPlay(SongInfo{MusicID: 999, SongName: "夜曲", SongArtists: "周杰伦"}, msg, sourceLive)
	drainStats(t)
	var n int64
	StatsDB.Model(&PlayRecord{}).Where("music_id = ?", 999).Count(&n)
	if n != 2 {
		t.Errorf("同曲两次点播应记录 2 条，实际 %d", n)
	}

	sum, err := statsSummary()
	if err != nil {
		t.Fatalf("statsSummary 失败: %v", err)
	}
	if sum.Groups != 1 || sum.Privates != 1 || sum.GroupsLeft != 1 || sum.GroupsIn != 0 {
		t.Errorf("概览统计错误: %+v", sum)
	}
	if sum.PlaysTotal != 3 || sum.PlaysInline != 1 || sum.SongsTotal != 2 {
		t.Errorf("点播统计错误: %+v", sum)
	}
	if sum.PlaysWeek != 3 || sum.WeekKey != currentWeekKey() {
		t.Errorf("本周统计错误: %+v", sum)
	}
	today := time.Now().In(panelLoc).Format("01-02")
	idx := -1
	for i, l := range sum.DayLabels {
		if l == today {
			idx = i
		}
	}
	if idx < 0 || len(sum.DayPlays) != 7 {
		t.Errorf("每日分布标签异常: %+v %+v", sum.DayLabels, sum.DayPlays)
	} else if sum.DayPlays[idx] != 3 {
		t.Errorf("今天（%s）的点播数应为 3，实际 %d（分布 %+v）", today, sum.DayPlays[idx], sum.DayPlays)
	}

	top, err := statsTopTracks(currentWeekKey(), 0, "group", 10)
	if err != nil {
		t.Fatalf("statsTopTracks 失败: %v", err)
	}
	if len(top) != 1 || top[0].MusicID != 999 || top[0].Plays != 2 || top[0].Rank != 1 {
		t.Errorf("周榜结果错误: %+v", top)
	}

	detail, err := statsTrackDetail(999, 10)
	if err != nil {
		t.Fatalf("statsTrackDetail 失败: %v", err)
	}
	if detail.PlaysTotal != 2 || len(detail.Chats) != 1 || detail.Chats[0].ID != group.ID {
		t.Errorf("曲目详情错误: %+v", detail)
	}
}

// TestWeekKeyAndRange 检验周边界（含跨年那一周）
func TestWeekKeyAndRange(t *testing.T) {
	old := panelLoc
	panelLoc = time.FixedZone("CST", 8*3600)
	defer func() { panelLoc = old }()

	cases := []struct {
		when time.Time
		want string
	}{
		{time.Date(2026, 9, 19, 12, 0, 0, 0, panelLoc), "2026-W38"},
		{time.Date(2026, 9, 14, 0, 30, 0, 0, panelLoc), "2026-W38"},  // 周一凌晨
		{time.Date(2026, 9, 13, 23, 30, 0, 0, panelLoc), "2026-W37"}, // 周日晚
		{time.Date(2025, 12, 29, 10, 0, 0, 0, panelLoc), "2026-W01"}, // 跨年周
		{time.Date(2026, 1, 4, 23, 0, 0, 0, panelLoc), "2026-W01"},
		{time.Date(2026, 1, 5, 0, 1, 0, 0, panelLoc), "2026-W02"},
	}
	for _, c := range cases {
		if got := weekKeyOf(c.when); got != c.want {
			t.Errorf("%v 应属于 %s，实际 %s", c.when.Format("2006-01-02 15:04"), c.want, got)
		}
	}

	// 时区换算：UTC 周日晚 23:30 在东八区已是周一，应归入下一周
	utc := time.Date(2026, 9, 13, 23, 30, 0, 0, time.UTC)
	if got := weekKeyOf(utc); got != "2026-W38" {
		t.Errorf("UTC 2026-09-13 23:30 在东八区应属 2026-W38，实际 %s", got)
	}

	start, end, err := parseWeekKey("2026-W38")
	if err != nil {
		t.Fatalf("parseWeekKey 失败: %v", err)
	}
	if start.Format("2006-01-02 15:04") != "2026-09-14 00:00" || end.Format("2006-01-02") != "2026-09-21" {
		t.Errorf("周范围错误: %s ~ %s", start.Format("2006-01-02 15:04"), end.Format("2006-01-02 15:04"))
	}
	if _, _, err := parseWeekKey("2026-W99"); err == nil {
		t.Errorf("非法周应报错")
	}
	if got := fmtLocal("2026-09-19 06:04:05.401870588+00:00"); got != "2026-09-19 14:04" {
		t.Errorf("时间换算错误: %s", got)
	}
}

// TestBackfillIdempotent 回填可重复执行且不重复计数
func TestBackfillIdempotent(t *testing.T) {
	cleanup := newTestStatsDB(t)
	defer cleanup()

	dir := t.TempDir()
	cacheFile := filepath.Join(dir, "cache.db")
	db, err := gorm.Open(sqlite.Open(cacheFile), &gorm.Config{Logger: NewLogger(logger.Silent)})
	if err != nil {
		t.Fatalf("打开测试缓存库失败: %v", err)
	}
	if err := db.Table("song_infos").AutoMigrate(&SongInfo{}); err != nil {
		t.Fatalf("建表失败: %v", err)
	}
	base := time.Date(2026, 9, 14, 4, 0, 0, 0, time.UTC)
	seeds := []SongInfo{
		{MusicID: 1, SongName: "A", SongArtists: "x", FromChatID: -100, FromChatName: "群一", FromUserID: 11, FromUserName: "u1"},
		{MusicID: 2, SongName: "B", SongArtists: "y", FromChatID: -100, FromChatName: "群一", FromUserID: 12, FromUserName: "u2"},
		{MusicID: 3, SongName: "C", SongArtists: "z", FromChatID: 55, FromChatName: "someone", FromUserID: 13, FromUserName: "u3"},
	}
	// CreatedAt/UpdatedAt 是内嵌 gorm.Model 的提升字段，不能在复合字面量里赋值
	for i := range seeds {
		seeds[i].CreatedAt = base.Add(time.Duration(i) * 24 * time.Hour)
		seeds[i].UpdatedAt = seeds[i].CreatedAt
	}
	if err := db.Table("song_infos").Create(&seeds).Error; err != nil {
		t.Fatalf("写入测试数据失败: %v", err)
	}
	sqlDB, _ := db.DB()
	_ = sqlDB.Close()

	conf := map[string]string{"Database": cacheFile}
	res1, err := backfillFromCache(conf)
	if err != nil {
		t.Fatalf("回填失败: %v", err)
	}
	if res1.Songs != 3 || res1.Chats != 2 {
		t.Errorf("首次回填结果错误: %+v", res1)
	}
	res2, err := backfillFromCache(conf)
	if err != nil {
		t.Fatalf("二次回填失败: %v", err)
	}
	if res2.Songs != 3 {
		t.Errorf("二次回填结果错误: %+v", res2)
	}
	var plays, chats int64
	StatsDB.Model(&PlayRecord{}).Count(&plays)
	StatsDB.Model(&ChatRecord{}).Count(&chats)
	if plays != 3 || chats != 2 {
		t.Errorf("回填幂等失败: plays=%d chats=%d", plays, chats)
	}

	// 回填的群应为「未校验」状态
	rows, err := statsChats("group", false)
	if err != nil {
		t.Fatalf("statsChats 失败: %v", err)
	}
	if len(rows) != 1 || rows[0].Verified {
		t.Errorf("回填的群不应带校验状态: %+v", rows)
	}
	if rows[0].Plays != 2 || rows[0].FirstSeen == "" || rows[0].LastPlay == "" {
		t.Errorf("群聚合错误: %+v", rows[0])
	}
	// 未校验的群必须计入「未校验」，不能计成「已退出」（曾经因为零值时间不是 NULL 而误报）
	sum, err := statsSummary()
	if err != nil {
		t.Fatalf("statsSummary 失败: %v", err)
	}
	if sum.GroupsUnknown != 1 || sum.GroupsLeft != 0 || sum.GroupsIn != 0 {
		t.Errorf("回填的群应计入未校验: groupsUnknown=%d groupsLeft=%d groupsIn=%d", sum.GroupsUnknown, sum.GroupsLeft, sum.GroupsIn)
	}
	var rec ChatRecord
	if err := StatsDB.Where("id = ?", -100).First(&rec).Error; err != nil {
		t.Fatalf("读会话失败: %v", err)
	}
	if rec.VerifiedAt != nil {
		t.Errorf("回填的会话不应带 verified_at，实际 %v", rec.VerifiedAt)
	}
	_ = os.Remove(cacheFile)
}

// TestChatTitleFallback 无标题的会话名回退
func TestChatTitleFallback(t *testing.T) {
	if got := chatTitleOf(&tgbotapi.Chat{ID: -5, Type: "supergroup"}); got != "-5" {
		t.Errorf("无标题群应回退为 ID，实际 %s", got)
	}
	if got := chatTitleOf(&tgbotapi.Chat{ID: 5, Type: "private", UserName: "abc"}); got != "abc" {
		t.Errorf("私聊应使用 username，实际 %s", got)
	}
	if got := chatTitleOf(nil); got != "" {
		t.Errorf("nil 会话应返回空串，实际 %s", got)
	}
	if got := chatTypeOf(nil); got != "unknown" {
		t.Errorf("nil 会话类型应为 unknown，实际 %s", got)
	}
	if isGroupChat("private") || !isGroupChat("supergroup") {
		t.Errorf("isGroupChat 判断错误")
	}
}
