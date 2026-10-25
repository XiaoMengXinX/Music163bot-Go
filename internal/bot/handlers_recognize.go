package bot

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	tgb "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/recognize"
	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/render"
)

// handleRecognize /recognize：识别回复的语音留言并发歌
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

	file, err := a.tg.GetFile(ctx, &tgb.GetFileParams{FileID: voice.Voice.FileID})
	if err != nil {
		a.log.Errorln(err)
		return
	}
	voiceURL := a.tg.FileDownloadLink(file)

	oggPath := filepath.Join(a.cacheDir, fmt.Sprintf("%d-%d-%d.ogg", voice.Chat.ID, voice.ID, time.Now().Unix()))
	mp3Path := oggPath + ".mp3"
	defer func() {
		for _, f := range []string{oggPath, mp3Path} {
			if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
				a.log.Errorln(err)
			}
		}
	}()

	if err := a.downloadVoice(oggPath, voiceURL); err != nil {
		a.log.Errorln(err)
		return
	}
	if err := recognize.ToMp3(oggPath, mp3Path); err != nil {
		a.log.Errorln(err)
		return
	}
	audio, err := os.ReadFile(mp3Path)
	if err != nil {
		a.log.Errorln(err)
		return
	}
	musicID, err := a.recog.Recognize(audio)
	if err != nil {
		if _, sendErr := a.reply(ctx, voice, render.RecognizeFailed); sendErr != nil {
			a.log.Errorln(sendErr)
		}
		if !errors.Is(err, recognize.ErrNoResult) {
			a.log.Errorln(err)
		}
		return
	}

	if _, err := a.reply(ctx, voice, render.SongLink(musicID)); err != nil {
		a.log.Errorln(err)
	}
	if err := a.processMusic(ctx, musicID, voice, nil); err != nil {
		a.log.Errorln(err)
	}
}

// downloadVoice 下载语音文件到本地，确保响应体与文件句柄关闭
func (a *App) downloadVoice(dstPath, url string) error {
	resp, err := a.http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载语音失败: %s", resp.Status)
	}
	file, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	if _, err := io.Copy(file, resp.Body); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}
