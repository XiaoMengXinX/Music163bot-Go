package netease

import (
	"errors"
	"net/http"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestClientHTTP(t *testing.T) {
	wantErr := errors.New("proxy unavailable")
	var calls int
	var wantCookie string
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		cookie, err := r.Cookie("MUSIC_U")
		if wantCookie == "" {
			if !errors.Is(err, http.ErrNoCookie) {
				t.Errorf("匿名请求不应携带 MUSIC_U: %v", cookie)
			}
		} else if err != nil || cookie.Value != wantCookie {
			t.Errorf("MUSIC_U 错误: got %v, want %q", cookie, wantCookie)
		}
		return nil, wantErr
	})}
	c := New("old", "hires", httpClient)
	for _, musicU := range []string{"old", "new"} {
		c.SetMUSICU(musicU)
		for _, tt := range []struct {
			name      string
			anonymous bool
			call      func() error
		}{
			{name: "song detail", call: func() error { _, err := c.SongDetail(123); return err }},
			{name: "song", call: func() error { _, _, err := c.SongDetailAndURL(123); return err }},
			{name: "lyric", call: func() error { _, _, err := c.LyricAndDetail(123); return err }},
			{name: "search", call: func() error { _, err := c.SearchSong("test", 1, 0); return err }},
			{name: "program", call: func() error { _, err := c.ProgramDetail(123); return err }},
			{name: "login status", call: func() error { _, err := c.LoginStatusValid(); return err }},
			{name: "QR key", anonymous: true, call: func() error { _, err := c.CreateQRLoginKey(); return err }},
			{name: "QR status", anonymous: true, call: func() error { _, _, _, err := c.CheckQRLogin("test"); return err }},
		} {
			t.Run(musicU+"/"+tt.name, func(t *testing.T) {
				wantCookie = musicU
				if tt.anonymous {
					wantCookie = ""
				}
				before := calls
				if err := tt.call(); !errors.Is(err, wantErr) {
					t.Errorf("应返回代理错误: %v", err)
				}
				if calls != before+1 {
					t.Errorf("客户端请求次数错误: got %d, want 1", calls-before)
				}
			})
		}
	}
}
