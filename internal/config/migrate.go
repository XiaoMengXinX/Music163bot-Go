package config

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

// ParseINI 按行取第一个 "="，去首尾空白，跳过空键值（v2 语义）
func ParseINI(path string) (map[string]string, error) {
	ini := make(map[string]string)
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := bufio.NewReader(f)
	for {
		line, _, err := r.ReadLine()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		s := strings.TrimSpace(string(line))
		index := strings.Index(s, "=")
		if index < 0 {
			continue
		}
		key := strings.TrimSpace(s[:index])
		if key == "" {
			continue
		}
		value := strings.TrimSpace(s[index+1:])
		if value == "" {
			continue
		}
		ini[key] = value
	}
	return ini, nil
}

// MigrateINI v2 键值映射为 v3 配置，返回结果与提示列表
func MigrateINI(ini map[string]string) (*Config, []string, error) {
	cfg := Default()
	var notes []string
	get := func(key string) (string, bool) {
		v, ok := ini[key]
		return v, ok
	}
	if v, ok := get("BOT_TOKEN"); ok {
		cfg.Bot.Token = v
	}
	if v, ok := get("MUSIC_U"); ok {
		cfg.Netease.MUSICU = v
	}
	if v, ok := get("BotAPI"); ok && v != "" {
		cfg.Bot.APIURL = v
	}
	if v, ok := get("BotAdmin"); ok && v != "" {
		for _, s := range strings.Split(v, ",") {
			if id, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64); err == nil {
				cfg.Bot.Admin = append(cfg.Bot.Admin, id)
			} else {
				notes = append(notes, fmt.Sprintf("BotAdmin 中的 %q 不是合法数字，已跳过", s))
			}
		}
	}
	if v, ok := get("BotDebug"); ok && v == "true" {
		cfg.Bot.Debug = true
	}
	if v, ok := get("Database"); ok && v != "" {
		cfg.Database.Path = v
	}
	if v, ok := get("LogLevel"); ok && v != "" {
		cfg.Log.Level = v
	}
	if v, ok := get("DownloadTimeout"); ok && v != "" {
		seconds, err := strconv.Atoi(v)
		if err != nil || seconds <= 0 {
			notes = append(notes, fmt.Sprintf("DownloadTimeout %q 不合法，使用默认 60s", v))
		} else {
			cfg.Download.Timeout = Duration(time.Duration(seconds) * time.Second)
		}
	}
	if v, ok := get("MaxRetryTimes"); ok && v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			notes = append(notes, fmt.Sprintf("MaxRetryTimes %q 不合法，使用默认 3", v))
		} else {
			cfg.Download.Retries = n
		}
	}
	if v, ok := get("ReverseProxy"); ok {
		cfg.Download.ReverseProxy = v
	}
	if v, ok := get("AutoRetry"); ok && v == "false" {
		cfg.Download.AutoRetry = false
	}
	for _, dead := range []string{"AutoUpdate", "CheckMD5", "SrcPath", "BotEntry"} {
		if v, ok := get(dead); ok && v != "" {
			notes = append(notes, fmt.Sprintf("v2 配置项 %s 在 v3 中无实现，已删除", dead))
		}
	}
	return cfg, notes, nil
}

// MigrateINIFile 读取 INI 转换并写出 TOML
func MigrateINIFile(inPath, outPath string) (*Config, []string, error) {
	ini, err := ParseINI(inPath)
	if err != nil {
		return nil, nil, fmt.Errorf("读取 %s 失败: %w", inPath, err)
	}
	cfg, notes, err := MigrateINI(ini)
	if err != nil {
		return nil, nil, err
	}
	data, err := toml.Marshal(cfg)
	if err != nil {
		return nil, nil, err
	}
	header := fmt.Appendf(nil, "# 由 %s 迁移生成的 v3 配置文件\n", inPath)
	if err := os.WriteFile(outPath, append(header, data...), 0o644); err != nil {
		return nil, nil, err
	}
	return cfg, notes, nil
}
