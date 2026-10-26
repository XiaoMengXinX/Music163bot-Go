package bot

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	tgb "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/recognize"
	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/render"
	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/store"
)

func (a *App) handleRecognize(ctx context.Context, tg *tgb.Bot, update *models.Update) {
	m := update.Message
	if m.ReplyToMessage == nil {
		if _, err := a.reply(ctx, m, render.ReplyVoiceFirst); err != nil {
			a.log.Errorln(err)
		}
		return
	}
	voice := m.ReplyToMessage
	if voice.Voice == nil {
		if _, err := a.reply(ctx, voice, render.ReplyVoiceFirst); err != nil {
			a.log.Errorln(err)
		}
		return
	}

	status, err := a.reply(ctx, voice, render.Recognizing)
	if err != nil {
		a.log.Errorln(err)
		return
	}
	setStatus := func(text string) {
		if err := a.editMessage(ctx, voice.Chat.ID, status.ID, text, nil); err != nil {
			a.log.Errorln(err)
		}
	}

	audio, err := a.readVoice(ctx, voice.Voice)
	if err != nil {
		a.log.Errorln(err)
		setStatus(render.RecognizeReadFailed)
		return
	}
	musicID, err := a.recog.Recognize(ctx, audio)
	if err != nil {
		if errors.Is(err, recognize.ErrNoResult) {
			setStatus(render.RecognizeNoResult)
		} else {
			a.log.Errorln(err)
			setStatus(render.RecognizeFailed)
		}
		return
	}

	keyboard := songLinkKeyboard(musicID)
	showResult := func(text string) {
		if err := a.editMessage(ctx, voice.Chat.ID, status.ID, text, keyboard); err != nil {
			a.log.Errorln(err)
		}
	}
	shown := false
	if err := a.processMusic(ctx, musicID, voice, nil, func(info store.SongInfo) {
		showResult(render.RecognizeResult(info.SongName, info.SongArtists, info.SongAlbum))
		shown = true
	}); err != nil {
		a.log.Errorln(err)
	}
	if !shown {
		showResult(render.RecognizeSuccess)
	}
}

func (a *App) readVoice(ctx context.Context, voice *models.Voice) ([]byte, error) {
	file, err := a.tg.GetFile(ctx, &tgb.GetFileParams{FileID: voice.FileID})
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp(a.cacheDir, "recognize-")
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			a.log.Errorln(err)
		}
	}()
	voicePath := filepath.Join(dir, "voice.ogg")
	if err := a.downloadVoice(ctx, voicePath, file); err != nil {
		return nil, err
	}
	return os.ReadFile(voicePath)
}

func (a *App) downloadVoice(ctx context.Context, dstPath string, file *models.File) error {
	var source io.Reader
	if a.cfg.Bot.LocalMode {
		if !filepath.IsAbs(file.FilePath) {
			return fmt.Errorf("本地模式需要 Bot API 返回绝对文件路径，请确认服务已启用 --local")
		}
		localFile, err := os.Open(file.FilePath)
		if err != nil {
			return fmt.Errorf("读取 Bot API 本地文件失败，请检查共享目录和读取权限: %w", err)
		}
		defer localFile.Close()
		info, err := localFile.Stat()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("Bot API 本地路径不是普通文件")
		}
		source = localFile
	} else {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.tg.FileDownloadLink(file), nil)
		if err != nil {
			return err
		}
		resp, err := a.http.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("下载语音失败: %s", resp.Status)
		}
		source = resp.Body
	}
	out, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, source); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
