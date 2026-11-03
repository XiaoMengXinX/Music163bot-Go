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
	result, err := a.api.SearchSong(args, 10, 0)
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
	musicID, prog, err := a.resolveMusic(musicFromText(m.Text))
	if err != nil {
		a.log.Errorln(err)
		return
	}
	if musicID == 0 {
		return
	}
	if err := a.processMusic(ctx, musicID, m, prog, nil); err != nil {
		a.log.Errorln(err)
	}
}

// musicFromText 解析分享链接中的歌曲或电台节目 ID
func musicFromText(text string) (musicID, programID int) {
	if !textx.ContainsMusicLink(text) {
		return 0, 0
	}
	if textx.ContainsShortLink(text) {
		text = textx.ResolveShortLink(text)
	}
	if !textx.ContainsSongLink(text) {
		return 0, 0
	}
	if musicID = textx.ParseMusicID(text); musicID != 0 {
		return musicID, 0
	}
	return 0, textx.ParseProgramID(text)
}

// resolveMusic 电台节目返回主歌曲 ID 及节目详情
func (a *App) resolveMusic(musicID, programID int) (int, *types.ProgramDetailData, error) {
	if musicID != 0 || programID == 0 {
		return musicID, nil, nil
	}
	prog, err := a.api.ProgramDetail(programID)
	if err != nil {
		return 0, nil, err
	}
	return prog.Program.MainSong.ID, &prog, nil
}

// messageMusic 依次从正文、说明、文字链接和按钮中解析分享链接
func messageMusic(m *models.Message) (musicID, programID int) {
	links := []string{m.Text, m.Caption}
	for _, entities := range [][]models.MessageEntity{m.Entities, m.CaptionEntities} {
		for _, e := range entities {
			if e.Type == models.MessageEntityTypeTextLink {
				links = append(links, e.URL)
			}
		}
	}
	if m.ReplyMarkup != nil {
		for _, row := range m.ReplyMarkup.InlineKeyboard {
			for _, button := range row {
				if button.URL != "" {
					links = append(links, button.URL)
				}
			}
		}
	}
	for _, link := range links {
		if musicID, programID = musicFromText(link); musicID != 0 || programID != 0 {
			return musicID, programID
		}
	}
	return 0, 0
}

// prog 非 nil 时用节目详情补全元数据
func (a *App) processMusic(ctx context.Context, musicID int, message *models.Message, prog *types.ProgramDetailData, onInfo func(store.SongInfo)) error {
	chatID := message.Chat.ID
	reply := &models.ReplyParameters{MessageID: message.ID, AllowSendingWithoutReply: true}

	if songInfo, ok := a.store.GetByMusicID(musicID); ok {
		if onInfo != nil {
			onInfo(songInfo)
		}
		msgResult, err := a.reply(ctx, message, render.MusicStatus(songInfo.SongName, songInfo.SongAlbum, songInfo.FileExt, float64(songInfo.MusicSize)/1024/1024, render.HitCache))
		if err != nil {
			return err
		}
		if _, err := a.sendMusic(ctx, songInfo, prog, "", "", chatID, reply); err != nil {
			a.showDownloadFailed(func(text string) error { return a.editMessage(ctx, chatID, msgResult.ID, text, nil) }, songInfo, render.SendFailed)
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
	status := func(text string) error { return a.editMessage(ctx, chatID, msgResult.ID, text, nil) }

	if err := a.musicSem.Acquire(ctx, 1); err != nil {
		return err
	}
	defer a.musicSem.Release(1)

	// 等待下载位时缓存可能已被其他任务填充
	if songInfo, ok := a.store.GetByMusicID(musicID); ok {
		if onInfo != nil {
			onInfo(songInfo)
		}
		if err := status(render.MusicStatus(songInfo.SongName, songInfo.SongAlbum, songInfo.FileExt, float64(songInfo.MusicSize)/1024/1024, render.HitCache)); err != nil {
			return err
		}
		if _, err := a.sendMusic(ctx, songInfo, prog, "", "", chatID, reply); err != nil {
			a.showDownloadFailed(status, songInfo, render.SendFailed)
			return err
		}
		return a.deleteMessage(ctx, chatID, msgResult.ID)
	}

	if _, err := a.fetchMusic(ctx, musicID, message, prog, onInfo, chatID, reply, status); err != nil {
		return err
	}
	return a.deleteMessage(ctx, chatID, msgResult.ID)
}

// fetchMusic 下载歌曲上传到 chatID 并写入缓存，失败原因已经由 status 展示
func (a *App) fetchMusic(ctx context.Context, musicID int, message *models.Message, prog *types.ProgramDetailData, onInfo func(store.SongInfo),
	chatID int64, reply *models.ReplyParameters, status func(string) error) (store.SongInfo, error) {
	var songInfo store.SongInfo
	if err := status(render.FetchInfo); err != nil {
		return songInfo, err
	}

	detail, urlData, err := a.api.SongDetailAndURL(musicID)
	if err != nil {
		a.showStatus(status, render.FetchInfoFailed)
		return songInfo, err
	}
	if prog != nil {
		enrichFromProgram(&detail, prog)
	}
	if len(detail.Songs) == 0 {
		a.showStatus(status, render.FetchInfoFailed)
		return songInfo, fmt.Errorf("%s: music %d", render.FetchInfoFailed, musicID)
	}

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
		a.showStatus(status, render.GetURLFailed)
		code, fee := urlData.Code, 0
		if len(urlData.Data) != 0 {
			code, fee = urlData.Data[0].Code, urlData.Data[0].Fee
		}
		return songInfo, fmt.Errorf("%s: music %d, code %d, fee %d", render.GetURLFailed, musicID, code, fee)
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
	if limitMB := a.uploadLimitMB(); songInfo.MusicSize > limitMB<<20 {
		reason := render.MusicTooLarge(limitMB)
		a.showStatus(status, render.MusicStatus(songInfo.SongName, songInfo.SongAlbum, songInfo.FileExt, float64(songInfo.MusicSize)/1024/1024, reason))
		return songInfo, fmt.Errorf("%s: music %d, size %d", reason, musicID, songInfo.MusicSize)
	}

	if err := status(render.MusicStatus(songInfo.SongName, songInfo.SongAlbum, songInfo.FileExt, float64(songInfo.MusicSize)/1024/1024, render.Downloading)); err != nil {
		return songInfo, err
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
		if err := status(render.MusicStatus(songInfo.SongName, songInfo.SongAlbum, songInfo.FileExt, float64(songInfo.MusicSize)/1024/1024,
			statusText+render.DownloadStatus(p.Speed, float64(p.Written)/1024/1024, float64(p.Total)/1024/1024, p.Written*100/p.Total))); err != nil {
			a.log.Errorln(err)
		}
		lastUpdateTime = time.Now().Unix()
	}

	if err := a.dl.DownloadMusic(ctx, urlData.Data[0].Url, musicFileName, onProgress); err != nil {
		a.showDownloadFailed(status, songInfo, render.DownloadFailed)
		return songInfo, err
	}

	filePath := musicPath
	var picPath, resizePicPath, newDir string
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

	if md5sum := urlData.Data[0].Md5; md5sum != "" {
		if err := fetch.VerifyMD5(musicPath, md5sum); err != nil {
			a.showDownloadFailed(status, songInfo, render.Md5VerFailed)
			return songInfo, err
		}
	}

	// 封面失败不阻断发歌
	var musicPic string
	if picURL := detail.Songs[0].Al.PicUrl; picURL != "" {
		picName := fmt.Sprintf("%d-%s", timeStamp, textx.URLBase(picURL))
		if err := a.dl.DownloadFile(picURL, picName); err != nil {
			a.log.Errorln(err)
		} else {
			picPath = filepath.Join(a.cacheDir, picName)
			if resizePicPath, err = tagging.ResizeCover(picPath); err != nil {
				a.log.Errorln(err)
			}
			if picStat, err := os.Stat(picPath); err != nil {
				a.log.Errorln(err)
			} else {
				songInfo.PicSize = int(picStat.Size())
				if picStat.Size() > 2*1024*1024 {
					musicPic = resizePicPath
					if embStat, err := os.Stat(resizePicPath); err == nil {
						songInfo.EmbPicSize = int(embStat.Size())
					}
				} else {
					musicPic = picPath
					songInfo.EmbPicSize = songInfo.PicSize
				}
			}
		}
	}

	newDir = filepath.Join(a.cacheDir, strconv.FormatInt(timeStamp, 10))
	if err := os.Mkdir(newDir, os.ModePerm); err != nil {
		a.showDownloadFailed(status, songInfo, render.ProcessFailed)
		return songInfo, err
	}
	renamed := filepath.Join(newDir, songFileName(songInfo.SongArtists, songInfo.SongName, songInfo.FileExt))
	if err := os.Rename(musicPath, renamed); err != nil {
		a.log.Errorln(err)
	} else {
		filePath = renamed
	}

	if err := tagging.Write(filePath, musicPic, detail.Songs[0], urlData.Data[0]); err != nil {
		a.showDownloadFailed(status, songInfo, render.ProcessFailed)
		return songInfo, err
	}

	if err := status(render.MusicStatus(songInfo.SongName, songInfo.SongAlbum, songInfo.FileExt, float64(songInfo.MusicSize)/1024/1024, render.Uploading)); err != nil {
		return songInfo, err
	}

	audio, err := a.sendMusic(ctx, songInfo, prog, filePath, resizePicPath, chatID, reply)
	if err != nil {
		a.showDownloadFailed(status, songInfo, render.SendFailed)
		return songInfo, err
	}

	songInfo.FileID = audio.Audio.FileID
	if audio.Audio.Thumbnail != nil {
		songInfo.ThumbFileID = audio.Audio.Thumbnail.FileID
	}
	return songInfo, a.store.Save(&songInfo)
}

// uploadLimitMB 官方 Bot API 限制上传 50 MB，--local 模式的自建服务为 2000 MB
func (a *App) uploadLimitMB() int {
	if a.cfg.Bot.LocalMode {
		return 2000
	}
	return 50
}

// songFileName 歌曲与歌词文件共用命名
func songFileName(artists, name, ext string) string {
	return textx.SanitizeFilename(fmt.Sprintf("%s - %s.%s", strings.ReplaceAll(artists, "/", ","), name, ext))
}

func artistNames(song types.SongDetailData) []string {
	names := make([]string, 0, len(song.Ar))
	for _, ar := range song.Ar {
		names = append(names, ar.Name)
	}
	return names
}

func (a *App) showStatus(status func(string) error, text string) {
	if err := status(text); err != nil {
		a.log.Errorln(err)
	}
}

// 错误详情可能含服务器 IP、下载直链等，只展示错误类型，详情由调用方记录日志
func (a *App) showDownloadFailed(status func(string) error, songInfo store.SongInfo, reason string) {
	a.showStatus(status, render.MusicStatus(songInfo.SongName, songInfo.SongAlbum, songInfo.FileExt, float64(songInfo.MusicSize)/1024/1024, reason+"\n"+render.RetryLater))
}

func (a *App) musicCaption(songInfo store.SongInfo) string {
	return render.MusicCaption(songInfo.SongName, songInfo.SongArtists, songInfo.SongAlbum, songInfo.FileExt,
		float64(songInfo.MusicSize+songInfo.EmbPicSize)/1024/1024, float64(songInfo.BitRate)/1000, a.me.Username)
}

// cachedMusicResult 缓存歌曲用 CachedAudio 会被拒绝（AUDIO_CONTENT_TYPE_INVALID），须以文档形式发送
func (a *App) cachedMusicResult(id string, songInfo store.SongInfo, prog *types.ProgramDetailData) *models.InlineQueryResultCachedDocument {
	return &models.InlineQueryResultCachedDocument{
		ID:             id,
		DocumentFileID: songInfo.FileID,
		Title:          fmt.Sprintf("%s - %s", songInfo.SongArtists, songInfo.SongName),
		Description:    songInfo.SongAlbum,
		Caption:        a.musicCaption(songInfo),
		ParseMode:      models.ParseModeHTML,
		ReplyMarkup:    musicKeyboard(songInfo, prog),
	}
}

func (a *App) sendMusic(ctx context.Context, songInfo store.SongInfo, prog *types.ProgramDetailData, musicPath, picPath string, chatID int64, reply *models.ReplyParameters) (*models.Message, error) {
	params := &tgb.SendAudioParams{
		ChatID:          chatID,
		Caption:         a.musicCaption(songInfo),
		ParseMode:       models.ParseModeHTML,
		Title:           songInfo.SongName,
		Performer:       songInfo.SongArtists,
		Duration:        songInfo.Duration,
		ReplyMarkup:     musicKeyboard(songInfo, prog),
		ReplyParameters: reply,
	}
	if songInfo.FileID != "" {
		params.Audio = &models.InputFileString{Data: songInfo.FileID}
		return a.tg.SendAudio(ctx, params)
	}
	_, _ = a.tg.SendChatAction(ctx, &tgb.SendChatActionParams{
		ChatID: chatID,
		Action: models.ChatActionUploadDocument,
	})
	return a.sendAudioFile(ctx, params, musicPath, picPath)
}
