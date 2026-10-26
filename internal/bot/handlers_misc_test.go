package bot

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	tgb "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/render"
)

func TestSetCacheChat(t *testing.T) {
	const original = "[bot]\ntoken = \"1:test\"\n"
	for _, tt := range []struct {
		name       string
		text       string
		initial    int64
		wantChat   int64
		wantConfig string
		wantSent   []string
	}{
		{name: "current chat", text: "/setcachechat", wantChat: -100123, wantConfig: "[bot]\ncache_chat = -100123\ntoken = \"1:test\"\n",
			wantSent: []string{"-100123:" + render.CacheChatSet(-100123)}},
		{name: "other chat", text: "/setcachechat -100456", wantChat: -100456, wantConfig: "[bot]\ncache_chat = -100456\ntoken = \"1:test\"\n",
			wantSent: []string{"-100456:" + render.CacheChatTest, "-100123:" + render.CacheChatSet(-100456)}},
		{name: "unreachable chat", text: "/setcachechat -100999", initial: -100456, wantChat: -100456, wantConfig: original,
			wantSent: []string{"-100999:" + render.CacheChatTest, "-100123:" + render.CacheChatUnreachable}},
		{name: "invalid ID", text: "/setcachechat abc", initial: -100456, wantChat: -100456, wantConfig: original,
			wantSent: []string{"-100123:" + render.InputChatID}},
		{name: "disable", text: "/setcachechat 0", initial: -100456, wantConfig: "[bot]\ncache_chat = 0\ntoken = \"1:test\"\n",
			wantSent: []string{"-100123:" + render.CacheChatDisabled}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var sent []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path != "/bot1:test/sendMessage" {
					t.Errorf("unexpected Telegram request: %s", r.URL.Path)
				}
				if err := r.ParseMultipartForm(1 << 20); err != nil {
					t.Error(err)
				}
				sent = append(sent, r.FormValue("chat_id")+":"+r.FormValue("text"))
				if r.FormValue("chat_id") == "-100999" {
					_, _ = io.WriteString(w, `{"ok":false,"error_code":403,"description":"bot is not a member"}`)
					return
				}
				_, _ = io.WriteString(w, `{"ok":true,"result":{"message_id":1,"chat":{"id":1,"type":"supergroup"}}}`)
			}))
			t.Cleanup(srv.Close)
			tg, err := tgb.New("1:test", tgb.WithServerURL(srv.URL), tgb.WithSkipGetMe(), tgb.WithNotAsyncHandlers())
			if err != nil {
				t.Fatal(err)
			}
			a := newTestApp(tg)
			a.cfgPath = filepath.Join(t.TempDir(), "config.toml")
			if err := os.WriteFile(a.cfgPath, []byte(original), 0o644); err != nil {
				t.Fatal(err)
			}
			a.cacheChat.Store(tt.initial)
			a.registerHandlers(tg)

			for _, from := range []int64{7, 42} {
				tg.ProcessUpdate(t.Context(), &models.Update{Message: &models.Message{
					ID:       1,
					Text:     tt.text,
					Chat:     models.Chat{ID: -100123, Type: models.ChatTypeSupergroup},
					From:     &models.User{ID: from},
					Entities: []models.MessageEntity{{Type: models.MessageEntityTypeBotCommand, Length: len("/setcachechat")}},
				}})
				if from == 7 && len(sent) != 0 {
					t.Fatalf("non-admin command handled: %v", sent)
				}
			}

			if got := a.cacheChat.Load(); got != tt.wantChat {
				t.Errorf("cache chat = %d, want %d", got, tt.wantChat)
			}
			config, err := os.ReadFile(a.cfgPath)
			if err != nil {
				t.Fatal(err)
			}
			if string(config) != tt.wantConfig {
				t.Errorf("config = %q, want %q", config, tt.wantConfig)
			}
			if len(sent) != len(tt.wantSent) {
				t.Fatalf("sent = %q, want %q", sent, tt.wantSent)
			}
			for i := range sent {
				if sent[i] != tt.wantSent[i] {
					t.Errorf("sent[%d] = %q, want %q", i, sent[i], tt.wantSent[i])
				}
			}
		})
	}
}
