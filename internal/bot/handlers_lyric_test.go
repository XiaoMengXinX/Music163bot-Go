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
	keyboard := musicKeyboard("Song", "Artist", 123)
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
