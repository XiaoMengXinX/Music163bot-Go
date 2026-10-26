package bot

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/XiaoMengXinX/Music163Api-Go/types"
	tgb "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"gorm.io/gorm/logger"

	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/render"
	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/store"
)

func TestMessageMusic(t *testing.T) {
	audioKeyboard := musicKeyboard(store.SongInfo{MusicID: 123, SongName: "Song", SongArtists: "Artist"}, nil)
	guestKeyboard := linkKeyboard(render.SongLink(456))
	for _, tt := range []struct {
		name        string
		message     *models.Message
		want        int
		wantProgram int
	}{
		{name: "text", message: &models.Message{Text: "分享 https://music.163.com/song?id=123&userid=1"}, want: 123},
		{name: "caption", message: &models.Message{Caption: "https://music.163.com/#/song?id=123"}, want: 123},
		{name: "caption text link", message: &models.Message{
			Caption:         "点我",
			CaptionEntities: []models.MessageEntity{{Type: models.MessageEntityTypeTextLink, URL: "https://music.163.com/song?id=123"}},
		}, want: 123},
		{name: "audio button", message: &models.Message{Caption: "「Song」- Artist\nvia @testbot", ReplyMarkup: &audioKeyboard}, want: 123},
		{name: "song link button", message: &models.Message{Text: "已识别到歌曲", ReplyMarkup: &guestKeyboard}, want: 456},
		{name: "text before button", message: &models.Message{Text: "https://music.163.com/song?id=789", ReplyMarkup: &audioKeyboard}, want: 789},
		{name: "program", message: &models.Message{Text: "https://music.163.com/program?id=2061"}, wantProgram: 2061},
		{name: "program before button", message: &models.Message{Text: "https://music.163.com/dj?id=2061", ReplyMarkup: &audioKeyboard}, wantProgram: 2061},
		{name: "no link", message: &models.Message{Text: "雨き声残響 2", Entities: []models.MessageEntity{{Type: models.MessageEntityTypeURL}}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, program := messageMusic(tt.message)
			if got != tt.want || program != tt.wantProgram {
				t.Errorf("messageMusic = %d, %d; want %d, %d", got, program, tt.want, tt.wantProgram)
			}
		})
	}
}

func TestProcessCachedProgramKeyboard(t *testing.T) {
	var keyboards []models.InlineKeyboardMarkup
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/sendAudio"):
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Error(err)
			}
			var keyboard models.InlineKeyboardMarkup
			if err := json.Unmarshal([]byte(r.FormValue("reply_markup")), &keyboard); err != nil {
				t.Error(err)
			}
			keyboards = append(keyboards, keyboard)
		case strings.HasSuffix(r.URL.Path, "/deleteMessage"):
			_, _ = io.WriteString(w, `{"ok":true,"result":true}`)
			return
		}
		_, _ = io.WriteString(w, `{"ok":true,"result":{"message_id":10,"chat":{"id":42,"type":"private"}}}`)
	}))
	t.Cleanup(srv.Close)
	tg, err := tgb.New("1:test", tgb.WithServerURL(srv.URL), tgb.WithSkipGetMe())
	if err != nil {
		t.Fatal(err)
	}
	a := newTestApp(tg)
	st, err := store.Open(filepath.Join(t.TempDir(), "cache.db"), logger.Discard)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	a.store = st
	if err := st.Save(&store.SongInfo{MusicID: 123, SongName: "Song", FileID: "cached-audio"}); err != nil {
		t.Fatal(err)
	}
	var prog types.ProgramDetailData
	prog.Program.ID = 2061
	for _, p := range []*types.ProgramDetailData{nil, &prog} {
		if err := a.processMusic(t.Context(), 123, &models.Message{ID: 1, Chat: models.Chat{ID: 42}}, p, nil); err != nil {
			t.Fatal(err)
		}
	}
	if len(keyboards) != 2 {
		t.Fatalf("sent audio = %d, want 2", len(keyboards))
	}
	for i, want := range []string{"https://music.163.com/song?id=123", render.ProgramLink(2061)} {
		if got := keyboards[i].InlineKeyboard[0][0].URL; got != want {
			t.Errorf("link = %q, want %q", got, want)
		}
		if got := *keyboards[i].InlineKeyboard[1][0].SwitchInlineQuery; got != "https://music.163.com/song?id=123" {
			t.Errorf("inline query = %q, want song link", got)
		}
	}
}
