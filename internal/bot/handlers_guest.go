package bot

import (
	"context"
	"errors"

	"github.com/XiaoMengXinX/Music163Api-Go/types"
	tgb "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/recognize"
	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/render"
	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/store"
	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/textx"
)

func (a *App) handleGuest(ctx context.Context, tg *tgb.Bot, update *models.Update) {
	m := update.GuestMessage
	if m == nil || m.GuestQueryID == "" {
		return
	}
	// 通过 inline 发送的消息也会触发访客模式
	if m.ViaBot != nil && m.ViaBot.ID == a.me.ID {
		return
	}

	var result models.InlineQueryResult
	if reply := m.ReplyToMessage; reply != nil && reply.Voice != nil {
		result = a.guestRecognize(ctx, reply.Voice)
	} else {
		result = a.guestMusic(ctx, m)
	}
	if _, err := tg.AnswerGuestQuery(ctx, &tgb.AnswerGuestQueryParams{
		GuestQueryID: m.GuestQueryID,
		Result:       result,
	}); err != nil {
		a.log.Errorln(err)
	}
}

// guestMusic 访客模式无法上传文件，未缓存的歌曲须先上传到缓存群
func (a *App) guestMusic(ctx context.Context, m *models.Message) models.InlineQueryResult {
	musicID, programID := musicFromText(m.Text)
	if musicID == 0 && programID == 0 && m.ReplyToMessage != nil {
		musicID, programID = messageMusic(m.ReplyToMessage)
	}
	musicID, prog, err := a.resolveMusic(musicID, programID)
	if err != nil {
		a.log.Errorln(err)
		return guestArticle("网易云音乐", render.FetchInfoFailed, nil)
	}
	if musicID == 0 {
		return guestArticle("使用帮助", render.GuestUsage, nil)
	}

	songInfo, ok := a.store.GetByMusicID(musicID)
	if chatID := a.cacheChat.Load(); !ok && chatID != 0 && a.cookieOK.Load() {
		if songInfo, err = a.cacheMusic(ctx, musicID, m, prog, chatID); err != nil {
			a.log.Errorln(err)
		}
	}
	if songInfo.FileID != "" {
		return a.cachedMusicResult("music", songInfo, prog)
	}
	text := render.GuestNoCache
	if songInfo.SongName != "" {
		text = render.GuestMusicResult(songInfo.SongName, songInfo.SongArtists, songInfo.SongAlbum)
	} else if detail, err := a.api.SongDetail(musicID); err != nil {
		a.log.Errorln(err)
	} else if len(detail.Songs) != 0 {
		song := detail.Songs[0]
		text = render.GuestMusicResult(song.Name, textx.JoinNames(artistNames(song)), song.Al.Name)
	}
	link := render.SongLink(musicID)
	if programID != 0 {
		link = render.ProgramLink(programID)
	}
	return guestArticle("网易云音乐", text, a.guestKeyboard(link, musicID))
}

func (a *App) cacheMusic(ctx context.Context, musicID int, m *models.Message, prog *types.ProgramDetailData, chatID int64) (store.SongInfo, error) {
	if err := a.musicSem.Acquire(ctx, 1); err != nil {
		return store.SongInfo{}, err
	}
	defer a.musicSem.Release(1)
	if songInfo, ok := a.store.GetByMusicID(musicID); ok {
		return songInfo, nil
	}
	return a.fetchMusic(ctx, musicID, m, prog, nil, chatID, nil, func(string) error { return nil })
}

func (a *App) guestRecognize(ctx context.Context, voice *models.Voice) models.InlineQueryResult {
	audio, err := a.readVoice(ctx, voice)
	if err != nil {
		a.log.Errorln(err)
		return guestArticle("音乐识别", render.RecognizeReadFailed, nil)
	}
	musicID, err := a.recog.Recognize(ctx, audio)
	switch {
	case errors.Is(err, recognize.ErrNoResult):
		return guestArticle("音乐识别", render.RecognizeNoResult, nil)
	case err != nil:
		a.log.Errorln(err)
		return guestArticle("音乐识别", render.RecognizeFailed, nil)
	}
	text := render.RecognizeSuccess
	if info, ok := a.store.GetByMusicID(musicID); ok {
		text = render.RecognizeResult(info.SongName, info.SongArtists, info.SongAlbum)
	} else if detail, err := a.api.SongDetail(musicID); err != nil {
		a.log.Errorln(err)
	} else if len(detail.Songs) != 0 {
		song := detail.Songs[0]
		text = render.RecognizeResult(song.Name, textx.JoinNames(artistNames(song)), song.Al.Name)
	}
	return guestArticle("音乐识别", text, a.guestKeyboard(render.SongLink(musicID), musicID))
}

func guestArticle(title, text string, markup models.ReplyMarkup) *models.InlineQueryResultArticle {
	disabled := true
	return &models.InlineQueryResultArticle{
		ID:    "guest",
		Title: title,
		InputMessageContent: &models.InputTextMessageContent{
			MessageText:        text,
			ParseMode:          models.ParseModeHTML,
			LinkPreviewOptions: &models.LinkPreviewOptions{IsDisabled: &disabled},
		},
		ReplyMarkup: markup,
	}
}
