package bot

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	tgb "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/semaphore"
	"gorm.io/gorm/logger"

	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/fetch"
	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/netease"
	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/recognize"
	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/render"
	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/store"
)

type guestAnswer struct {
	queryID string
	result  struct {
		Type                string                         `json:"type"`
		ID                  string                         `json:"id"`
		Title               string                         `json:"title"`
		DocumentFileID      string                         `json:"document_file_id"`
		Caption             string                         `json:"caption"`
		ParseMode           models.ParseMode               `json:"parse_mode"`
		InputMessageContent models.InputTextMessageContent `json:"input_message_content"`
		ReplyMarkup         *models.InlineKeyboardMarkup   `json:"reply_markup"`
	}
}

type guestTelegram struct {
	methods       []string
	uploads       []url.Values
	answers       []guestAnswer
	audio         []byte
	getFileError  bool
	downloadError bool
	answerError   bool
}

func newGuestTestBot(t *testing.T) (*tgb.Bot, *guestTelegram) {
	t.Helper()
	fake := &guestTelegram{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := filepath.Base(r.URL.Path)
		fake.methods = append(fake.methods, method)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/bot1:test/getFile":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Error(err)
			}
			if r.FormValue("file_id") != "guest-voice" {
				t.Errorf("file_id = %q, want guest-voice", r.FormValue("file_id"))
			}
			if fake.getFileError {
				_, _ = io.WriteString(w, `{"ok":false,"error_code":400,"description":"file unavailable"}`)
				return
			}
			_, _ = io.WriteString(w, `{"ok":true,"result":{"file_id":"guest-voice","file_path":"voice/guest.wav"}}`)
		case "/file/bot1:test/voice/guest.wav":
			if r.Method != http.MethodGet {
				t.Errorf("download method = %s, want GET", r.Method)
			}
			if fake.downloadError {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write(fake.audio)
		case "/bot1:test/sendChatAction":
			_, _ = io.WriteString(w, `{"ok":true,"result":true}`)
		case "/bot1:test/sendAudio":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Error(err)
			}
			if _, _, err := r.FormFile("audio"); err != nil {
				t.Error(err)
			}
			fake.uploads = append(fake.uploads, r.MultipartForm.Value)
			_, _ = io.WriteString(w, `{"ok":true,"result":{"message_id":1,"chat":{"id":-100123,"type":"supergroup"},"audio":{"file_id":"uploaded-audio","file_unique_id":"u","duration":1}}}`)
		case "/bot1:test/answerGuestQuery":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Error(err)
			}
			answer := guestAnswer{queryID: r.FormValue("guest_query_id")}
			if err := json.Unmarshal([]byte(r.FormValue("result")), &answer.result); err != nil {
				t.Error(err)
			}
			fake.answers = append(fake.answers, answer)
			if fake.answerError {
				_, _ = io.WriteString(w, `{"ok":false,"error_code":400,"description":"query expired"}`)
				return
			}
			_, _ = io.WriteString(w, `{"ok":true,"result":{"inline_message_id":"guest-result"}}`)
		default:
			t.Errorf("unexpected Telegram request: %s %s", r.Method, r.URL.Path)
			_, _ = io.WriteString(w, `{"ok":true,"result":{}}`)
		}
	}))
	t.Cleanup(srv.Close)
	tg, err := tgb.New("1:test", tgb.WithServerURL(srv.URL), tgb.WithSkipGetMe(), tgb.WithNotAsyncHandlers())
	if err != nil {
		t.Fatal(err)
	}
	return tg, fake
}

func (f *guestTelegram) checkAnswer(t *testing.T, queryID, text string, button bool) {
	t.Helper()
	link := ""
	if button {
		link = render.SongLink(123)
	}
	f.checkAnswerLink(t, queryID, text, link)
}

func (f *guestTelegram) checkAnswerLink(t *testing.T, queryID, text, link string) {
	t.Helper()
	if len(f.answers) != 1 {
		t.Fatalf("answers = %d, want 1; requests = %v", len(f.answers), f.methods)
	}
	answer := f.answers[0]
	if answer.queryID != queryID {
		t.Errorf("guest_query_id = %q, want %q", answer.queryID, queryID)
	}
	result := answer.result
	if result.Type != "article" || result.ID == "" || len(result.ID) > 64 || result.Title == "" {
		t.Errorf("invalid article: %+v", result)
	}
	content := result.InputMessageContent
	if content.MessageText != text || content.ParseMode != models.ParseModeHTML {
		t.Errorf("message content = %+v, want HTML %q", content, text)
	}
	if content.LinkPreviewOptions == nil || content.LinkPreviewOptions.IsDisabled == nil || !*content.LinkPreviewOptions.IsDisabled {
		t.Error("link previews must be disabled")
	}
	if link != "" {
		want := models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: "在网易云音乐中打开", URL: link}},
			{{Text: "在 Bot 中获取歌曲", URL: "https://t.me/testbot?start=123"}},
		}}
		if result.ReplyMarkup == nil || !reflect.DeepEqual(*result.ReplyMarkup, want) {
			t.Errorf("keyboard = %+v, want %+v", result.ReplyMarkup, want)
		}
	} else if result.ReplyMarkup != nil {
		t.Errorf("error result has keyboard: %+v", result.ReplyMarkup)
	}
}

func TestGuestRecognizeReplyValidation(t *testing.T) {
	for _, tt := range []struct {
		name    string
		message *models.Message
		answer  bool
	}{
		{name: "no guest message"},
		{name: "missing query ID", message: &models.Message{ReplyToMessage: &models.Message{Voice: &models.Voice{FileID: "guest-voice"}}}},
		{name: "no reply", message: &models.Message{GuestQueryID: "query"}, answer: true},
		{name: "non voice reply", message: &models.Message{GuestQueryID: "query", ReplyToMessage: &models.Message{Text: "song"}}, answer: true},
		{name: "voice on guest message only", message: &models.Message{GuestQueryID: "query", Voice: &models.Voice{FileID: "guest-voice"}}, answer: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tg, fake := newGuestTestBot(t)
			a := newTestApp(tg)
			a.handleGuest(t.Context(), tg, &models.Update{GuestMessage: tt.message})
			if !tt.answer {
				if len(fake.methods) != 0 {
					t.Fatalf("unexpected requests: %v", fake.methods)
				}
				return
			}
			fake.checkAnswer(t, "query", render.GuestUsage, false)
			if !reflect.DeepEqual(fake.methods, []string{"answerGuestQuery"}) {
				t.Errorf("requests = %v, want only answerGuestQuery", fake.methods)
			}
		})
	}
}

func TestGuestRecognize(t *testing.T) {
	const name = "<Song & One>"
	const artists = "A&B/<Singer>"
	const album = "<Album & Two>"
	metadata := `{"code":200,"songs":[{"id":123,"name":"<Song & One>","ar":[{"name":"A&B"},{"name":"<Singer>"}],"al":{"name":"<Album & Two>"}}]}`
	for _, tt := range []struct {
		name            string
		getFileError    bool
		downloadError   bool
		invalidAudio    bool
		noMatch         bool
		recognizeError  bool
		cached          bool
		metadataError   bool
		emptyMetadata   bool
		wantText        string
		wantRecognition int
		wantMetadata    int
		wantButton      bool
		wantLog         bool
	}{
		{name: "get file failure", getFileError: true, wantText: render.RecognizeReadFailed, wantLog: true},
		{name: "download failure", downloadError: true, wantText: render.RecognizeReadFailed, wantLog: true},
		{name: "invalid audio", invalidAudio: true, wantText: render.RecognizeFailed, wantLog: true},
		{name: "no match", noMatch: true, wantText: render.RecognizeNoResult, wantRecognition: 1},
		{name: "recognition network failure", recognizeError: true, wantText: render.RecognizeFailed, wantRecognition: 1, wantLog: true},
		{name: "cached metadata without cookie", cached: true, wantText: render.RecognizeResult(name, artists, album), wantRecognition: 1, wantButton: true},
		{name: "API metadata without cookie", wantText: render.RecognizeResult(name, artists, album), wantRecognition: 1, wantMetadata: 1, wantButton: true},
		{name: "metadata failure fallback", metadataError: true, wantText: render.RecognizeSuccess, wantRecognition: 1, wantMetadata: 1, wantButton: true, wantLog: true},
		{name: "empty metadata fallback", emptyMetadata: true, wantText: render.RecognizeSuccess, wantRecognition: 1, wantMetadata: 1, wantButton: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tg, fake := newGuestTestBot(t)
			fake.audio = guestTestAudio()
			fake.getFileError = tt.getFileError
			fake.downloadError = tt.downloadError
			if tt.invalidAudio {
				fake.audio = []byte("invalid audio")
			}
			a := newTestApp(tg)
			a.http = &http.Client{}
			a.cacheDir = t.TempDir()
			var logs bytes.Buffer
			a.log.SetOutput(&logs)
			a.log.SetLevel(logrus.ErrorLevel)
			st, err := store.Open(filepath.Join(t.TempDir(), "cache.db"), logger.Discard)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = st.Close() })
			a.store = st
			if tt.cached {
				if err := st.Save(&store.SongInfo{MusicID: 123, SongName: name, SongArtists: artists, SongAlbum: album}); err != nil {
					t.Fatal(err)
				}
			}
			var recognitionCalls, metadataCalls int
			a.recog = recognize.New(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				recognitionCalls++
				if r.URL.Host != "interface.music.163.com" || r.URL.Path != "/api/music/audio/match" || r.Method != http.MethodPost {
					t.Errorf("unexpected recognition request: %s %s", r.Method, r.URL)
				}
				if err := r.ParseForm(); err != nil {
					t.Error(err)
				}
				if r.Form.Get("rawdata") == "" || r.Form.Get("duration") != "2" {
					t.Error("missing fingerprint or unexpected audio duration")
				}
				if tt.recognizeError {
					return nil, errors.New("recognition network unavailable")
				}
				body := `{"code":200,"data":{"result":[{"song":{"id":123}}]}}`
				if tt.noMatch {
					body = `{"code":200,"data":{"result":[]}}`
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
			})})
			a.api = netease.New("", "hires", &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				metadataCalls++
				if r.URL.Path != "/eapi/v3/song/detail" {
					t.Errorf("expected only song detail, got %s", r.URL)
				}
				if _, err := r.Cookie("MUSIC_U"); !errors.Is(err, http.ErrNoCookie) {
					t.Error("metadata request must work anonymously")
				}
				if tt.metadataError {
					return nil, errors.New("metadata unavailable")
				}
				body := metadata
				if tt.emptyMetadata {
					body = `{"code":200,"songs":[]}`
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
			})})
			queryID := strings.Repeat("guest-query-", 10)
			a.handleGuest(t.Context(), tg, &models.Update{GuestMessage: &models.Message{
				GuestQueryID: queryID,
				Chat:         models.Chat{ID: -10099, Type: models.ChatTypeSupergroup},
				ReplyToMessage: &models.Message{
					Voice: &models.Voice{FileID: "guest-voice"},
				},
			}})
			fake.checkAnswer(t, queryID, tt.wantText, tt.wantButton)
			wantMethods := []string{"getFile", "guest.wav", "answerGuestQuery"}
			if tt.getFileError {
				wantMethods = []string{"getFile", "answerGuestQuery"}
			}
			if !reflect.DeepEqual(fake.methods, wantMethods) {
				t.Errorf("Telegram requests = %v, want %v", fake.methods, wantMethods)
			}
			if recognitionCalls != tt.wantRecognition || metadataCalls != tt.wantMetadata {
				t.Errorf("recognition calls = %d, metadata calls = %d; want %d, %d", recognitionCalls, metadataCalls, tt.wantRecognition, tt.wantMetadata)
			}
			if (logs.Len() != 0) != tt.wantLog {
				t.Errorf("logs = %q, want logged error = %v", logs.String(), tt.wantLog)
			}
			if tt.wantButton && !tt.metadataError && !tt.emptyMetadata {
				text := fake.answers[0].result.InputMessageContent.MessageText
				for _, escaped := range []string{"&lt;Song &amp; One&gt;", "A&amp;B", "&lt;Singer&gt;", "&lt;Album &amp; Two&gt;"} {
					if !strings.Contains(text, escaped) {
						t.Errorf("result %q is missing escaped metadata %q", text, escaped)
					}
				}
			}
			entries, err := os.ReadDir(a.cacheDir)
			if err != nil || len(entries) != 0 {
				t.Errorf("voice temporary files left behind: %v, %v", entries, err)
			}
		})
	}
}

func TestGuestRecognizeAnswerFailure(t *testing.T) {
	tg, fake := newGuestTestBot(t)
	fake.answerError = true
	a := newTestApp(tg)
	var logs bytes.Buffer
	a.log.SetOutput(&logs)
	a.log.SetLevel(logrus.ErrorLevel)
	a.handleGuest(t.Context(), tg, &models.Update{GuestMessage: &models.Message{GuestQueryID: "query"}})
	fake.checkAnswer(t, "query", render.GuestUsage, false)
	if !reflect.DeepEqual(fake.methods, []string{"answerGuestQuery"}) || !strings.Contains(logs.String(), "query expired") {
		t.Errorf("answer failure must be logged without another request: %v, %s", fake.methods, logs.String())
	}
}

func TestGuestRecognizeDispatch(t *testing.T) {
	tg, fake := newGuestTestBot(t)
	a := newTestApp(tg)
	a.registerHandlers(tg)
	tg.ProcessUpdate(t.Context(), &models.Update{Message: &models.Message{GuestQueryID: "ordinary-message", Text: "@testbot"}})
	if len(fake.methods) != 0 {
		t.Fatalf("ordinary message dispatched as guest: %v", fake.methods)
	}
	tg.ProcessUpdate(t.Context(), &models.Update{GuestMessage: &models.Message{GuestQueryID: "guest-message", Text: "@testbot"}})
	fake.checkAnswer(t, "guest-message", render.GuestUsage, false)
	if !reflect.DeepEqual(fake.methods, []string{"answerGuestQuery"}) {
		t.Errorf("requests = %v, want only answerGuestQuery", fake.methods)
	}
}

func TestGuestMusic(t *testing.T) {
	const link = "https://music.163.com/song?id=123&userid=1"
	cachedInfo := store.SongInfo{MusicID: 123, SongName: "<Song>", SongArtists: "A&B", SongAlbum: "Album", FileExt: "flac", FileID: "cached-audio", MusicSize: 1024 * 1024}
	for _, tt := range []struct {
		name          string
		message       *models.Message
		cached        *store.SongInfo
		metadataError bool
		wantText      string
		wantButton    bool
		wantMetadata  int
		wantAudio     bool
	}{
		{name: "reply text link cached", message: &models.Message{ReplyToMessage: &models.Message{Text: "分享歌曲 " + link}}, cached: &cachedInfo, wantAudio: true},
		{name: "reply caption link", message: &models.Message{ReplyToMessage: &models.Message{Caption: link}}, wantText: render.GuestMusicResult("<Song>", "A&B", "Album"), wantButton: true, wantMetadata: 1},
		{name: "own text link", message: &models.Message{Text: "@testbot " + link}, wantText: render.GuestMusicResult("<Song>", "A&B", "Album"), wantButton: true, wantMetadata: 1},
		{name: "metadata failure", message: &models.Message{Text: link}, metadataError: true, wantText: render.GuestNoCache, wantButton: true, wantMetadata: 1},
		{name: "reply without link", message: &models.Message{ReplyToMessage: &models.Message{Text: "https://music.163.com/"}}, wantText: render.GuestUsage},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tg, fake := newGuestTestBot(t)
			a := newTestApp(tg)
			st, err := store.Open(filepath.Join(t.TempDir(), "cache.db"), logger.Discard)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = st.Close() })
			a.store = st
			if tt.cached != nil {
				if err := st.Save(tt.cached); err != nil {
					t.Fatal(err)
				}
			}
			var metadataCalls int
			a.api = netease.New("", "hires", &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				metadataCalls++
				if r.URL.Path != "/eapi/v3/song/detail" {
					t.Errorf("expected only song detail, got %s", r.URL)
				}
				if tt.metadataError {
					return nil, errors.New("metadata unavailable")
				}
				body := `{"code":200,"songs":[{"id":123,"name":"<Song>","ar":[{"name":"A&B"}],"al":{"name":"Album"}}]}`
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
			})})
			tt.message.GuestQueryID = "query"
			a.handleGuest(t.Context(), tg, &models.Update{GuestMessage: tt.message})
			if !reflect.DeepEqual(fake.methods, []string{"answerGuestQuery"}) {
				t.Fatalf("requests = %v, want only answerGuestQuery", fake.methods)
			}
			if metadataCalls != tt.wantMetadata {
				t.Errorf("metadata calls = %d, want %d", metadataCalls, tt.wantMetadata)
			}
			if !tt.wantAudio {
				fake.checkAnswer(t, "query", tt.wantText, tt.wantButton)
				return
			}
			result := fake.answers[0].result
			if result.Type != "document" || result.DocumentFileID != "cached-audio" || result.Title != "A&B - <Song>" || result.ParseMode != models.ParseModeHTML {
				t.Errorf("invalid cached audio: %+v", result)
			}
			if result.Caption != a.musicCaption(cachedInfo) {
				t.Errorf("caption = %q, want %q", result.Caption, a.musicCaption(cachedInfo))
			}
			want := musicKeyboard(cachedInfo, nil)
			if result.ReplyMarkup == nil || !reflect.DeepEqual(*result.ReplyMarkup, want) {
				t.Errorf("keyboard = %+v, want %+v", result.ReplyMarkup, want)
			}
		})
	}
}

func TestGuestMusicCacheChat(t *testing.T) {
	audio := append([]byte("ID3\x03\x00\x00\x00\x00\x00\x00"), bytes.Repeat([]byte{0xff, 0xfb, 0x90, 0x64}, 256)...)
	music := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "123.mp3", time.Time{}, bytes.NewReader(audio))
	}))
	t.Cleanup(music.Close)
	// 下载器只在经反代时跳过证书校验，源站失败后改由反代返回测试音频
	origin := httptest.NewTLSServer(http.NotFoundHandler())
	t.Cleanup(origin.Close)
	for _, tt := range []struct {
		name          string
		cacheChat     int64
		cookieExpired bool
		noURL         bool
		program       bool
		wantUpload    bool
		wantText      string
		wantMetadata  int
	}{
		{name: "upload to cache chat", cacheChat: -100123, wantUpload: true, wantMetadata: 1},
		{name: "no download URL", cacheChat: -100123, noURL: true, wantText: render.GuestMusicResult("<Song>", "A&B", "Album"), wantMetadata: 1},
		{name: "cookie expired", cacheChat: -100123, cookieExpired: true, wantText: render.GuestMusicResult("<Song>", "A&B", "Album"), wantMetadata: 1},
		{name: "no cache chat", wantText: render.GuestMusicResult("<Song>", "A&B", "Album"), wantMetadata: 1},
		{name: "program upload to cache chat", cacheChat: -100123, program: true, wantUpload: true, wantMetadata: 2},
		{name: "program without cache chat", program: true, wantText: render.GuestMusicResult("<Song>", "A&B", "Album"), wantMetadata: 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tg, fake := newGuestTestBot(t)
			a := newTestApp(tg)
			a.cacheDir = t.TempDir()
			a.http = &http.Client{}
			a.dl = fetch.New(a.cacheDir, 10*time.Second, 1, strings.TrimPrefix(music.URL, "https://"), true)
			a.musicSem = semaphore.NewWeighted(1)
			a.cookieOK.Store(!tt.cookieExpired)
			a.cacheChat.Store(tt.cacheChat)
			st, err := store.Open(filepath.Join(t.TempDir(), "cache.db"), logger.Discard)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = st.Close() })
			a.store = st
			detail := `{"songs":[{"id":123,"name":"<Song>","dt":1000,"ar":[{"name":"A&B"}],"al":{"name":"Album"}}]}`
			var metadataCalls int
			a.api = netease.New("cookie", "hires", &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				metadataCalls++
				body := detail
				switch r.URL.Path {
				case "/eapi/dj/program/detail":
					body = `{"code":200,"program":{"id":2061,"name":"Program","mainSong":{"id":123,"name":"<Song>","duration":1000}}}`
				case "/eapi/v3/song/detail":
				default:
					songURL := origin.URL + "/123.mp3"
					if tt.noURL {
						songURL = ""
					}
					body = `{"/api/v3/song/detail":` + detail + `,"/api/song/enhance/player/url/v1":{"data":[{"url":"` + songURL + `","size":` + strconv.Itoa(len(audio)) + `}]}}`
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
			})})
			shared, link := "https://music.163.com/song?id=123", render.SongLink(123)
			if tt.program {
				shared, link = render.ProgramLink(2061), render.ProgramLink(2061)
			}
			a.handleGuest(t.Context(), tg, &models.Update{GuestMessage: &models.Message{
				GuestQueryID:   "query",
				Chat:           models.Chat{ID: 7, Type: models.ChatTypePrivate},
				From:           &models.User{ID: 7},
				ReplyToMessage: &models.Message{Text: shared},
			}})
			if metadataCalls != tt.wantMetadata {
				t.Errorf("metadata calls = %d, want %d", metadataCalls, tt.wantMetadata)
			}
			info, cached := st.GetByMusicID(123)
			if !tt.wantUpload {
				fake.checkAnswerLink(t, "query", tt.wantText, link)
				if len(fake.uploads) != 0 || cached {
					t.Errorf("unexpected upload: %v, cached = %v", fake.uploads, cached)
				}
				return
			}
			if len(fake.uploads) != 1 {
				t.Fatalf("uploads = %d, want 1; requests = %v", len(fake.uploads), fake.methods)
			}
			upload := fake.uploads[0]
			if upload.Get("chat_id") != "-100123" || upload.Get("reply_parameters") != "" {
				t.Errorf("upload target = %v", upload)
			}
			if !cached || info.FileID != "uploaded-audio" || info.FromChatID != 7 || info.FromUserID != 7 {
				t.Errorf("cached info = %+v, %v", info, cached)
			}
			result := fake.answers[0].result
			if result.Type != "document" || result.DocumentFileID != "uploaded-audio" || result.Caption != a.musicCaption(info) {
				t.Errorf("invalid cached audio: %+v", result)
			}
			wantLink := "https://music.163.com/song?id=123"
			if tt.program {
				wantLink = render.ProgramLink(2061)
			}
			var uploadKeyboard models.InlineKeyboardMarkup
			if err := json.Unmarshal([]byte(upload.Get("reply_markup")), &uploadKeyboard); err != nil {
				t.Fatal(err)
			}
			for _, keyboard := range []*models.InlineKeyboardMarkup{&uploadKeyboard, result.ReplyMarkup} {
				if keyboard == nil || keyboard.InlineKeyboard[0][0].URL != wantLink || *keyboard.InlineKeyboard[1][0].SwitchInlineQuery != "https://music.163.com/song?id=123" {
					t.Errorf("keyboard = %+v, want link %s", keyboard, wantLink)
				}
			}
			entries, err := os.ReadDir(a.cacheDir)
			if err != nil || len(entries) != 0 {
				t.Errorf("music temporary files left behind: %v, %v", entries, err)
			}
		})
	}
}

func guestTestAudio() []byte {
	const sampleRate = 8000
	const samples = sampleRate * 2
	audio := make([]byte, 44+samples*2)
	copy(audio, "RIFF")
	binary.LittleEndian.PutUint32(audio[4:], uint32(len(audio)-8))
	copy(audio[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(audio[16:], 16)
	binary.LittleEndian.PutUint16(audio[20:], 1)
	binary.LittleEndian.PutUint16(audio[22:], 1)
	binary.LittleEndian.PutUint32(audio[24:], sampleRate)
	binary.LittleEndian.PutUint32(audio[28:], sampleRate*2)
	binary.LittleEndian.PutUint16(audio[32:], 2)
	binary.LittleEndian.PutUint16(audio[34:], 16)
	copy(audio[36:], "data")
	binary.LittleEndian.PutUint32(audio[40:], samples*2)
	for i := range samples {
		sample := int16(16000 * math.Sin(2*math.Pi*440*float64(i)/sampleRate))
		binary.LittleEndian.PutUint16(audio[44+i*2:], uint16(sample))
	}
	return audio
}
