package bot

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	tgb "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/config"
)

func newUploadApp(t *testing.T, handler http.HandlerFunc) *App {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	cfg := config.Default()
	cfg.Bot.Token = "1:test"
	cfg.Bot.APIURL = srv.URL
	return &App{cfg: cfg, tgHTTP: srv.Client()}
}

func TestSendAudioFile(t *testing.T) {
	dir := t.TempDir()
	audioPath := filepath.Join(dir, "a - b.flac")
	thumbPath := filepath.Join(dir, "cover.jpg")
	if err := os.WriteFile(audioPath, []byte("audio-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(thumbPath, []byte("thumb-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	a := newUploadApp(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bot1:test/sendAudio" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.ContentLength <= 0 || len(r.TransferEncoding) != 0 {
			t.Errorf("ContentLength = %d, TransferEncoding = %v", r.ContentLength, r.TransferEncoding)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		for k, want := range map[string]string{"chat_id": "42", "caption": "<b>hi</b>", "duration": "180", "parse_mode": "HTML"} {
			if got := r.FormValue(k); got != want {
				t.Errorf("%s = %q, want %q", k, got, want)
			}
		}
		var markup models.InlineKeyboardMarkup
		if err := json.Unmarshal([]byte(r.FormValue("reply_markup")), &markup); err != nil || markup.InlineKeyboard[0][0].Text != "btn" {
			t.Errorf("reply_markup = %q, err %v", r.FormValue("reply_markup"), err)
		}
		for field, want := range map[string][2]string{"audio": {"a - b.flac", "audio-bytes"}, "thumbnail": {"cover.jpg", "thumb-bytes"}} {
			f, h, err := r.FormFile(field)
			if err != nil {
				t.Fatalf("%s: %v", field, err)
			}
			data, _ := io.ReadAll(f)
			if h.Filename != want[0] || string(data) != want[1] {
				t.Errorf("%s = %s %q, want %s %q", field, h.Filename, data, want[0], want[1])
			}
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":7,"audio":{"file_id":"fid"}}}`))
	})

	msg, err := a.sendAudioFile(t.Context(), &tgb.SendAudioParams{
		ChatID:      42,
		Caption:     "<b>hi</b>",
		ParseMode:   models.ParseModeHTML,
		Duration:    180,
		ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: "btn", URL: "https://example.com"}}}},
	}, audioPath, thumbPath)
	if err != nil {
		t.Fatal(err)
	}
	if msg.ID != 7 || msg.Audio.FileID != "fid" {
		t.Errorf("message = %+v", msg)
	}
}

func TestSendAudioFileAPIError(t *testing.T) {
	audioPath := filepath.Join(t.TempDir(), "a.mp3")
	if err := os.WriteFile(audioPath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := newUploadApp(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":429,"description":"Too Many Requests: retry after 5","parameters":{"retry_after":5}}`))
	})
	_, err := a.sendAudioFile(t.Context(), &tgb.SendAudioParams{ChatID: 42}, audioPath, "")
	if tooMany, ok := errors.AsType[*tgb.TooManyRequestsError](err); !ok || tooMany.RetryAfter != 5 {
		t.Fatalf("err = %v, want TooManyRequestsError with RetryAfter 5", err)
	}
}
