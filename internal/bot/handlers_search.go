package bot

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	tgb "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/render"
	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/textx"
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
	searchResult, err := a.api.SearchSong(args, 10)
	if err != nil {
		a.log.Errorln(err)
	}
	if len(searchResult.Result.Songs) == 0 {
		if err := a.editMessage(ctx, m.Chat.ID, msgResult.ID, render.NoResults, nil); err != nil {
			a.log.Errorln(err)
		}
		return
	}

	var musicIDs []int
	var textMessage string
	for i := 0; i < len(searchResult.Result.Songs) && i < 8; i++ {
		song := searchResult.Result.Songs[i]
		var names []string
		for _, artist := range song.Artists {
			names = append(names, artist.Name)
		}
		musicIDs = append(musicIDs, song.Id)
		textMessage = fmt.Sprintf("%s%d.「%s」 - %s\n", textMessage, i+1, render.Escape(song.Name), render.Escape(textx.JoinNames(names)))
	}
	if err := a.editMessage(ctx, m.Chat.ID, msgResult.ID, textMessage, searchKeyboard(musicIDs)); err != nil {
		a.log.Errorln(err)
	}
}

func (a *App) handleCallbackMusic(ctx context.Context, tg *tgb.Bot, update *models.Update) {
	query := update.CallbackQuery
	args := strings.Split(query.Data, " ")
	if len(args) < 2 {
		return
	}
	musicID, _ := strconv.Atoi(args[1])

	var msg *models.Message
	if query.Message.Message != nil {
		msg = query.Message.Message
	}
	if msg == nil {
		_, _ = a.tg.AnswerCallbackQuery(ctx, &tgb.AnswerCallbackQueryParams{
			CallbackQueryID: query.ID,
			Text:            render.CallbackText,
		})
		return
	}
	if msg.Chat.Type == models.ChatTypePrivate {
		if _, err := a.tg.AnswerCallbackQuery(ctx, &tgb.AnswerCallbackQueryParams{
			CallbackQueryID: query.ID,
			Text:            render.CallbackText,
		}); err != nil {
			a.log.Errorln(err)
		}
		if err := a.processMusic(ctx, musicID, msg, nil, nil); err != nil {
			a.log.Errorln(err)
		}
		return
	}
	if _, err := a.tg.AnswerCallbackQuery(ctx, &tgb.AnswerCallbackQueryParams{
		CallbackQueryID: query.ID,
		Text:            render.CallbackText,
		URL:             fmt.Sprintf("t.me/%s?start=%d", a.me.Username, musicID),
	}); err != nil {
		a.log.Errorln(err)
	}
}
