// Package config 负责 v3 TOML 配置的解析、默认值、环境变量覆盖与校验。
package config

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
	"github.com/sirupsen/logrus"
)

// Duration 时长类型，接受 "60s" 这类格式
type Duration time.Duration

func (d Duration) D() time.Duration { return time.Duration(d) }

func (d *Duration) UnmarshalText(b []byte) error {
	v, err := time.ParseDuration(string(b))
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", string(b), err)
	}
	*d = Duration(v)
	return nil
}

func (d Duration) MarshalText() ([]byte, error) { return []byte(time.Duration(d).String()), nil }

// BotConfig 对应 [bot] 段
type BotConfig struct {
	Token         string   `toml:"token"`
	APIURL        string   `toml:"api_url"`
	Admin         []int64  `toml:"admin"`
	Debug         bool     `toml:"debug"`
	LocalMode     bool     `toml:"local_mode"`
	UploadTimeout Duration `toml:"upload_timeout"`
}

// NeteaseConfig 对应 [netease] 段
type NeteaseConfig struct {
	MUSICU string `toml:"music_u"`
	Level  string `toml:"level"`
	Proxy  string `toml:"proxy"`
}

func (c NeteaseConfig) proxyURL() (*url.URL, error) {
	if c.Proxy == "" {
		return nil, nil
	}
	invalid := fmt.Errorf("配置项 [netease] proxy 必须为 http/https/socks5/socks5h 代理 URL，主机名不能为空，端口须为 1..65535，不允许路径、query 或 fragment")
	u, err := url.Parse(c.Proxy)
	if err != nil {
		return nil, invalid
	}
	switch u.Scheme {
	case "http", "https", "socks5", "socks5h":
	default:
		return nil, invalid
	}
	if u.Hostname() == "" || u.Opaque != "" ||
		(u.EscapedPath() != "" && u.EscapedPath() != "/") ||
		u.ForceQuery || u.RawQuery != "" || strings.Contains(c.Proxy, "#") ||
		strings.HasSuffix(u.Host, ":") ||
		(strings.Contains(u.Hostname(), ":") && !strings.HasPrefix(u.Host, "[")) {
		return nil, invalid
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return nil, invalid
		}
	}
	return u, nil
}

// HTTPClient 使用已校验的配置创建客户端，空代理保留默认传输行为。
func (c NeteaseConfig) HTTPClient() (*http.Client, error) {
	client := &http.Client{}
	if c.Proxy == "" {
		return client, nil
	}
	u, err := url.Parse(c.Proxy)
	if err != nil {
		return nil, fmt.Errorf("配置项 [netease] proxy 不是合法代理 URL")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyURL(u)
	client.Transport = transport
	return client, nil
}

// DatabaseConfig 对应 [database] 段
type DatabaseConfig struct {
	Path string `toml:"path"`
}

// DownloadConfig 对应 [download] 段
type DownloadConfig struct {
	Timeout      Duration `toml:"timeout"`
	Retries      int      `toml:"retries"`
	Concurrency  int      `toml:"concurrency"`
	ReverseProxy string   `toml:"reverse_proxy"`
	AutoRetry    bool     `toml:"auto_retry"`
}

// LogConfig 对应 [log] 段
type LogConfig struct {
	Level string `toml:"level"`
	Path  string `toml:"path"`
}

// Config 是 v3 的完整配置
type Config struct {
	Bot      BotConfig      `toml:"bot"`
	Netease  NeteaseConfig  `toml:"netease"`
	Database DatabaseConfig `toml:"database"`
	Download DownloadConfig `toml:"download"`
	Log      LogConfig      `toml:"log"`
}

// Default 返回填好默认值的配置
func Default() *Config {
	return &Config{
		Bot: BotConfig{
			APIURL:        "https://api.telegram.org",
			UploadTimeout: Duration(120 * time.Second),
		},
		Netease: NeteaseConfig{
			// 与 Music163Api-Go 默认一致
			Level: "hires",
		},
		Database: DatabaseConfig{
			Path: "cache.db",
		},
		Download: DownloadConfig{
			Timeout:     Duration(60 * time.Second),
			Retries:     3,
			Concurrency: 4,
			AutoRetry:   true,
		},
		Log: LogConfig{
			Level: "info",
			Path:  "./log",
		},
	}
}

// Load 读取 TOML：默认值打底，严格模式拒绝未知字段，应用环境变量覆盖
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg := Default()
	dec := toml.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(cfg); err != nil {
		return nil, fmt.Errorf("解析 %s 失败: %w", path, err)
	}
	if err := applyEnv(cfg); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Validate 校验取值范围；music_u 可为空，启动时引导扫码登录
func (c *Config) Validate() error {
	if c.Bot.Token == "" {
		return fmt.Errorf("配置项 [bot] token 不能为空")
	}
	if _, err := url.Parse(c.Bot.APIURL); err != nil || c.Bot.APIURL == "" {
		return fmt.Errorf("配置项 [bot] api_url %q 不是合法 URL", c.Bot.APIURL)
	}
	if c.Bot.UploadTimeout <= 0 {
		return fmt.Errorf("配置项 [bot] upload_timeout 必须为正时长")
	}
	switch c.Netease.Level {
	case "standard", "higher", "lossless", "hires", "jyeffect", "sky", "jymaster":
	default:
		return fmt.Errorf("配置项 [netease] level %q 不合法（可选 standard/higher/lossless/hires/jyeffect/sky/jymaster）", c.Netease.Level)
	}
	if _, err := c.Netease.proxyURL(); err != nil {
		return err
	}
	for _, id := range c.Bot.Admin {
		if id <= 0 {
			return fmt.Errorf("配置项 [bot] admin 包含非法 ID %d", id)
		}
	}
	if c.Database.Path == "" {
		return fmt.Errorf("配置项 [database] path 不能为空")
	}
	if c.Download.Timeout <= 0 {
		return fmt.Errorf("配置项 [download] timeout 必须为正时长")
	}
	if c.Download.Retries < 0 {
		return fmt.Errorf("配置项 [download] retries 不能为负数")
	}
	if c.Download.Concurrency <= 0 {
		return fmt.Errorf("配置项 [download] concurrency 必须为正整数")
	}
	if _, err := logrus.ParseLevel(c.Log.Level); err != nil {
		return fmt.Errorf("配置项 [log] level %q 不合法", c.Log.Level)
	}
	if c.Log.Path == "" {
		return fmt.Errorf("配置项 [log] path 不能为空")
	}
	return nil
}

const envPrefix = "MUSIC163BOT_"

// applyEnv 用 MUSIC163BOT_<段>_<键> 环境变量覆盖配置项
func applyEnv(cfg *Config) error {
	return walkStruct(reflect.ValueOf(cfg).Elem(), "", func(v reflect.Value, envName string) error {
		s, ok := os.LookupEnv(envName)
		if !ok {
			return nil
		}
		return setFromString(v, s, envName)
	})
}

func walkStruct(v reflect.Value, path string, fn func(reflect.Value, string) error) error {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		name := f.Tag.Get("toml")
		if name == "" {
			continue
		}
		envName := envPrefix + strings.ToUpper(path+strings.ReplaceAll(name, "-", "_"))
		fv := v.Field(i)
		if f.Anonymous || f.Type.Kind() == reflect.Struct && f.Type != reflect.TypeOf(Duration(0)) {
			if err := walkStruct(fv, strings.ToUpper(strings.ReplaceAll(name, "-", "_"))+"_", fn); err != nil {
				return err
			}
			continue
		}
		if err := fn(fv, envName); err != nil {
			return err
		}
	}
	return nil
}

func setFromString(v reflect.Value, s, name string) error {
	if v.Type() == reflect.TypeOf(Duration(0)) {
		var d Duration
		if err := d.UnmarshalText([]byte(s)); err != nil {
			return fmt.Errorf("环境变量 %s=%q 不是合法时长", name, s)
		}
		v.Set(reflect.ValueOf(d))
		return nil
	}
	switch v.Kind() {
	case reflect.String:
		v.SetString(s)
	case reflect.Bool:
		b, err := strconv.ParseBool(s)
		if err != nil {
			return fmt.Errorf("环境变量 %s=%q 不是合法 bool", name, s)
		}
		v.SetBool(b)
	case reflect.Int, reflect.Int64:
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return fmt.Errorf("环境变量 %s=%q 无法解析为数字", name, s)
		}
		v.SetInt(n)
	case reflect.Slice:
		if v.Type().Elem().Kind() != reflect.Int64 {
			return fmt.Errorf("环境变量 %s 不支持的类型 %s", name, v.Type())
		}
		var ids []int64
		for _, part := range strings.Split(s, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			id, err := strconv.ParseInt(part, 10, 64)
			if err != nil {
				return fmt.Errorf("环境变量 %s=%q 包含非法数字 %q", name, s, part)
			}
			ids = append(ids, id)
		}
		v.Set(reflect.ValueOf(ids))
	default:
		return fmt.Errorf("环境变量 %s 不支持的类型 %s", name, v.Kind())
	}
	return nil
}
