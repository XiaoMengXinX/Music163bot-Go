package textx

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestParseMusicID(t *testing.T) {
	tests := []struct {
		name string
		text string
		want int
	}{
		{"完整歌曲链接", "https://music.163.com/song?id=1824020873&userid=1", 1824020873},
		{"路径式歌曲链接", "https://music.163.com/song/1824020873/", 1824020873},
		{"纯数字", "1824020873", 1824020873},
		{"分享文本带链接", "校正中分享了一首歌曲 https://music.163.com/song?id=1824020873", 1824020873},
		{"短链文本", "https://163cn.tv/AbCdEf", 0},
		{"专辑链接不算歌曲", "https://music.163.com/album?id=1", 0},
		{"空文本", "", 0},
		{"无数字文本", "hello world", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseMusicID(tt.text); got != tt.want {
				t.Errorf("ParseMusicID(%q) = %d, want %d", tt.text, got, tt.want)
			}
		})
	}
}

func TestParseProgramID(t *testing.T) {
	tests := []struct {
		name string
		text string
		want int
	}{
		{"节目链接", "https://music.163.com/program?id=2064195060", 2064195060},
		{"短链落点的节目链接（id 不在首位）", "https://y.music.163.com/m/program?app_version=9.5.05&id=3718588406&userid=1&djId=2", 3718588406},
		{"dj 链接", "https://music.163.com/dj?id=2064195060", 2064195060},
		{"路径式节目链接", "https://music.163.com/program/2064195060/", 2064195060},
		{"歌曲链接不算节目", "https://music.163.com/song?id=1824020873", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseProgramID(tt.text); got != tt.want {
				t.Errorf("ParseProgramID(%q) = %d, want %d", tt.text, got, tt.want)
			}
		})
	}
}

func TestURLBase(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://m801.music.126.net/abc/def.mp3?auth=1&x=2", "def.mp3"},
		{"https://p1.music.126.net/xxx/cover.jpg", "cover.jpg"},
		{"https://p1.music.126.net/xxx/cover.jpg#frag", "cover.jpg"},
	}
	for _, tt := range tests {
		if got := URLBase(tt.url); got != tt.want {
			t.Errorf("URLBase(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}

func TestSanitizeFilename(t *testing.T) {
	got := SanitizeFilename(`a/b\c*d:e?f"g<h>i|j`)
	want := "a b c d e f g h i j"
	if got != want {
		t.Errorf("SanitizeFilename = %q, want %q", got, want)
	}
}

func TestSanitizeFilenameLength(t *testing.T) {
	artists := strings.Repeat("影山ヒロノブ,ささきいさお,", 8)
	for _, tt := range []struct {
		name string
		ext  string
	}{
		{name: artists + " - 秘密戦隊ゴレンジャー [スーパー戦隊“魂”2004 LIVE].mp3", ext: ".mp3"},
		{name: "a" + artists + " - 愿世界和平.flac", ext: ".flac"},
		{name: "ab" + artists + " - 愿世界和平.lrc", ext: ".lrc"},
	} {
		got := SanitizeFilename(tt.name)
		if len(got) > 255 || !utf8.ValidString(got) || !strings.HasSuffix(got, tt.ext) {
			t.Errorf("SanitizeFilename(%q) = %q (%d bytes)", tt.name, got, len(got))
		}
		if base := strings.TrimSuffix(got, tt.ext); !strings.HasPrefix(tt.name, base) || len(got) < 252 {
			t.Errorf("SanitizeFilename(%q) = %q, want longest prefix", tt.name, got)
		}
	}
	exact := strings.Repeat("a", 250) + ".flac"
	if got := SanitizeFilename(exact); got != exact {
		t.Errorf("255-byte name changed: %q", got)
	}
}

func TestContainsMusicLink(t *testing.T) {
	if !ContainsMusicLink("看这个 https://music.163.com/song?id=1") {
		t.Error("music.163.com 应命中")
	}
	if !ContainsMusicLink("https://163cn.tv/xyz") {
		t.Error("163cn.tv 应命中")
	}
	if !ContainsMusicLink("https://163cn.link/xyz") {
		t.Error("163cn.link 应命中")
	}
	if ContainsMusicLink("普通消息") {
		t.Error("普通消息不应命中")
	}
}

func TestContainsSongLink(t *testing.T) {
	positive := []string{
		"分享一首歌 https://music.163.com/song?id=1824020873",
		"https://music.163.com/song/1824020873/",
		"https://music.163.com/program?id=2064195060",
		"https://music.163.com/dj?id=2064195060",
		"https://y.music.163.com/m/program?app_version=9.5.05&id=3718588406&userid=1&djId=2",
	}
	for _, s := range positive {
		if !ContainsSongLink(s) {
			t.Errorf("%q 应识别为歌曲分享", s)
		}
	}
	negative := []string{
		"music.163.com 挺好用的",
		"https://music.163.com/playlist?id=7016142538",
		"https://music.163.com/mv?id=123456",
		"https://music.163.com/",
		"普通消息",
	}
	for _, s := range negative {
		if ContainsSongLink(s) {
			t.Errorf("%q 不应识别为歌曲分享", s)
		}
	}
}

func TestResolveShortLink(t *testing.T) {
	if got := ResolveShortLink("https://music.163.com/song?id=1"); got != "https://music.163.com/song?id=1" {
		t.Errorf("非短链应原样返回, got %q", got)
	}
	if got := ResolveShortLink("see https://example.com/163cn.tv/foo"); got != "see https://example.com/163cn.tv/foo" {
		t.Errorf("非 163cn 短链应原样返回, got %q", got)
	}
}
