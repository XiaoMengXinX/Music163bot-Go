package bot

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tgb "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
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

func TestRecognizeDownloadFailureCleansTempDir(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"result":{"file_id":"voice","file_path":"relative.ogg"}}`))
	}))
	defer srv.Close()
	tg, err := tgb.New("1:test", tgb.WithServerURL(srv.URL), tgb.WithSkipGetMe())
	if err != nil {
		t.Fatal(err)
	}
	a := newTestApp(tg)
	a.cfg.Bot.LocalMode = true
	a.cacheDir = t.TempDir()
	a.handleRecognize(context.Background(), tg, &models.Update{Message: &models.Message{
		ReplyToMessage: &models.Message{Voice: &models.Voice{FileID: "voice"}},
	}})
	entries, err := os.ReadDir(a.cacheDir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("temporary directory left after failed download: %v, %v", entries, err)
	}
}
