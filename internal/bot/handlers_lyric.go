package bot

import (
	"bufio"
	"context"
	"os"
	"path/filepath"

	tgb "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/render"
	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/textx"
)

func (a *App) handleLyric(ctx context.Context, tg *tgb.Bot, update *models.Update) {
	m := update.Message
	source := m
	args := commandArguments(m)
	if args == "" {
		if m.ReplyToMessage == nil {
			if _, err := a.reply(ctx, m, render.InputContent); err != nil {
				a.log.Errorln(err)
			}
			return
		}
		source = m.ReplyToMessage
		args = source.Text
	}

	msgResult, err := a.reply(ctx, source, render.FetchingLyric)
	if err != nil {
		a.log.Errorln(err)
		return
	}
	sendFailed := func() {
		if err := a.editMessage(ctx, msgResult.Chat.ID, msgResult.ID, render.GetLrcFailed, nil); err != nil {
			a.log.Errorln(err)
		}
	}

	var musicID, programID int
	if source != m {
		musicID, programID = messageMusic(source)
	} else {
		musicID, programID = musicFromText(args)
	}
	if programID != 0 {
		if err := a.editMessage(ctx, msgResult.Chat.ID, msgResult.ID, render.LrcNoProgram, nil); err != nil {
			a.log.Errorln(err)
		}
		return
	}
	if musicID == 0 {
		musicID = textx.ParseMusicID(args)
	}
	if musicID == 0 {
		searchResult, err := a.api.SearchSong(args, 5)
		if err != nil {
			a.log.Errorln(err)
		}
		if len(searchResult.Result.Songs) == 0 {
			if err := a.editMessage(ctx, msgResult.Chat.ID, msgResult.ID, render.NoResults, nil); err != nil {
				a.log.Errorln(err)
			}
			return
		}
		musicID = searchResult.Result.Songs[0].Id
	}

	lyric, detail, err := a.api.LyricAndDetail(musicID)
	if err != nil {
		a.log.Errorln(err)
		sendFailed()
		return
	}
	if lyric.Lrc.Lyric == "" || len(detail.Songs) == 0 {
		sendFailed()
		return
	}

	song := detail.Songs[0]
	lrcPath := filepath.Join(a.cacheDir, songFileName(textx.JoinNames(artistNames(song)), song.Name, "lrc"))
	file, err := os.OpenFile(lrcPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o666)
	if err != nil {
		sendFailed()
		a.log.Errorln(err)
		return
	}
	defer os.Remove(lrcPath)
	write := bufio.NewWriter(file)
	_, _ = write.WriteString(lyric.Lrc.Lyric)
	if err := write.Flush(); err != nil {
		_ = file.Close()
		sendFailed()
		a.log.Errorln(err)
		return
	}
	if err := file.Close(); err != nil {
		sendFailed()
		a.log.Errorln(err)
		return
	}

	lrcFile, err := os.Open(lrcPath)
	if err != nil {
		sendFailed()
		a.log.Errorln(err)
		return
	}
	defer lrcFile.Close()
	params := &tgb.SendDocumentParams{
		ChatID:          source.Chat.ID,
		Document:        &models.InputFileUpload{Filename: filepath.Base(lrcPath), Data: lrcFile},
		ReplyParameters: &models.ReplyParameters{MessageID: source.ID},
	}
	if text := textx.LrcText(lyric.Lrc.Lyric); text != "" {
		params.Caption = render.LyricCaption(text)
		params.ParseMode = models.ParseModeHTML
	}
	if _, err := a.tg.SendDocument(ctx, params); err != nil {
		a.log.Errorln(err)
		return
	}
	if err := a.deleteMessage(ctx, msgResult.Chat.ID, msgResult.ID); err != nil {
		a.log.Errorln(err)
	}
}
