package bot

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/XiaoMengXinX/Music163Api-Go/types"
	tgb "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/fetch"
	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/render"
	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/store"
	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/tagging"
	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/textx"
)

func (a *App) handleStart(ctx context.Context, tg *tgb.Bot, update *models.Update) {
	m := update.Message
	if m.Chat.Type != models.ChatTypePrivate {
		return
	}
	musicID, _ := strconv.Atoi(commandArguments(m))
	if musicID == 0 {
		return
	}
	if err := a.processMusic(ctx, musicID, m, nil, nil); err != nil {
		a.log.Errorln(err)
	}
}

func (a *App) handleMusic(ctx context.Context, tg *tgb.Bot, update *models.Update) {
	m := update.Message
	args := commandArguments(m)
	if args == "" {
		if _, err := a.reply(ctx, m, render.InputIDorKeyword); err != nil {
			a.log.Errorln(err)
		}
		return
	}
	if musicID, _ := strconv.Atoi(args); musicID != 0 {
		if err := a.processMusic(ctx, musicID, m, nil, nil); err != nil {
			a.log.Errorln(err)
		}
		return
	}
	result, err := a.api.SearchSong(args, 10)
	if err != nil {
		a.log.Errorln(err)
	}
	if len(result.Result.Songs) == 0 {
		if _, err := a.reply(ctx, m, render.NoResults); err != nil {
			a.log.Errorln(err)
		}
		return
	}
	if err := a.processMusic(ctx, result.Result.Songs[0].Id, m, nil, nil); err != nil {
		a.log.Errorln(err)
	}
}

func (a *App) handleProgram(ctx context.Context, tg *tgb.Bot, update *models.Update) {
	m := update.Message
	id, _ := strconv.Atoi(commandArguments(m))
	prog, err := a.api.ProgramDetail(id)
	if err != nil {
		a.log.Errorln(err)
		return
	}
	musicID := prog.Program.MainSong.ID
	if musicID == 0 {
		return
	}
	if err := a.processMusic(ctx, musicID, m, &prog, nil); err != nil {
		a.log.Errorln(err)
	}
}

func (a *App) handleDefault(ctx context.Context, tg *tgb.Bot, update *models.Update) {
	m := update.Message
	if m == nil {
		return
	}
	text := m.Text
	if !textx.ContainsMusicLink(text) {
		return
	}
	if textx.ContainsShortLink(text) {
		text = textx.ResolveShortLink(text)
	}
	if !textx.ContainsSongLink(text) {
		return
	}
	if id := textx.ParseMusicID(text); id != 0 {
		if err := a.processMusic(ctx, id, m, nil, nil); err != nil {
			a.log.Errorln(err)
		}
		return
	}
	if pid := textx.ParseProgramID(text); pid != 0 {
		prog, err := a.api.ProgramDetail(pid)
		if err != nil {
			a.log.Errorln(err)
			return
		}
		if rid := prog.Program.MainSong.ID; rid != 0 {
			if err := a.processMusic(ctx, rid, m, &prog, nil); err != nil {
				a.log.Errorln(err)
			}
		}
	}
}

// prog 非 nil 时用节目详情补全元数据
func (a *App) processMusic(ctx context.Context, musicID int, message *models.Message, prog *types.ProgramDetailData, onInfo func(store.SongInfo)) error {
	chatID := message.Chat.ID

	if songInfo, ok := a.store.GetByMusicID(musicID); ok {
		if onInfo != nil {
			onInfo(songInfo)
		}
		msgResult, err := a.reply(ctx, message, render.MusicStatus(songInfo.SongName, songInfo.SongAlbum, songInfo.FileExt, float64(songInfo.MusicSize)/1024/1024, render.HitCache))
		if err != nil {
			return err
		}
		if _, err := a.sendMusic(ctx, songInfo, "", "", message); err != nil {
			a.sendDownloadFailed(ctx, chatID, msgResult.ID, songInfo, err)
			return err
		}
		return a.deleteMessage(ctx, chatID, msgResult.ID)
	}

	// Cookie 失效不影响缓存发送
	if !a.cookieOK.Load() {
		if _, err := a.reply(ctx, message, render.CookieExpired); err != nil {
			a.log.Errorln(err)
		}
		return nil
	}

	msgResult, err := a.reply(ctx, message, render.WaitForDown)
	if err != nil {
		return err
	}

	if err := a.musicSem.Acquire(ctx, 1); err != nil {
		return err
	}
	defer a.musicSem.Release(1)

	// 等待下载位时缓存可能已被其他任务填充
	if songInfo, ok := a.store.GetByMusicID(musicID); ok {
		if onInfo != nil {
			onInfo(songInfo)
		}
		if err := a.editMessage(ctx, chatID, msgResult.ID, render.MusicStatus(songInfo.SongName, songInfo.SongAlbum, songInfo.FileExt, float64(songInfo.MusicSize)/1024/1024, render.HitCache), nil); err != nil {
			return err
		}
		if _, err := a.sendMusic(ctx, songInfo, "", "", message); err != nil {
			a.sendDownloadFailed(ctx, chatID, msgResult.ID, songInfo, err)
			return err
		}
		return a.deleteMessage(ctx, chatID, msgResult.ID)
	}

	if err := a.editMessage(ctx, chatID, msgResult.ID, render.FetchInfo, nil); err != nil {
		return err
	}

	detail, urlData, err := a.api.SongDetailAndURL(musicID)
	if err != nil {
		a.log.Errorln(err)
		return a.editMessage(ctx, chatID, msgResult.ID, render.FetchInfoFailed, nil)
	}
	if prog != nil {
		enrichFromProgram(&detail, prog)
	}
	if len(detail.Songs) == 0 {
		return a.editMessage(ctx, chatID, msgResult.ID, render.FetchInfoFailed, nil)
	}

	var songInfo store.SongInfo
	songInfo.FromChatID = message.Chat.ID
	if message.Chat.Type == models.ChatTypePrivate {
		songInfo.FromChatName = message.Chat.Username
	} else {
		songInfo.FromChatName = message.Chat.Title
	}
	songInfo.FromUserID = message.From.ID
	songInfo.FromUserName = message.From.Username
	songInfo.MusicID = musicID
	songInfo.Duration = detail.Songs[0].Dt / 1000
	songInfo.SongName = detail.Songs[0].Name
	songInfo.SongArtists = textx.JoinNames(artistNames(detail.Songs[0]))
	songInfo.SongAlbum = detail.Songs[0].Al.Name
	if onInfo != nil {
		onInfo(songInfo)
	}
	if len(urlData.Data) == 0 || urlData.Data[0].Url == "" {
		return a.editMessage(ctx, chatID, msgResult.ID, render.GetURLFailed, nil)
	}
	switch path.Ext(textx.URLBase(urlData.Data[0].Url)) {
	case ".mp3":
		songInfo.FileExt = "mp3"
	case ".flac":
		songInfo.FileExt = "flac"
	default:
		songInfo.FileExt = "mp3"
	}
	songInfo.MusicSize = urlData.Data[0].Size
	if songInfo.Duration > 0 {
		songInfo.BitRate = 8 * urlData.Data[0].Size / songInfo.Duration
	}
	if picRes, err := a.http.Head(detail.Songs[0].Al.PicUrl); err == nil {
		_ = picRes.Body.Close()
		songInfo.PicSize = int(picRes.ContentLength)
	} else {
		a.log.Errorln(err)
	}

	if err := a.editMessage(ctx, chatID, msgResult.ID, render.MusicStatus(songInfo.SongName, songInfo.SongAlbum, songInfo.FileExt, float64(songInfo.MusicSize)/1024/1024, render.Downloading), nil); err != nil {
		return err
	}

	timeStamp := time.Now().UnixMicro()
	musicFileName := fmt.Sprintf("%d-%s", timeStamp, textx.URLBase(urlData.Data[0].Url))
	musicPath := filepath.Join(a.cacheDir, musicFileName)

	var lastUpdateTime int64
	onProgress := func(p fetch.Progress) {
		if time.Now().Unix()-lastUpdateTime < 5 {
			return
		}
		statusText := render.Downloading
		if p.Attempt > 1 && p.ViaProxy {
			statusText = render.Redownloading
		}
		if err := a.editMessage(ctx, chatID, msgResult.ID, render.MusicStatus(songInfo.SongName, songInfo.SongAlbum, songInfo.FileExt, float64(songInfo.MusicSize)/1024/1024,
			statusText+render.DownloadStatus(p.Speed, float64(p.Written)/1024/1024, float64(p.Total)/1024/1024, p.Written*100/p.Total)), nil); err != nil {
			a.log.Errorln(err)
		}
		lastUpdateTime = time.Now().Unix()
	}

	if err := a.dl.DownloadMusic(ctx, urlData.Data[0].Url, musicFileName, onProgress); err != nil {
		a.sendDownloadFailed(ctx, chatID, msgResult.ID, songInfo, err)
		return err
	}

	if md5sum := urlData.Data[0].Md5; md5sum != "" {
		if err := fetch.VerifyMD5(musicPath, md5sum); err != nil {
			a.log.Errorln(err)
			if rmErr := os.Remove(musicPath); rmErr != nil {
				a.log.Errorln(rmErr)
			}
			a.sendDownloadFailed(ctx, chatID, msgResult.ID, songInfo, fmt.Errorf("%s\n%s", render.Md5VerFailed, render.RetryLater))
			return nil
		}
	}

	// 封面失败不阻断发歌
	var picPath, resizePicPath string
	if picURL := detail.Songs[0].Al.PicUrl; picURL != "" {
		picName := fmt.Sprintf("%d-%s", timeStamp, textx.URLBase(picURL))
		if err := a.dl.DownloadFile(picURL, picName); err != nil {
			a.log.Errorln(err)
		} else {
			picPath = filepath.Join(a.cacheDir, picName)
			resized, err := tagging.ResizeCover(picPath)
			if err != nil {
				a.log.Errorln(err)
			} else {
				resizePicPath = resized
			}
		}
	}

	var musicPic string
	if picStat, err := os.Stat(picPath); err == nil {
		if picStat.Size() > 2*1024*1024 {
			musicPic = resizePicPath
			if embStat, err := os.Stat(resizePicPath); err == nil {
				songInfo.EmbPicSize = int(embStat.Size())
			}
		} else {
			musicPic = picPath
			songInfo.EmbPicSize = songInfo.PicSize
		}
	} else {
		a.log.Errorln(err)
	}

	newDir := filepath.Join(a.cacheDir, strconv.FormatInt(timeStamp, 10))
	if err := os.Mkdir(newDir, os.ModePerm); err != nil {
		a.sendDownloadFailed(ctx, chatID, msgResult.ID, songInfo, err)
		return err
	}
	fileName := textx.SanitizeFilename(fmt.Sprintf("%v - %v.%v", strings.ReplaceAll(songInfo.SongArtists, "/", ","), songInfo.SongName, songInfo.FileExt))
	filePath := filepath.Join(newDir, fileName)
	if err := os.Rename(musicPath, filePath); err != nil {
		a.log.Errorln(err)
		filePath = musicPath
	}
	defer func() {
		for _, f := range []string{filePath, resizePicPath, picPath} {
			if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
				a.log.Errorln(err)
			}
		}
		if err := os.RemoveAll(newDir); err != nil {
			a.log.Errorln(err)
		}
	}()

	if err := tagging.Write(filePath, musicPic, detail.Songs[0], urlData.Data[0]); err != nil {
		a.sendDownloadFailed(ctx, chatID, msgResult.ID, songInfo, err)
		return err
	}

	if err := a.editMessage(ctx, chatID, msgResult.ID, render.MusicStatus(songInfo.SongName, songInfo.SongAlbum, songInfo.FileExt, float64(songInfo.MusicSize)/1024/1024, render.Uploading), nil); err != nil {
		return err
	}

	audio, err := a.sendMusic(ctx, songInfo, filePath, resizePicPath, message)
	if err != nil {
		a.sendDownloadFailed(ctx, chatID, msgResult.ID, songInfo, err)
		return err
	}

	songInfo.FileID = audio.Audio.FileID
	if audio.Audio.Thumbnail != nil {
		songInfo.ThumbFileID = audio.Audio.Thumbnail.FileID
	}
	if err := a.store.Save(&songInfo); err != nil {
		return err
	}

	return a.deleteMessage(ctx, chatID, msgResult.ID)
}

func artistNames(song types.SongDetailData) []string {
	names := make([]string, 0, len(song.Ar))
	for _, ar := range song.Ar {
		names = append(names, ar.Name)
	}
	return names
}

// 错误展示前须隐藏 bot token
func (a *App) sendDownloadFailed(ctx context.Context, chatID int64, messageID int, songInfo store.SongInfo, err error) {
	scrubbed := strings.ReplaceAll(err.Error(), a.cfg.Bot.Token, "BOT_TOKEN")
	text := render.MusicStatus(songInfo.SongName, songInfo.SongAlbum, songInfo.FileExt, float64(songInfo.MusicSize)/1024/1024, render.DownloadFailed(scrubbed))
	if editErr := a.editMessage(ctx, chatID, messageID, text, nil); editErr != nil {
		a.log.Errorln(editErr)
	}
}

func (a *App) sendMusic(ctx context.Context, songInfo store.SongInfo, musicPath, picPath string, message *models.Message) (*models.Message, error) {
	params := &tgb.SendAudioParams{
		ChatID:          message.Chat.ID,
		Caption:         render.MusicCaption(songInfo.SongName, songInfo.SongArtists, songInfo.SongAlbum, songInfo.FileExt, float64(songInfo.MusicSize+songInfo.EmbPicSize)/1024/1024, float64(songInfo.BitRate)/1000, a.me.Username),
		ParseMode:       models.ParseModeHTML,
		Title:           songInfo.SongName,
		Performer:       songInfo.SongArtists,
		Duration:        songInfo.Duration,
		ReplyMarkup:     musicKeyboard(songInfo.SongName, songInfo.SongArtists, songInfo.MusicID),
		ReplyParameters: &models.ReplyParameters{MessageID: message.ID, AllowSendingWithoutReply: true},
	}
	if songInfo.FileID != "" {
		params.Audio = &models.InputFileString{Data: songInfo.FileID}
	} else {
		file, err := os.Open(musicPath)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		params.Audio = &models.InputFileUpload{Filename: filepath.Base(musicPath), Data: file}
		_, _ = a.tg.SendChatAction(ctx, &tgb.SendChatActionParams{
			ChatID: message.Chat.ID,
			Action: models.ChatActionUploadDocument,
		})
	}
	if songInfo.ThumbFileID != "" {
		params.Thumbnail = &models.InputFileString{Data: songInfo.ThumbFileID}
	}
	if picPath != "" {
		picFile, err := os.Open(picPath)
		if err != nil {
			return nil, err
		}
		defer picFile.Close()
		params.Thumbnail = &models.InputFileUpload{Filename: filepath.Base(picPath), Data: picFile}
	}
	return a.tg.SendAudio(ctx, params)
}
