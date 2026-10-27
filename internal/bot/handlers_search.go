package bot

import (
	"context"
	"hash/fnv"
	"math"
	"strconv"
	"strings"
	"time"

	tgb "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/render"
	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/textx"
)

const (
	searchPageSize      = 8
	searchMaxPage       = 5
	searchLifetime      = 10 * time.Minute
	searchMusicCooldown = 10 * time.Second
	searchPageCooldown  = 5 * time.Second
)

func (a *App) handleSearch(ctx context.Context, tg *tgb.Bot, update *models.Update) {
	m := update.Message
	args := commandArguments(m)
	if args == "" {
		if _, err := a.reply(ctx, m, render.InputKeyword); err != nil {
			a.log.Errorln(err)
		}
		return
	}
	msgResult, err := a.reply(ctx, m, render.Searching)
	if err != nil {
		a.log.Errorln(err)
		return
	}
	text, keyboard, err := a.searchPage(args, 1)
	if err != nil {
		a.log.Errorln(err)
	}
	if text == "" {
		if err := a.editMessage(ctx, m.Chat.ID, msgResult.ID, render.NoResults, nil); err != nil {
			a.log.Errorln(err)
		}
		return
	}
	if err := a.editMessage(ctx, m.Chat.ID, msgResult.ID, text, keyboard); err != nil {
		a.log.Errorln(err)
	}
}

func (a *App) handleCallbackSearch(ctx context.Context, tg *tgb.Bot, update *models.Update) {
	query := update.CallbackQuery
	msg, answer := a.checkSearch(query, "page", searchPageCooldown)
	if msg != nil {
		answer = a.turnPage(ctx, msg, query.Data)
	}
	a.answerCallback(ctx, query.ID, answer)
}

// turnPage 关键词取自被回复的 /search 命令，回调中的哈希用于确认命令未被编辑
func (a *App) turnPage(ctx context.Context, msg *models.Message, data string) string {
	args := strings.Fields(data)
	if len(args) != 3 || len(msg.ReplyToMessage.Entities) == 0 {
		return render.SearchExpired
	}
	page, _ := strconv.Atoi(args[1])
	keyword := commandArguments(msg.ReplyToMessage)
	if page < 1 || page > searchMaxPage || keyword == "" || searchHash(keyword) != args[2] {
		return render.SearchExpired
	}
	text, keyboard, err := a.searchPage(keyword, page)
	if err != nil {
		a.log.Errorln(err)
	}
	if text == "" {
		return render.NoMoreResults
	}
	if err := a.editMessage(ctx, msg.Chat.ID, msg.ID, text, keyboard); err != nil {
		a.log.Errorln(err)
	}
	return ""
}

// checkSearch 搜索结果按钮仅限发起人在有效期内操作，失败时返回提示
func (a *App) checkSearch(query *models.CallbackQuery, op string, cooldown time.Duration) (*models.Message, string) {
	msg := query.Message.Message
	if msg == nil || msg.ReplyToMessage == nil || msg.ReplyToMessage.From == nil || time.Since(time.Unix(int64(msg.Date), 0)) > searchLifetime {
		return nil, render.SearchExpired
	}
	if query.From.ID != msg.ReplyToMessage.From.ID {
		return nil, render.SearchNotOwner
	}
	if remain := a.cooldown(op, query.From.ID, cooldown); remain > 0 {
		return nil, render.Cooldown(int(math.Ceil(remain.Seconds())))
	}
	return msg, ""
}

type cooldownKey struct {
	op     string
	userID int64
}

// cooldown 冷却中返回剩余时间，否则开始新的冷却
func (a *App) cooldown(op string, userID int64, d time.Duration) time.Duration {
	a.cooldownMu.Lock()
	defer a.cooldownMu.Unlock()
	now := time.Now()
	key := cooldownKey{op, userID}
	if until := a.cooldowns[key]; now.Before(until) {
		return until.Sub(now)
	}
	if a.cooldowns == nil {
		a.cooldowns = make(map[cooldownKey]time.Time)
	}
	// 冷却很短，记录较多时顺带清理
	if len(a.cooldowns) >= 1024 {
		for k, until := range a.cooldowns {
			if !now.Before(until) {
				delete(a.cooldowns, k)
			}
		}
	}
	a.cooldowns[key] = now.Add(d)
	return 0
}

func (a *App) answerCallback(ctx context.Context, queryID, text string) {
	if _, err := a.tg.AnswerCallbackQuery(ctx, &tgb.AnswerCallbackQueryParams{CallbackQueryID: queryID, Text: text}); err != nil {
		a.log.Errorln(err)
	}
}

// searchPage 没有结果时返回空文本
func (a *App) searchPage(keyword string, page int) (string, models.InlineKeyboardMarkup, error) {
	result, err := a.api.SearchSong(keyword, searchPageSize, (page-1)*searchPageSize)
	if err != nil || len(result.Result.Songs) == 0 {
		return "", models.InlineKeyboardMarkup{}, err
	}
	total := max(page, min(searchMaxPage, (result.Result.SongCount+searchPageSize-1)/searchPageSize))
	start := (page-1)*searchPageSize + 1
	var text strings.Builder
	text.WriteString(render.SearchPage(page, total))
	musicIDs := make([]int, 0, len(result.Result.Songs))
	for i, song := range result.Result.Songs {
		names := make([]string, 0, len(song.Artists))
		for _, artist := range song.Artists {
			names = append(names, artist.Name)
		}
		musicIDs = append(musicIDs, song.Id)
		text.WriteString(render.SearchItem(start+i, song.Name, textx.JoinNames(names)))
	}
	return text.String(), searchKeyboard(musicIDs, start, page, page < total, searchHash(keyword)), nil
}

func searchHash(keyword string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(keyword))
	return strconv.FormatUint(uint64(h.Sum32()), 36)
}

func (a *App) handleCallbackMusic(ctx context.Context, tg *tgb.Bot, update *models.Update) {
	query := update.CallbackQuery
	args := strings.Split(query.Data, " ")
	if len(args) < 2 {
		return
	}
	musicID, _ := strconv.Atoi(args[1])

	msg, answer := a.checkSearch(query, "music", searchMusicCooldown)
	if msg == nil {
		a.answerCallback(ctx, query.ID, answer)
		return
	}
	a.answerCallback(ctx, query.ID, render.CallbackText)
	if err := a.processMusic(ctx, musicID, msg.ReplyToMessage, nil, nil); err != nil {
		a.log.Errorln(err)
	}
}
