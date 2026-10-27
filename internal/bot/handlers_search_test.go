package bot

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/XiaoMengXinX/Music163Api-Go/utils"
	tgb "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/netease"
	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/render"
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
	if want := "<code>第[1/1]页</code>\n<code>1. </code> <code>&lt;album&gt; &amp; 预 - &lt;gv&gt;/A&amp;B</code>\n"; result != want {
		t.Errorf("search result = %q, want %q", result, want)
	}
}

type searchTelegram struct {
	edits   []string
	answers []string
	markups []models.InlineKeyboardMarkup
	offsets []int
}

func newSearchTest(t *testing.T, total int) (*App, *tgb.Bot, *searchTelegram) {
	t.Helper()
	fake := &searchTelegram{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/editMessageText"):
			fake.edits = append(fake.edits, r.FormValue("text"))
			var markup models.InlineKeyboardMarkup
			if err := json.Unmarshal([]byte(r.FormValue("reply_markup")), &markup); err != nil {
				t.Error(err)
			}
			fake.markups = append(fake.markups, markup)
		case strings.HasSuffix(r.URL.Path, "/answerCallbackQuery"):
			fake.answers = append(fake.answers, r.FormValue("text")+r.FormValue("url"))
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
	offsetPattern := regexp.MustCompile(`"offset":(\d+)`)
	a.api = netease.New("", "hires", &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		params, err := hex.DecodeString(r.Form.Get("params"))
		if err != nil {
			t.Error(err)
		}
		match := offsetPattern.FindStringSubmatch(string(utils.EapiDecrypt(params)))
		if match == nil {
			t.Fatalf("search request without offset: %s", utils.EapiDecrypt(params))
		}
		offset, _ := strconv.Atoi(match[1])
		fake.offsets = append(fake.offsets, offset)
		var songs []string
		for id := offset + 1; id <= min(offset+searchPageSize, total); id++ {
			songs = append(songs, fmt.Sprintf(`{"id":%d,"name":"Song %d","artists":[{"name":"Artist"}]}`, id, id))
		}
		body := fmt.Sprintf(`{"code":200,"result":{"songs":[%s],"songCount":%d}}`, strings.Join(songs, ","), total)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
	})})
	return a, tg, fake
}

func searchCommand(chat models.Chat) *models.Message {
	return &models.Message{
		ID:       1,
		Text:     "/search 晴天",
		Chat:     chat,
		From:     &models.User{ID: 42},
		Entities: []models.MessageEntity{{Type: models.MessageEntityTypeBotCommand, Length: len("/search")}},
	}
}

func searchPress(t *testing.T, handler tgb.HandlerFunc, tg *tgb.Bot, data string, from int64, reply *models.Message, sent time.Time) {
	t.Helper()
	chat := models.Chat{ID: 42, Type: models.ChatTypePrivate}
	if reply != nil {
		chat = reply.Chat
	}
	handler(t.Context(), tg, &models.Update{CallbackQuery: &models.CallbackQuery{
		ID:      "query",
		From:    models.User{ID: from},
		Data:    data,
		Message: models.MaybeInaccessibleMessage{Message: &models.Message{ID: 10, Date: int(sent.Unix()), Chat: chat, ReplyToMessage: reply}},
	}})
}

func TestSearchPagination(t *testing.T) {
	a, tg, fake := newSearchTest(t, 19)
	command := searchCommand(models.Chat{ID: 42, Type: models.ChatTypePrivate})
	a.handleSearch(t.Context(), tg, &models.Update{Message: command})
	hash := searchHash("晴天")
	edited := *command
	edited.Text = "/search 七里香"
	for _, press := range []struct {
		page  int
		reply *models.Message
	}{{2, command}, {3, command}, {4, command}, {2, &edited}, {2, nil}} {
		a.cooldowns = nil
		searchPress(t, a.handleCallbackSearch, tg, fmt.Sprintf("search %d %s", press.page, hash), 42, press.reply, time.Now())
	}

	if want := []int{0, 8, 16, 24}; fmt.Sprint(fake.offsets) != fmt.Sprint(want) {
		t.Errorf("offsets = %v, want %v", fake.offsets, want)
	}
	if want := []string{"", "", render.NoMoreResults, render.SearchExpired, render.SearchExpired}; strings.Join(fake.answers, "|") != strings.Join(want, "|") {
		t.Errorf("answers = %q, want %q", fake.answers, want)
	}
	if len(fake.edits) != 3 {
		t.Fatalf("edits = %d, want 3", len(fake.edits))
	}
	for i, tt := range []struct {
		first, last int
		pages       []string
	}{
		{first: 1, last: 8, pages: []string{"search 2 " + hash}},
		{first: 9, last: 16, pages: []string{"search 1 " + hash, "search 3 " + hash}},
		{first: 17, last: 19, pages: []string{"search 2 " + hash}},
	} {
		page := i + 1
		if !strings.HasPrefix(fake.edits[i], render.SearchPage(page, 3)+render.SearchItem(tt.first, fmt.Sprintf("Song %d", tt.first), "Artist")) ||
			!strings.HasSuffix(fake.edits[i], render.SearchItem(tt.last, fmt.Sprintf("Song %d", tt.last), "Artist")) {
			t.Errorf("page %d text = %q", page, fake.edits[i])
		}
		rows := fake.markups[i].InlineKeyboard
		songs := rows[0]
		if len(songs) != tt.last-tt.first+1 || songs[0].Text != strconv.Itoa(tt.first) || songs[0].CallbackData != fmt.Sprintf("music %d", tt.first) {
			t.Errorf("page %d song buttons = %+v", page, songs)
		}
		var pages []string
		if len(rows) > 1 {
			for _, button := range rows[1] {
				pages = append(pages, button.CallbackData)
			}
		}
		if fmt.Sprint(pages) != fmt.Sprint(tt.pages) {
			t.Errorf("page %d buttons = %v, want %v", page, pages, tt.pages)
		}
	}
}

func TestSearchMaxPage(t *testing.T) {
	a, tg, fake := newSearchTest(t, 100)
	command := searchCommand(models.Chat{ID: 42, Type: models.ChatTypePrivate})
	hash := searchHash("晴天")
	for page := 5; page <= 6; page++ {
		a.cooldowns = nil
		searchPress(t, a.handleCallbackSearch, tg, fmt.Sprintf("search %d %s", page, hash), 42, command, time.Now())
	}
	if fmt.Sprint(fake.offsets) != "[32]" || len(fake.markups) != 1 {
		t.Fatalf("offsets = %v, edits = %d; want only page 5", fake.offsets, len(fake.markups))
	}
	if !strings.HasPrefix(fake.edits[0], render.SearchPage(5, 5)+render.SearchItem(33, "Song 33", "Artist")) {
		t.Errorf("page 5 text = %q", fake.edits[0])
	}
	if rows := fake.markups[0].InlineKeyboard; len(rows) != 2 || len(rows[1]) != 1 || rows[1][0].CallbackData != "search 4 "+hash {
		t.Errorf("page 5 buttons = %+v, want only previous page", rows)
	}
	if want := []string{"", render.SearchExpired}; strings.Join(fake.answers, "|") != strings.Join(want, "|") {
		t.Errorf("answers = %q, want %q", fake.answers, want)
	}
}

func TestSearchCallbackChecks(t *testing.T) {
	group := models.Chat{ID: -100123, Type: models.ChatTypeSupergroup}
	hash := searchHash("晴天")
	deepLink := render.CallbackText + "t.me/testbot?start=1"
	type press struct {
		music bool
		from  int64
		sent  time.Duration
	}
	for _, tt := range []struct {
		name     string
		presses  []press
		want     []string
		requests int
	}{
		{name: "expired page", presses: []press{{from: 42, sent: -11 * time.Minute}}, want: []string{render.SearchExpired}},
		{name: "expired music", presses: []press{{music: true, from: 42, sent: -11 * time.Minute}}, want: []string{render.SearchExpired}},
		{name: "other user page", presses: []press{{from: 7}}, want: []string{render.SearchNotOwner}},
		{name: "other user music", presses: []press{{music: true, from: 7}}, want: []string{render.SearchNotOwner}},
		{name: "page cooldown", presses: []press{{from: 42}, {from: 42}}, want: []string{"", render.Cooldown(5)}, requests: 1},
		{name: "music cooldown", presses: []press{{music: true, from: 42}, {music: true, from: 42}}, want: []string{deepLink, render.Cooldown(10)}},
		{name: "separate cooldowns", presses: []press{{from: 42}, {music: true, from: 42}}, want: []string{"", deepLink}, requests: 1},
		{name: "other user not cooled down", presses: []press{{from: 7}, {from: 42}}, want: []string{render.SearchNotOwner, ""}, requests: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a, tg, fake := newSearchTest(t, 19)
			command := searchCommand(group)
			for _, p := range tt.presses {
				handler, data := a.handleCallbackSearch, "search 2 "+hash
				if p.music {
					handler, data = a.handleCallbackMusic, "music 1"
				}
				searchPress(t, handler, tg, data, p.from, command, time.Now().Add(p.sent))
			}
			if strings.Join(fake.answers, "|") != strings.Join(tt.want, "|") {
				t.Errorf("answers = %q, want %q", fake.answers, tt.want)
			}
			if len(fake.offsets) != tt.requests {
				t.Errorf("search requests = %d, want %d", len(fake.offsets), tt.requests)
			}
		})
	}
}
