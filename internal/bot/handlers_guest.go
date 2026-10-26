package bot

import (
	"context"
	"errors"
	"fmt"

	tgb "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/recognize"
	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/render"
	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/textx"
)

func (a *App) handleGuestRecognize(ctx context.Context, tg *tgb.Bot, update *models.Update) {
	m := update.GuestMessage
	if m == nil || m.GuestQueryID == "" {
		return
	}

	text := render.GuestReplyVoiceFirst
	var markup models.ReplyMarkup
	if m.ReplyToMessage != nil && m.ReplyToMessage.Voice != nil {
		audio, err := a.readVoice(ctx, m.ReplyToMessage.Voice)
		if err != nil {
			a.log.Errorln(err)
			text = render.RecognizeReadFailed
		} else {
			musicID, err := a.recog.Recognize(ctx, audio)
			switch {
			case errors.Is(err, recognize.ErrNoResult):
				text = render.RecognizeNoResult
			case err != nil:
				a.log.Errorln(err)
				text = render.RecognizeFailed
			default:
				text = render.RecognizeSuccess
				keyboard := songLinkKeyboard(musicID)
				keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, []models.InlineKeyboardButton{{
					Text: "在 Bot 中获取歌曲",
					URL:  fmt.Sprintf("https://t.me/%s?start=%d", a.me.Username, musicID),
				}})
				markup = keyboard
				if info, ok := a.store.GetByMusicID(musicID); ok {
					text = render.RecognizeResult(info.SongName, info.SongArtists, info.SongAlbum)
				} else if detail, err := a.api.SongDetail(musicID); err != nil {
					a.log.Errorln(err)
				} else if len(detail.Songs) != 0 {
					song := detail.Songs[0]
					text = render.RecognizeResult(song.Name, textx.JoinNames(artistNames(song)), song.Al.Name)
				}
			}
		}
	}

	disabled := true
	if _, err := tg.AnswerGuestQuery(ctx, &tgb.AnswerGuestQueryParams{
		GuestQueryID: m.GuestQueryID,
		Result: &models.InlineQueryResultArticle{
			ID:    "recognize",
			Title: "音乐识别",
			InputMessageContent: &models.InputTextMessageContent{
				MessageText:        text,
				ParseMode:          models.ParseModeHTML,
				LinkPreviewOptions: &models.LinkPreviewOptions{IsDisabled: &disabled},
			},
			ReplyMarkup: markup,
		},
	}); err != nil {
		a.log.Errorln(err)
	}
}
