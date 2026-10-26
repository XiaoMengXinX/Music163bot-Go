package bot

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tgb "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/netease"
	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/render"
	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/store"
)

func TestLyricFromReplyButton(t *testing.T) {
	var document string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/sendDocument") {
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Error(err)
			}
			if _, header, err := r.FormFile("document"); err == nil {
				document = header.Filename
			}
		}
		_, _ = io.WriteString(w, `{"ok":true,"result":{"message_id":10,"chat":{"id":-100123,"type":"supergroup"}}}`)
	}))
	t.Cleanup(srv.Close)
	tg, err := tgb.New("1:test", tgb.WithServerURL(srv.URL), tgb.WithSkipGetMe(), tgb.WithNotAsyncHandlers())
	if err != nil {
		t.Fatal(err)
	}
	a := newTestApp(tg)
	a.cacheDir = t.TempDir()
	a.api = netease.New("", "hires", &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "search") {
			t.Errorf("reply with song button must not search: %s", r.URL)
		}
		body := `{"/api/song/lyric":{"lrc":{"lyric":"[00:00.00]lyric"}},"/api/v3/song/detail":{"songs":[{"id":123,"name":"Song","ar":[{"name":"Artist"}]}]}}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
	})})
	keyboard := musicKeyboard(store.SongInfo{MusicID: 123, SongName: "Song", SongArtists: "Artist"}, nil)
	a.handleLyric(t.Context(), tg, &models.Update{Message: &models.Message{
		ID:       2,
		Text:     "/lyric",
		Chat:     models.Chat{ID: -100123, Type: models.ChatTypeSupergroup},
		Entities: []models.MessageEntity{{Type: models.MessageEntityTypeBotCommand, Length: len("/lyric")}},
		ReplyToMessage: &models.Message{
			ID:          1,
			Chat:        models.Chat{ID: -100123, Type: models.ChatTypeSupergroup},
			Audio:       &models.Audio{FileID: "audio"},
			ReplyMarkup: &keyboard,
		},
	}})
	if document != "Artist - Song.lrc" {
		t.Errorf("document = %q, want lyric file", document)
	}
}

func TestLyricRejectsProgram(t *testing.T) {
	const link = "https://music.163.com/program?id=2061"
	for _, tt := range []struct {
		name  string
		text  string
		reply *models.Message
	}{
		{name: "argument", text: "/lyric " + link},
		{name: "reply", text: "/lyric", reply: &models.Message{ID: 1, Chat: models.Chat{ID: 42}, Text: link}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var texts []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if err := r.ParseMultipartForm(1 << 20); err != nil {
					t.Error(err)
				}
				texts = append(texts, r.FormValue("text"))
				_, _ = io.WriteString(w, `{"ok":true,"result":{"message_id":10,"chat":{"id":42,"type":"private"}}}`)
			}))
			t.Cleanup(srv.Close)
			tg, err := tgb.New("1:test", tgb.WithServerURL(srv.URL), tgb.WithSkipGetMe(), tgb.WithNotAsyncHandlers())
			if err != nil {
				t.Fatal(err)
			}
			a := newTestApp(tg)
			a.api = netease.New("", "hires", &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				t.Errorf("program link must not request NetEase: %s", r.URL)
				return nil, http.ErrNotSupported
			})})
			a.handleLyric(t.Context(), tg, &models.Update{Message: &models.Message{
				ID:             2,
				Text:           tt.text,
				Chat:           models.Chat{ID: 42, Type: models.ChatTypePrivate},
				Entities:       []models.MessageEntity{{Type: models.MessageEntityTypeBotCommand, Length: len("/lyric")}},
				ReplyToMessage: tt.reply,
			}})
			if want := []string{render.FetchingLyric, render.LrcNoProgram}; strings.Join(texts, "|") != strings.Join(want, "|") {
				t.Errorf("texts = %q, want %q", texts, want)
			}
		})
	}
}
