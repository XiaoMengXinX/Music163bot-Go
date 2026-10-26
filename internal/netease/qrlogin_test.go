package netease

import (
	"net/http"
	"testing"
)

func TestExtractMUSICU(t *testing.T) {
	header := http.Header{}
	header.Add("Set-Cookie", "NMTID=xxx; Max-Age=31536000; Domain=.music.163.com; Path=/")
	header.Add("Set-Cookie", "MUSIC_U=abc123; Max-Age=31536000; Domain=.music.163.com; Path=/")
	header.Add("Set-Cookie", "__csrf=yyy; Path=/")

	if got := extractMUSICU(header); got != "abc123" {
		t.Errorf("extractMUSICU = %q, want abc123", got)
	}

	bare := http.Header{}
	bare.Add("Set-Cookie", "MUSIC_U=a-b_c.1%2F2")
	if got := extractMUSICU(bare); got != "a-b_c.1%2F2" {
		t.Errorf("无属性 Set-Cookie 解析错误: %q", got)
	}

	none := http.Header{}
	none.Add("Set-Cookie", "NMTID=xxx; Path=/")
	if got := extractMUSICU(none); got != "" {
		t.Errorf("无 MUSIC_U 应返回空, got %q", got)
	}
}

func TestSetMUSICU(t *testing.T) {
	c := New("old", "hires", &http.Client{})
	c.SetMUSICU("new")
	data := c.requestData()
	if len(data.Cookies) != 1 || data.Cookies[0].Value != "new" {
		t.Errorf("SetMUSICU 后 cookie 错误: %+v", data.Cookies)
	}
	if data.Cookies[0].Name != "MUSIC_U" {
		t.Errorf("cookie 名错误: %s", data.Cookies[0].Name)
	}
}
