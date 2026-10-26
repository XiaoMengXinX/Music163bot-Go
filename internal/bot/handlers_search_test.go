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

func TestSearchEscapesResults(t *testing.T) {
	var result string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/editMessageText") {
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Error(err)
			}
			result = r.FormValue("text")
		}
		_, _ = io.WriteString(w, `{"ok":true,"result":{"message_id":10,"chat":{"id":42,"type":"private"}}}`)
	}))
	t.Cleanup(srv.Close)
	tg, err := tgb.New("1:test", tgb.WithServerURL(srv.URL), tgb.WithSkipGetMe())
	if err != nil {
		t.Fatal(err)
	}
	a := newTestApp(tg)
	a.api = netease.New("", "hires", &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body := `{"code":200,"result":{"songs":[{"id":1,"name":"<album> & 预","artists":[{"name":"<gv>"},{"name":"A&B"}]}]}}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
	})})
	a.handleSearch(t.Context(), tg, &models.Update{Message: &models.Message{
		ID:       1,
		Text:     "/search album",
		Chat:     models.Chat{ID: 42, Type: models.ChatTypePrivate},
		Entities: []models.MessageEntity{{Type: models.MessageEntityTypeBotCommand, Length: len("/search")}},
	}})
	if want := "1.「&lt;album&gt; &amp; 预」 - &lt;gv&gt;/A&amp;B\n"; result != want {
		t.Errorf("search result = %q, want %q", result, want)
	}
}
