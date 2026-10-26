package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestUpdateMUSICU(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	original := `# 我的配置注释
[bot]
token = "111:AAA" # token 注释
admin = [1, 2]

[netease]
# music_u 注释
music_u = 'old-cookie' # 行尾注释

[download]
timeout = "60s"
`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := UpdateMUSICU(path, "new-cookie<>&"); err != nil {
		t.Fatal(err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := `# 我的配置注释
[bot]
token = "111:AAA" # token 注释
admin = [1, 2]

[netease]
# music_u 注释
music_u = "new-cookie<>&" # 行尾注释

[download]
timeout = "60s"
`
	if string(after) != want {
		t.Errorf("回写结果不符合预期:\n%s", after)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Netease.MUSICU != "new-cookie<>&" {
		t.Errorf("music_u = %q", cfg.Netease.MUSICU)
	}
	if cfg.Bot.Token != "111:AAA" || cfg.Download.Timeout.D() != 60*time.Second {
		t.Errorf("其它配置被误改: %+v", cfg)
	}

	// 同时覆盖单引号和双引号赋值
	if err := UpdateMUSICU(path, "third"); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Netease.MUSICU != "third" {
		t.Errorf("二次更新失败: %q", cfg.Netease.MUSICU)
	}
}

func TestUpdateMUSICUMissingKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("[bot]\ntoken = \"x\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpdateMUSICU(path, "y"); err == nil {
		t.Error("缺少 music_u 应报错")
	}
}
