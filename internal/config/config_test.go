package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

const validTOML = `
[bot]
token = "123456:ABC"
admin = [115414, 1919810]

[netease]
music_u = "COOKIE"

[download]
timeout = "90s"
retries = 5
`

func TestLoadDefaults(t *testing.T) {
	path := writeTemp(t, `
[bot]
token = "123456:ABC"

[netease]
music_u = "COOKIE"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Bot.APIURL != "https://api.telegram.org" {
		t.Errorf("api_url 默认值错误: %s", cfg.Bot.APIURL)
	}
	if cfg.Database.Path != "cache.db" {
		t.Errorf("database.path 默认值错误: %s", cfg.Database.Path)
	}
	if cfg.Download.Timeout.D() != 60*time.Second {
		t.Errorf("timeout 默认值错误: %v", cfg.Download.Timeout)
	}
	if cfg.Download.Retries != 3 {
		t.Errorf("retries 默认值错误: %d", cfg.Download.Retries)
	}
	if cfg.Download.Concurrency != 4 {
		t.Errorf("concurrency 默认值错误: %d", cfg.Download.Concurrency)
	}
	if !cfg.Download.AutoRetry {
		t.Error("auto_retry 默认应为 true")
	}
	if cfg.Log.Level != "info" || cfg.Log.Path != "./log" {
		t.Errorf("log 默认值错误: %+v", cfg.Log)
	}
	if cfg.Netease.Level != "hires" {
		t.Errorf("netease.level 默认值错误: %s", cfg.Netease.Level)
	}
}

func TestLoadValues(t *testing.T) {
	cfg, err := Load(writeTemp(t, validTOML))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Bot.Token != "123456:ABC" {
		t.Errorf("token = %s", cfg.Bot.Token)
	}
	if len(cfg.Bot.Admin) != 2 || cfg.Bot.Admin[0] != 115414 || cfg.Bot.Admin[1] != 1919810 {
		t.Errorf("admin = %v", cfg.Bot.Admin)
	}
	if cfg.Download.Timeout.D() != 90*time.Second {
		t.Errorf("timeout = %v", cfg.Download.Timeout)
	}
	if cfg.Download.Retries != 5 {
		t.Errorf("retries = %d", cfg.Download.Retries)
	}
}

func TestLoadUnknownFieldRejected(t *testing.T) {
	_, err := Load(writeTemp(t, validTOML+"\n[bot]\nbadkey = 1\n"))
	if err == nil {
		t.Fatal("拼错的配置项应被严格模式拒绝")
	}
}

func TestNeteaseLevel(t *testing.T) {
	cfg, err := Load(writeTemp(t, `[bot]
token = "123:ABC"

[netease]
music_u = "COOKIE"
level = "lossless"
`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Netease.Level != "lossless" {
		t.Errorf("level = %s, want lossless", cfg.Netease.Level)
	}
	if _, err := Load(writeTemp(t, `[bot]
token = "123:ABC"

[netease]
music_u = "COOKIE"
level = "sq"
`)); err == nil {
		t.Error("非法音质等级应被拒绝")
	}
}

func TestLoadMissingRequired(t *testing.T) {
	if _, err := Load(writeTemp(t, `[netease]
music_u = "x"
`)); err == nil {
		t.Error("缺少 token 应报错")
	}
	// music_u 允许留空：启动时会引导终端扫码登录
	if _, err := Load(writeTemp(t, `[bot]
token = "x"
`)); err != nil {
		t.Errorf("music_u 留空不应报错: %v", err)
	}
}

func TestEnvOverride(t *testing.T) {
	t.Setenv("MUSIC163BOT_BOT_TOKEN", "env-token")
	t.Setenv("MUSIC163BOT_BOT_ADMIN", "1, 2")
	t.Setenv("MUSIC163BOT_DOWNLOAD_TIMEOUT", "120s")
	cfg, err := Load(writeTemp(t, validTOML))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Bot.Token != "env-token" {
		t.Errorf("token 环境变量覆盖失败: %s", cfg.Bot.Token)
	}
	if len(cfg.Bot.Admin) != 2 || cfg.Bot.Admin[1] != 2 {
		t.Errorf("admin 环境变量覆盖失败: %v", cfg.Bot.Admin)
	}
	if cfg.Download.Timeout.D() != 120*time.Second {
		t.Errorf("timeout 环境变量覆盖失败: %v", cfg.Download.Timeout)
	}
}

func TestMigrateINI(t *testing.T) {
	ini := map[string]string{
		"BOT_TOKEN":       "111:AAA",
		"MUSIC_U":         "COOKIE",
		"BotAPI":          "https://custom.api",
		"BotAdmin":        "115414, 1919810",
		"BotDebug":        "true",
		"Database":        "data/cache.db",
		"LogLevel":        "debug",
		"DownloadTimeout": "90",
		"MaxRetryTimes":   "5",
		"ReverseProxy":    "1.2.3.4:443",
		"AutoRetry":       "false",
		"AutoUpdate":      "true",
		"CheckMD5":        "true",
		"SrcPath":         "./src",
		"BotEntry":        "bot.Start",
	}
	cfg, notes, err := MigrateINI(ini)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Bot.Token != "111:AAA" || cfg.Netease.MUSICU != "COOKIE" {
		t.Errorf("token/music_u 迁移错误: %+v", cfg)
	}
	if cfg.Bot.APIURL != "https://custom.api" {
		t.Errorf("api_url 迁移错误: %s", cfg.Bot.APIURL)
	}
	// "115414, 1919810" 带空格也应解析出两个管理员（v2 不会 trim，是 bug）
	if len(cfg.Bot.Admin) != 2 || cfg.Bot.Admin[0] != 115414 || cfg.Bot.Admin[1] != 1919810 {
		t.Errorf("admin 迁移错误: %v", cfg.Bot.Admin)
	}
	if !cfg.Bot.Debug {
		t.Error("debug 迁移错误")
	}
	if cfg.Database.Path != "data/cache.db" {
		t.Errorf("database.path 迁移错误: %s", cfg.Database.Path)
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("log.level 迁移错误: %s", cfg.Log.Level)
	}
	if cfg.Download.Timeout.D() != 90*time.Second {
		t.Errorf("timeout 迁移错误（应为 90s）: %v", cfg.Download.Timeout)
	}
	if cfg.Download.Retries != 5 {
		t.Errorf("retries 迁移错误: %d", cfg.Download.Retries)
	}
	if cfg.Download.ReverseProxy != "1.2.3.4:443" {
		t.Errorf("reverse_proxy 迁移错误: %s", cfg.Download.ReverseProxy)
	}
	if cfg.Download.AutoRetry {
		t.Error("auto_retry=false 迁移错误")
	}
	if len(notes) != 4 {
		t.Errorf("死配置应有 4 条提示, got %v", notes)
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("迁移结果未通过校验: %v", err)
	}
}

func TestMigrateINIFile(t *testing.T) {
	dir := t.TempDir()
	iniPath := filepath.Join(dir, "config.ini")
	outPath := filepath.Join(dir, "config.toml")
	ini := "BOT_TOKEN = 111:AAA\nMUSIC_U = COOKIE\nBotAdmin = 115414,1919810\nDownloadTimeout = 30\n"
	if err := os.WriteFile(iniPath, []byte(ini), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := MigrateINIFile(iniPath, outPath); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(outPath)
	if err != nil {
		t.Fatalf("迁移产物无法被 Load 解析: %v", err)
	}
	if cfg.Bot.Token != "111:AAA" || len(cfg.Bot.Admin) != 2 || cfg.Download.Timeout.D() != 30*time.Second {
		t.Errorf("迁移产物内容错误: %+v", cfg)
	}
}

func TestParseINI(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.ini")
	content := "# comment\n\nBOT_TOKEN=abc=def\n  MUSIC_U = cookie  \nEMPTY=\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	ini, err := ParseINI(path)
	if err != nil {
		t.Fatal(err)
	}
	if ini["BOT_TOKEN"] != "abc=def" {
		t.Errorf("值中含 = 应取第一个 = 之后全部: %q", ini["BOT_TOKEN"])
	}
	if ini["MUSIC_U"] != "cookie" {
		t.Errorf("应 trim 首尾空白: %q", ini["MUSIC_U"])
	}
	if _, ok := ini["EMPTY"]; ok {
		t.Error("空值行应跳过")
	}
}
