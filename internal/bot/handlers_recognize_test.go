package bot

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tgb "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/recognize"
	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/render"
	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/store"
	"gorm.io/gorm/logger"
)

func TestDownloadVoiceLocal(t *testing.T) {
	tg, rec := newTestBot(t)
	a := newTestApp(tg)
	a.cfg.Bot.LocalMode = true
	original := filepath.Join(t.TempDir(), "voice.ogg")
	content := []byte("voice data")
	if err := os.WriteFile(original, content, 0o600); err != nil {
		t.Fatal(err)
	}
	workDir := t.TempDir()
	dst := filepath.Join(workDir, "copy.ogg")
	if err := a.downloadVoice(context.Background(), dst, &models.File{FilePath: original}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dst)
	if err != nil || string(got) != string(content) {
		t.Fatalf("copied voice = %q, error = %v", got, err)
	}
	if rec.has("/file/") {
		t.Fatal("local mode must not request an HTTP download")
	}
	if err := os.RemoveAll(workDir); err != nil {
		t.Fatal(err)
	}
	got, err = os.ReadFile(original)
	if err != nil || string(got) != string(content) {
		t.Fatalf("original voice changed after cleanup: %q, %v", got, err)
	}
}

func TestDownloadVoiceLocalInvalidPath(t *testing.T) {
	a := newTestApp(nil)
	a.cfg.Bot.LocalMode = true
	dir := t.TempDir()
	for _, path := range []string{"", "voice.ogg", filepath.Join(dir, "missing.ogg"), dir} {
		t.Run(path, func(t *testing.T) {
			dst := filepath.Join(t.TempDir(), "voice.ogg")
			if err := a.downloadVoice(context.Background(), dst, &models.File{FilePath: path}); err == nil {
				t.Fatal("expected local file error")
			}
			if _, err := os.Stat(dst); !os.IsNotExist(err) {
				t.Fatalf("destination created for invalid source: %v", err)
			}
		})
	}
}

func TestDownloadVoiceHTTP(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusNotFound} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/file/bot1:test/voice/voice.ogg" || r.Method != http.MethodGet {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				w.WriteHeader(status)
				_, _ = w.Write([]byte("voice data"))
			}))
			defer srv.Close()
			tg, err := tgb.New("1:test", tgb.WithServerURL(srv.URL), tgb.WithSkipGetMe())
			if err != nil {
				t.Fatal(err)
			}
			a := newTestApp(tg)
			a.http = srv.Client()
			dst := filepath.Join(t.TempDir(), "voice.ogg")
			file := &models.File{FilePath: "voice/voice.ogg"}
			err = a.downloadVoice(context.Background(), dst, file)
			if status != http.StatusOK {
				if err == nil || !strings.Contains(err.Error(), "404") {
					t.Fatalf("expected HTTP 404 error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(dst)
			if err != nil || string(got) != "voice data" {
				t.Fatalf("downloaded voice = %q, error = %v", got, err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if err := a.downloadVoice(ctx, dst, file); !errors.Is(err, context.Canceled) {
				t.Fatalf("expected canceled download, got %v", err)
			}
		})
	}
}

func TestProcessMusicCachedInfo(t *testing.T) {
	rec := &requestRecorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.add(r.URL.Path)
		if strings.HasSuffix(r.URL.Path, "/deleteMessage") {
			_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{}}`))
	}))
	defer srv.Close()
	tg, err := tgb.New("1:test", tgb.WithServerURL(srv.URL), tgb.WithSkipGetMe())
	if err != nil {
		t.Fatal(err)
	}
	a := newTestApp(tg)
	st, err := store.Open(filepath.Join(t.TempDir(), "cache.db"), logger.Discard)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	a.store = st
	info := store.SongInfo{
		MusicID: 123, SongName: "雨き声残響", SongArtists: "ゆめこ",
		SongAlbum: "最新热歌慢摇92", FileID: "cached-audio",
	}
	if err := st.Save(&info); err != nil {
		t.Fatal(err)
	}
	called := false
	err = a.processMusic(context.Background(), info.MusicID, &models.Message{ID: 1, Chat: models.Chat{ID: 42}}, nil, func(got store.SongInfo) {
		called = true
		if got.SongName != info.SongName || got.SongArtists != info.SongArtists || got.SongAlbum != info.SongAlbum {
			t.Errorf("unexpected song info: %+v", got)
		}
		if rec.has("/sendAudio") {
			t.Error("song info should be available before sending audio")
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if !called || !rec.has("/sendAudio") {
		t.Fatal("cached song should provide metadata and send audio")
	}
}

func TestRecognizeFailureStatus(t *testing.T) {
	for _, tt := range []struct {
		name string
		want string
	}{
		{name: "get file", want: render.RecognizeReadFailed},
		{name: "download", want: render.RecognizeReadFailed},
		{name: "invalid audio", want: render.RecognizeFailed},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "voice.ogg")
			if err := os.WriteFile(path, []byte("invalid audio"), 0o600); err != nil {
				t.Fatal(err)
			}
			var texts []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/getFile") {
					if tt.name == "get file" {
						_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"file unavailable"}`))
						return
					}
					filePath := path
					if tt.name == "download" {
						filePath = "relative.ogg"
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": models.File{FileID: "voice", FilePath: filePath}})
					return
				}
				if err := r.ParseMultipartForm(1 << 20); err != nil {
					t.Error(err)
				}
				if strings.HasSuffix(r.URL.Path, "/editMessageText") && r.FormValue("message_id") != "10" {
					t.Errorf("status message ID = %s, want 10", r.FormValue("message_id"))
				}
				texts = append(texts, r.FormValue("text"))
				_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":10,"chat":{"id":42,"type":"private"}}}`))
			}))
			defer srv.Close()
			tg, err := tgb.New("1:test", tgb.WithServerURL(srv.URL), tgb.WithSkipGetMe())
			if err != nil {
				t.Fatal(err)
			}
			a := newTestApp(tg)
			a.cfg.Bot.LocalMode = true
			a.cacheDir = t.TempDir()
			a.recog = recognize.New(&http.Client{})
			a.handleRecognize(context.Background(), tg, &models.Update{Message: &models.Message{
				ReplyToMessage: &models.Message{ID: 1, Chat: models.Chat{ID: 42}, Voice: &models.Voice{FileID: "voice"}},
			}})
			if len(texts) != 2 || texts[0] != render.Recognizing || texts[1] != tt.want {
				t.Errorf("unexpected status messages: %q", texts)
			}
			entries, err := os.ReadDir(a.cacheDir)
			if err != nil || len(entries) != 0 {
				t.Fatalf("temporary directory left after failure: %v, %v", entries, err)
			}
		})
	}
}
