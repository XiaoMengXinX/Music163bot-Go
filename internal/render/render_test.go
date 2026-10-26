package render

import (
	"strings"
	"testing"
	"unicode/utf16"
	"unicode/utf8"
)

func TestEscapeInTemplates(t *testing.T) {
	caption := MusicCaption("<b>歌名&</b>", "歌手", "专辑", "mp3", 1.5, 320, "Music163bot")
	if strings.Contains(caption, "<b>歌名") {
		t.Errorf("caption 未转义: %s", caption)
	}
	if !strings.Contains(caption, "&lt;b&gt;歌名&amp;&lt;/b&gt;") {
		t.Errorf("caption 转义结果错误: %s", caption)
	}

	info := SongInfo("雨き声残響", "ゆめこ & <歌手>", "最新热歌慢摇92")
	if info != "「雨き声残響」- ゆめこ &amp; &lt;歌手&gt;\n专辑: 最新热歌慢摇92" {
		t.Errorf("歌曲信息格式错误: %s", info)
	}
	if got := RecognizeResult("雨き声残響", "ゆめこ & <歌手>", "最新热歌慢摇92"); got != "🎵 识别成功\n「雨き声残響」- ゆめこ &amp; &lt;歌手&gt;\n专辑: 最新热歌慢摇92" {
		t.Errorf("识曲结果格式错误: %s", got)
	}
	if caption != "「&lt;b&gt;歌名&amp;&lt;/b&gt;」- 歌手\n专辑: 专辑\n#网易云音乐 #mp3 1.50MB 320.00kbps\nvia @Music163bot" {
		t.Errorf("caption 格式错误: %s", caption)
	}

	status := MusicStatus("a<b", "c&d", "mp3", 1.0, Downloading)
	if !strings.Contains(status, "a&lt;b") || !strings.Contains(status, "c&amp;d") {
		t.Errorf("状态消息未转义: %s", status)
	}

	about := About("go1.22", "v3<a", "sha", "2024", "darwin/arm64")
	if strings.Contains(about, "v3<a") {
		t.Errorf("about 未转义: %s", about)
	}

	info = StatusInfo("群<名>", "group_link", 1, 2, 3, 4, 5)
	if strings.Contains(info, "群<名>") {
		t.Errorf("status 未转义: %s", info)
	}
	if !strings.Contains(info, `href="https://t.me/group_link"`) {
		t.Errorf("群聊应生成 t.me 链接: %s", info)
	}
}

func TestStatusInfoChatVariants(t *testing.T) {
	private := StatusInfo("", "alice", 42, 42, 1, 1, 1)
	if !strings.Contains(private, `tg://user?id=42`) {
		t.Errorf("私聊应生成 tg://user 链接: %s", private)
	}
	plain := StatusInfo("群名", "", 1, 2, 3, 4, 5)
	if !strings.Contains(plain, "[群名]") {
		t.Errorf("匿名群应展示标题: %s", plain)
	}
}

func TestDownloadFailed(t *testing.T) {
	if got := DownloadFailed("MD5校验失败\n请稍后重试"); got != "MD5校验失败\n请稍后重试" {
		t.Errorf("md5 失败应原样展示: %q", got)
	}
	if got := DownloadFailed("下载超时"); got != "下载超时" {
		t.Errorf("超时应原样展示: %q", got)
	}
	got := DownloadFailed("unexpected error <x>")
	if got != "下载/发送失败\nunexpected error &lt;x&gt;" {
		t.Errorf("其余错误应加前缀并转义: %q", got)
	}
}

func TestDownloadStatusFormat(t *testing.T) {
	got := DownloadStatus("1.5MB/s", 3.0, 10.0, 30)
	want := " 1.5MB/s\n3.00MB/10.00MB 30%"
	if got != want {
		t.Errorf("DownloadStatus = %q, want %q", got, want)
	}
}

func TestLyricCaption(t *testing.T) {
	if got := LyricCaption("<a> & b"); got != "<blockquote expandable>&lt;a&gt; &amp; b</blockquote>" {
		t.Errorf("LyricCaption = %q", got)
	}
	for _, tt := range []struct {
		name      string
		lyric     string
		truncated bool
	}{
		{name: "exact limit", lyric: strings.Repeat("a", 1024)},
		{name: "ascii", lyric: strings.Repeat("a", 1500), truncated: true},
		{name: "cjk", lyric: strings.Repeat("歌词\n", 400), truncated: true},
		{name: "surrogate pairs", lyric: "a" + strings.Repeat("🎵", 600), truncated: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			text := strings.TrimSuffix(strings.TrimPrefix(LyricCaption(tt.lyric), "<blockquote expandable>"), "</blockquote>")
			length := len(utf16.Encode([]rune(text)))
			if !utf8.ValidString(text) || length > 1024 {
				t.Fatalf("caption text has %d UTF-16 units: %q", length, text)
			}
			if !tt.truncated {
				if text != tt.lyric {
					t.Errorf("caption changed: %q", text)
				}
				return
			}
			if !strings.HasSuffix(text, "…") || !strings.HasPrefix(tt.lyric, strings.TrimSuffix(text, "…")) || length < 1023 {
				t.Errorf("caption = %q (%d units), want longest prefix with ellipsis", text, length)
			}
		})
	}
}
