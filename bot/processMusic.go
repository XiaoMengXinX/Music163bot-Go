package bot

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	marker "github.com/XiaoMengXinX/163KeyMarker"
	"github.com/XiaoMengXinX/Music163Api-Go/api"
	"github.com/XiaoMengXinX/Music163Api-Go/types"
	downloader "github.com/XiaoMengXinX/SimpleDownloader"
	"github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// 限制并发任务数
var musicLimiter = make(chan bool, 4)

func processMusic(musicID int, message tgbotapi.Message, bot *tgbotapi.BotAPI) (err error) {
	d := downloader.NewDownloader().SetSavePath(cacheDir).SetBreakPoint(true)

	timeout, _ := strconv.Atoi(config["DownloadTimeout"])
	if timeout != 0 {
		d.SetTimeOut(time.Duration(int64(timeout)) * time.Second)
	} else {
		d.SetTimeOut(60 * time.Second) // 默认超时时间为 60 秒
	}

	defer func() {
		e := recover()
		if e != nil {
			err = fmt.Errorf("%v", e)
		}
	}()
	var songInfo SongInfo
	var msgResult tgbotapi.Message

	sendFailed := func(err error) {
		var errText string
		if strings.Contains(fmt.Sprintf("%v", err), md5VerFailed) || strings.Contains(fmt.Sprintf("%v", err), downloadTimeout) {
			errText = "%v"
		} else {
			errText = uploadFailed
		}
		editMsg := tgbotapi.NewEditMessageText(message.Chat.ID, msgResult.MessageID, fmt.Sprintf(musicInfoMsg+errText, songInfo.SongName, songInfo.SongAlbum, songInfo.FileExt, float64(songInfo.MusicSize)/1024/1024, strings.ReplaceAll(err.Error(), config["BOT_TOKEN"], "BOT_TOKEN")))
		_, err = bot.Send(editMsg)
		if err != nil {
			logrus.Errorln(err)
		}
	}

	db := MusicDB.Session(&gorm.Session{})
	err = db.Where("music_id = ?", musicID).First(&songInfo).Error
	if err == nil {
		msg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf(musicInfoMsg+hitCache, songInfo.SongName, songInfo.SongAlbum, songInfo.FileExt, float64(songInfo.MusicSize)/1024/1024))
		msg.ReplyToMessageID = message.MessageID
		msgResult, err = bot.Send(msg)
		if err != nil {
			return err
		}

		_, err := sendMusic(songInfo, "", "", message, bot)
		if err != nil {
			sendFailed(err)
			return err
		}

		deleteMsg := tgbotapi.NewDeleteMessage(msgResult.Chat.ID, msgResult.MessageID)
		_, err = bot.Request(deleteMsg)
		if err != nil {
			return err
		}

		return err
	}
	msg := tgbotapi.NewMessage(message.Chat.ID, waitForDown)
	msg.ReplyToMessageID = message.MessageID
	msgResult, err = bot.Send(msg)
	if err != nil {
		return err
	}

	musicLimiter <- true
	defer func() {
		<-musicLimiter
	}()

	err = db.Where("music_id = ?", musicID).First(&songInfo).Error
	if err == nil {
		editMsg := tgbotapi.NewEditMessageText(message.Chat.ID, msgResult.MessageID, fmt.Sprintf(musicInfoMsg+hitCache, songInfo.SongName, songInfo.SongAlbum, songInfo.FileExt, float64(songInfo.MusicSize)/1024/1024))
		msgResult, err = bot.Send(editMsg)
		if err != nil {
			return err
		}

		_, err := sendMusic(songInfo, "", "", message, bot)
		if err != nil {
			sendFailed(err)
			return err
		}

		deleteMsg := tgbotapi.NewDeleteMessage(msgResult.Chat.ID, msgResult.MessageID)
		_, err = bot.Request(deleteMsg)
		if err != nil {
			return err
		}

		return err
	}

	editMsg := tgbotapi.NewEditMessageText(message.Chat.ID, msgResult.MessageID, fetchInfo)
	_, err = bot.Send(editMsg)
	if err != nil {
		return err
	}

	b := api.NewBatch(
		api.BatchAPI{
			Key:  api.SongDetailAPI,
			Json: api.CreateSongDetailReqJson([]int{musicID}),
		},
		api.BatchAPI{
			Key:  api.SongUrlAPI,
			Json: api.CreateSongURLJson(api.SongURLConfig{Ids: []int{musicID}}),
		},
	)
	if b.Do(data).Error != nil {
		return err
	}
	_, result := b.Parse()

	var songDetail types.SongsDetailData
	_ = json.Unmarshal([]byte(result[api.SongDetailAPI]), &songDetail)

	var songURL types.SongsURLData
	_ = json.Unmarshal([]byte(result[api.SongUrlAPI]), &songURL)

	if len(songDetail.Songs) == 0 || len(songURL.Data) == 0 {
		editMsg := tgbotapi.NewEditMessageText(message.Chat.ID, msgResult.MessageID, fetchInfoFailed)
		_, err = bot.Send(editMsg)
		if err != nil {
			logrus.Errorln(err)
		}
		return err
	}
	if songURL.Data[0].Url == "" {
		editMsg := tgbotapi.NewEditMessageText(message.Chat.ID, msgResult.MessageID, getUrlFailed)
		_, err = bot.Send(editMsg)
		if err != nil {
			logrus.Errorln(err)
		}
		return err
	}

	songInfo.FromChatID = message.Chat.ID
	if message.Chat.IsPrivate() {
		songInfo.FromChatName = message.Chat.UserName
	} else {
		songInfo.FromChatName = message.Chat.Title
	}
	songInfo.FromUserID = message.From.ID
	songInfo.FromUserName = message.From.UserName

	songInfo.MusicID = musicID
	songInfo.Duration = songDetail.Songs[0].Dt / 1000
	songInfo.SongName = songDetail.Songs[0].Name // 解析歌曲信息
	songInfo.SongArtists = parseArtist(songDetail.Songs[0])
	songInfo.SongAlbum = songDetail.Songs[0].Al.Name
	url := songURL.Data[0].Url
	switch path.Ext(path.Base(url)) {
	case ".mp3":
		songInfo.FileExt = "mp3"
	case ".flac":
		songInfo.FileExt = "flac"
	default:
		songInfo.FileExt = "mp3"
	}
	songInfo.MusicSize = songURL.Data[0].Size
	songInfo.BitRate = 8 * songURL.Data[0].Size / (songDetail.Songs[0].Dt / 1000)

	if picRes, err := http.Head(songDetail.Songs[0].Al.PicUrl); err == nil {
		songInfo.PicSize = int(picRes.ContentLength)
	} else {
		logrus.Errorln(err)
	}

	editMsg = tgbotapi.NewEditMessageText(message.Chat.ID, msgResult.MessageID, fmt.Sprintf(musicInfoMsg+downloading, songInfo.SongName, songInfo.SongAlbum, songInfo.FileExt, float64(songInfo.MusicSize)/1024/1024))
	_, err = bot.Send(editMsg)
	if err != nil {
		return err
	}

	hostReplacer := strings.NewReplacer("m8.", "m7.", "m801.", "m701.", "m804.", "m701.", "m704.", "m701.")

	timeStamp := time.Now().UnixMicro()
	musicFileName := fmt.Sprintf("%d-%s", timeStamp, path.Base(url))

	task, _ := d.NewDownloadTask(url)
	host := task.GetHostName()
	task.ReplaceHostName(hostReplacer.Replace(host)).ForceHttps().ForceMultiThread()
	errCh := task.SetFileName(musicFileName).DownloadWithChannel()

	updateStatus := func(task *downloader.DownloadTask, ch chan error, statusText string) (err error) {
		var lastUpdateTime int64
	loop:
		for {
			select {
			case err = <-ch:
				break loop
			default:
				writtenBytes := task.GetWrittenBytes()
				if task.GetFileSize() == 0 || writtenBytes == 0 || time.Now().Unix()-lastUpdateTime < 5 {
					continue
				}
				editMsg = tgbotapi.NewEditMessageText(message.Chat.ID, msgResult.MessageID, fmt.Sprintf(musicInfoMsg+statusText+downloadStatus, songInfo.SongName, songInfo.SongAlbum, songInfo.FileExt, float64(songInfo.MusicSize)/1024/1024, task.CalculateSpeed(time.Millisecond*500), float64(writtenBytes)/1024/1024, float64(task.GetFileSize())/1024/1024, (writtenBytes*100)/task.GetFileSize()))
				_, _ = bot.Send(editMsg)
				lastUpdateTime = time.Now().Unix()
			}
		}
		return err
	}

	err = updateStatus(task, errCh, downloading)
	if err != nil {
		if config["ReverseProxy"] != "" {
			ch := task.WithResolvedIpOnHost(config["ReverseProxy"]).DownloadWithChannel()
			err = updateStatus(task, ch, redownloading)
			if err != nil {
				sendFailed(err)
				task.CleanTempFiles()
				return err
			}
		} else {
			sendFailed(err)
			task.CleanTempFiles()
			return err
		}
	}

	isMD5Verified, _ := verifyMD5(cacheDir+"/"+musicFileName, songURL.Data[0].Md5)
	if !isMD5Verified && songURL.Data[0].Md5 != "" {
		err = os.Remove(cacheDir + "/" + fmt.Sprintf("%d-%s", timeStamp, path.Base(url)))
		if err != nil {
			logrus.Errorln(err)
		}
		sendFailed(fmt.Errorf("%s\n%s", md5VerFailed, retryLater))
		return nil
	}

	var picPath, resizePicPath string
	p, _ := d.NewDownloadTask(songDetail.Songs[0].Al.PicUrl)
	err = p.SetFileName(fmt.Sprintf("%d-%s", timeStamp, path.Base(songDetail.Songs[0].Al.PicUrl))).Download()
	if err != nil {
		logrus.Errorln(err)
	} else {
		picPath = cacheDir + "/" + fmt.Sprintf("%d-%s", timeStamp, path.Base(songDetail.Songs[0].Al.PicUrl))
		var err error
		resizePicPath, err = resizeImg(picPath)
		if err != nil {
			logrus.Errorln(err)
		}
	}

	var musicPic string
	picStat, err := os.Stat(picPath)
	if picStat != nil && err == nil {
		if picStat.Size() > 2*1024*1024 {
			musicPic = resizePicPath
			embPicStat, _ := os.Stat(resizePicPath)
			songInfo.EmbPicSize = int(embPicStat.Size())
		} else {
			musicPic = picPath
			songInfo.EmbPicSize = songInfo.PicSize
		}
	} else {
		logrus.Errorln(err)
	}

	var pic *os.File = nil

	if picStat != nil && err == nil {
		pic, _ = os.Open(musicPic)
		defer pic.Close()
	}

	var replacer = strings.NewReplacer("/", " ", "?", " ", "*", " ", ":", " ", "|", " ", "\\", " ", "<", " ", ">", " ", "\"", " ")
	fileName := replacer.Replace(fmt.Sprintf("%v - %v.%v", strings.Replace(songInfo.SongArtists, "/", ",", -1), songInfo.SongName, songInfo.FileExt))
	err = os.Rename(cacheDir+"/"+fmt.Sprintf("%d-%s", timeStamp, path.Base(url)), cacheDir+"/"+fileName)
	if err != nil {
		fileName = fmt.Sprintf("%d-%s", timeStamp, path.Base(url))
	}

	mark := marker.CreateMarker(songDetail.Songs[0], songURL.Data[0])

	file, _ := os.Open(cacheDir + "/" + fileName)
	defer file.Close()

	err = marker.AddMusicID3V2(file, pic, mark)
	if err != nil {
		file, _ = os.Open(cacheDir + "/" + fileName)
		defer file.Close()
		err = marker.AddMusicID3V2(file, nil, mark)
	}
	if err != nil {
		sendFailed(err)
		return err
	}

	editMsg = tgbotapi.NewEditMessageText(message.Chat.ID, msgResult.MessageID, fmt.Sprintf(musicInfoMsg+uploading, songInfo.SongName, songInfo.SongAlbum, songInfo.FileExt, float64(songInfo.MusicSize)/1024/1024))
	_, err = bot.Send(editMsg)
	if err != nil {
		return err
	}

	audio, err := sendMusic(songInfo, cacheDir+"/"+fileName, resizePicPath, message, bot)
	if err != nil {
		sendFailed(err)
		return err
	}

	songInfo.FileID = audio.Audio.FileID
	if audio.Audio.Thumbnail != nil {
		songInfo.ThumbFileID = audio.Audio.Thumbnail.FileID
	}

	err = db.Create(&songInfo).Error // 写入歌曲缓存
	if err != nil {
		return err
	}

	for _, f := range []string{cacheDir + "/" + fileName, resizePicPath, picPath} {
		err := os.Remove(f)
		if err != nil {
			logrus.Errorln(err)
		}
	}

	deleteMsg := tgbotapi.NewDeleteMessage(msgResult.Chat.ID, msgResult.MessageID)
	_, err = bot.Request(deleteMsg)
	if err != nil {
		return err
	}

	return err
}

func sendMusic(songInfo SongInfo, musicPath, picPath string, message tgbotapi.Message, bot *tgbotapi.BotAPI) (audio tgbotapi.Message, err error) {
	var numericKeyboard tgbotapi.InlineKeyboardMarkup
	numericKeyboard = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL(fmt.Sprintf("%s- %s", songInfo.SongName, songInfo.SongArtists), fmt.Sprintf("https://music.163.com/song?id=%d", songInfo.MusicID)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonSwitch(sendMeTo, fmt.Sprintf("https://music.163.com/song?id=%d", songInfo.MusicID)),
		),
	)
	var newAudio tgbotapi.AudioConfig
	if songInfo.FileID != "" {
		newAudio = tgbotapi.NewAudio(message.Chat.ID, tgbotapi.FileID(songInfo.FileID))
	} else {
		newAudio = tgbotapi.NewAudio(message.Chat.ID, tgbotapi.FilePath(musicPath))
		status := tgbotapi.NewChatAction(message.Chat.ID, "upload_document")
		_, _ = bot.Send(status)
	}
	newAudio.Caption = fmt.Sprintf(musicInfo, songInfo.SongName, songInfo.SongArtists, songInfo.SongAlbum, songInfo.FileExt, float64(songInfo.MusicSize+songInfo.EmbPicSize)/1024/1024, float64(songInfo.BitRate)/1000, botName)
	newAudio.Title = fmt.Sprintf("%s", songInfo.SongName)
	newAudio.Performer = songInfo.SongArtists
	newAudio.Duration = songInfo.Duration
	newAudio.ReplyMarkup = numericKeyboard
	newAudio.ReplyToMessageID = message.MessageID
	if songInfo.ThumbFileID != "" {
		newAudio.Thumb = tgbotapi.FileID(songInfo.ThumbFileID)
	}
	if picPath != "" {
		newAudio.Thumb = tgbotapi.FilePath(picPath)
	}
	audio, err = bot.Send(newAudio)
	if strings.Contains(fmt.Sprintf("%v", err), "replied message not found") {
		newAudio.ReplyToMessageID = 0
		audio, err = bot.Send(newAudio)
	}
	return audio, err
}
