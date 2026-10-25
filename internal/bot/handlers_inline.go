package bot

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tgb "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/render"
	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/textx"
)

// handleInline inline 查询分发：help / search+关键词 / 链接或 ID
func (a *App) handleInline(ctx context.Context, tg *tgb.Bot, update *models.Update) {
	query := update.InlineQuery
	var err error
	switch {
	case query.Query == "help":
		err = a.answerInlineHelp(ctx, query)
	case strings.Contains(query.Query, "search"):
		err = a.answerInlineSearch(ctx, query)
	default:
		if musicID, _ := strconv.Atoi(textx.LinkTestMusic(query.Query)); musicID != 0 {
			err = a.answerInlineMusic(ctx, query, musicID)
		} else {
			err = a.answerInlineEmpty(ctx, query)
		}
	}
	if err != nil {
		a.log.Errorln(err)
	}
}

func (a *App) answerInline(ctx context.Context, queryID string, results []models.InlineQueryResult, cacheTime int, button *models.InlineQueryResultsButton) error {
	_, err := a.tg.AnswerInlineQuery(ctx, &tgb.AnswerInlineQueryParams{
		InlineQueryID: queryID,
		Results:       results,
		CacheTime:     cacheTime,
		Button:        button,
	})
	return err
}

// answerInlineMusic 已缓存的歌曲直接回音频，未缓存的引导进私聊缓存
func (a *App) answerInlineMusic(ctx context.Context, query *models.InlineQuery, musicID int) error {
	songInfo, ok := a.store.GetByMusicID(musicID)
	if ok && songInfo.FileID != "" && songInfo.SongName != "" {
		return a.answerInline(ctx, query.ID, []models.InlineQueryResult{
			&models.InlineQueryResultCachedDocument{
				ID:             query.ID,
				DocumentFileID: songInfo.FileID,
				Title:          fmt.Sprintf("%s - %s", songInfo.SongArtists, songInfo.SongName),
				Description:    songInfo.SongAlbum,
				Caption: render.MusicCaption(songInfo.SongName, songInfo.SongArtists, songInfo.SongAlbum, songInfo.FileExt,
					float64(songInfo.MusicSize+songInfo.EmbPicSize)/1024/1024, float64(songInfo.BitRate)/1000, a.me.Username),
				ParseMode:   models.ParseModeHTML,
				ReplyMarkup: musicKeyboard(songInfo.SongName, songInfo.SongArtists, songInfo.MusicID),
			},
		}, 3600, nil)
	}
	// 有缓存记录但无 FileID 时也走未缓存引导
	return a.answerInline(ctx, query.ID, []models.InlineQueryResult{
		&models.InlineQueryResultArticle{
			ID:                  query.ID,
			Title:               render.NoCache,
			Description:         render.TapToDownload,
			InputMessageContent: &models.InputTextMessageContent{MessageText: query.Query},
		},
	}, 60, &models.InlineQueryResultsButton{
		Text:           render.TapMeToDown,
		StartParameter: strconv.Itoa(musicID),
	})
}

func (a *App) answerInlineHelp(ctx context.Context, query *models.InlineQuery) error {
	randomID := time.Now().UnixMicro()
	return a.answerInline(ctx, query.ID, []models.InlineQueryResult{
		&models.InlineQueryResultArticle{
			ID:                  fmt.Sprintf("%d", randomID),
			Title:               render.InlineHelpStep1,
			InputMessageContent: &models.InputTextMessageContent{MessageText: render.BotName},
		},
		&models.InlineQueryResultArticle{
			ID:                  fmt.Sprintf("%d", randomID+1),
			Title:               render.InlineHelpStep2,
			InputMessageContent: &models.InputTextMessageContent{MessageText: render.BotName},
		},
	}, 3600, nil)
}

func (a *App) answerInlineSearch(ctx context.Context, query *models.InlineQuery) error {
	randomID := time.Now().UnixMicro()
	keyword := strings.Replace(query.Query, "search", "", 1)
	if keyword == "" {
		return a.answerInline(ctx, query.ID, []models.InlineQueryResult{
			&models.InlineQueryResultArticle{
				ID:                  fmt.Sprintf("%d", randomID),
				Title:               render.InlineInputKeyword,
				InputMessageContent: &models.InputTextMessageContent{MessageText: render.BotName},
			},
		}, 3600, nil)
	}
	result, err := a.api.SearchSong(keyword, 0)
	if err != nil {
		return err
	}
	if len(result.Result.Songs) == 0 {
		return a.answerInline(ctx, query.ID, []models.InlineQueryResult{
			&models.InlineQueryResultArticle{
				ID:                  fmt.Sprintf("%d", randomID),
				Title:               render.NoResults,
				InputMessageContent: &models.InputTextMessageContent{MessageText: render.NoResults},
			},
		}, 3600, nil)
	}
	var results []models.InlineQueryResult
	for i := 0; i < len(result.Result.Songs) && i < 10; i++ {
		song := result.Result.Songs[i]
		var names []string
		for _, artist := range song.Artists {
			names = append(names, artist.Name)
		}
		results = append(results, &models.InlineQueryResultArticle{
			ID:                  fmt.Sprintf("%d", randomID+int64(i)),
			Title:               song.Name,
			Description:         textx.JoinNames(names),
			InputMessageContent: &models.InputTextMessageContent{MessageText: fmt.Sprintf("/netease %d", song.Id)},
		})
	}
	return a.answerInline(ctx, query.ID, results, 3600, nil)
}

func (a *App) answerInlineEmpty(ctx context.Context, query *models.InlineQuery) error {
	return a.answerInline(ctx, query.ID, []models.InlineQueryResult{
		&models.InlineQueryResultArticle{
			ID:                  query.ID,
			Title:               render.InlineHelpHint,
			InputMessageContent: &models.InputTextMessageContent{MessageText: render.BotName},
		},
	}, 3600, nil)
}
