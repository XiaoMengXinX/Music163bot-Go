package bot

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/XiaoMengXinX/CloudMusicDownloader/downloader"
	"github.com/XiaoMengXinX/Music163Api-Go/api"
	"github.com/XiaoMengXinX/Music163Api-Go/types"
	"github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// 限制并发任务数
var musicLimiter = make(chan bool, 4)

func processMusic(musicID int, message tgbotapi.Message, bot *tgbotapi.BotAPI) (err error) {
	defer func() {
		e := recover()
		if e != nil {
			err = fmt.Errorf("%v", e)
		}
	}()
	var songInfo SongInfo
	var msgResult tgbotapi.Message

	// 寄
	寄 := func() {
		_, err = bot.Send(tgbotapi.NewSticker(message.Chat.ID, tgbotapi.FileID("CAACAgEAAxkBAAFKA59iE5tHwVtRSQ9mruwzzCKok9hVHgAC1gEAAvrUsEYzQLN-IIuSFyME")))
		if err != nil {
			logrus.Errorln(err)
		}
	}

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
		寄()
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
		寄()
		return err
	}
	if songURL.Data[0].Url == "" {
		editMsg := tgbotapi.NewEditMessageText(message.Chat.ID, msgResult.MessageID, getUrlFailed)
		_, err = bot.Send(editMsg)
		if err != nil {
			logrus.Errorln(err)
		}
		寄()
		return err
	}

	var isAprilFool bool
	var userInfo UserInfo
	user := UserDB.Session(&gorm.Session{})
	err = user.Where("user_id = ?", message.From.ID).First(&userInfo).Error
	if err != nil {
		if isAprilFoolsDay() {
			userInfo.UserID = message.From.ID
			userInfo.IsAprilFooled = true
			fakeMusicDetail, _ := api.GetSongURL(data, api.SongURLConfig{
				EncodeType: "mp3",
				Level:      "standard",
				Ids:        []int{29038398},
			})
			if len(fakeMusicDetail.Data) != 0 {
				songURL.Data[0] = fakeMusicDetail.Data[0]
				isAprilFool = true
				err = user.Create(&userInfo).Error
				if err != nil {
					return err
				}
			}
		}
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

	timeStamp := time.Now().UnixMicro()

	d, err := newDownloader(url, fmt.Sprintf("%d-%s", timeStamp, path.Base(url)), 8)
	if err != nil {
		sendFailed(err)
		return err
	}

	var isMD5Verified = false
	for i := 0; i < maxRetryTimes && songURL.Data[0].Md5 != ""; i++ {
		err = d.download()
		if err != nil && !isTimeout(err) {
			sendFailed(err)
			return err
		} else if err != nil && isTimeout(err) {
			sendFailed(fmt.Errorf(downloadTimeout + "\n" + retryLater))
			return err
		}

		if isMD5Verified, err = verifyMD5(cacheDir+"/"+fmt.Sprintf("%d-%s", timeStamp, path.Base(url)), songURL.Data[0].Md5); !isMD5Verified && config["AutoRetry"] != "false" {
			sendFailed(fmt.Errorf("%s\n"+reTrying, err, i+1, maxRetryTimes))
			err := os.Remove(cacheDir + "/" + fmt.Sprintf("%d-%s", timeStamp, path.Base(url)))
			if err != nil {
				logrus.Errorln(err)
			}
			if songUrl, _ := api.GetSongURL(data, api.SongURLConfig{Ids: []int{musicID}}); len(songUrl.Data) != 0 {
				d, err = newDownloader(url, fmt.Sprintf("%d-%s", timeStamp, path.Base(songUrl.Data[0].Url)), 2)
				if err != nil {
					sendFailed(err)
					return err
				}
			}
		} else {
			break
		}
	}
	if !isMD5Verified && songURL.Data[0].Md5 != "" {
		sendFailed(fmt.Errorf("%s\n%s", md5VerFailed, retryLater))
		return nil
	}

	var picPath, resizePicPath string
	p, _ := newDownloader(songDetail.Songs[0].Al.PicUrl, fmt.Sprintf("%d-%s", timeStamp, path.Base(songDetail.Songs[0].Al.PicUrl)), 8)
	err = p.download()
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

	editMsg = tgbotapi.NewEditMessageText(message.Chat.ID, msgResult.MessageID, fmt.Sprintf(musicInfoMsg+uploading, songInfo.SongName, songInfo.SongAlbum, songInfo.FileExt, float64(songInfo.MusicSize)/1024/1024))
	_, err = bot.Send(editMsg)
	if err != nil {
		return err
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

	marker, _ := downloader.CreateMarker(songDetail.Songs[0], songURL.Data[0])
	switch path.Ext(path.Base(url)) {
	case ".mp3":
		err = downloader.AddMp3Id3v2(cacheDir+"/"+fmt.Sprintf("%d-%s", timeStamp, path.Base(url)), musicPic, marker, songDetail.Songs[0])
	case ".flac":
		err = downloader.AddFlacId3v2(cacheDir+"/"+fmt.Sprintf("%d-%s", timeStamp, path.Base(url)), musicPic, marker, songDetail.Songs[0])
	default:
		err = downloader.AddMp3Id3v2(cacheDir+"/"+fmt.Sprintf("%d-%s", timeStamp, path.Base(url)), musicPic, marker, songDetail.Songs[0])
	}
	if err != nil {
		sendFailed(err)
		return err
	}

	var replacer = strings.NewReplacer("/", " ", "?", " ", "*", " ", ":", " ", "|", " ", "\\", " ", "<", " ", ">", " ", "\"", " ")
	fileName := replacer.Replace(fmt.Sprintf("%v - %v.%v", strings.Replace(songInfo.SongArtists, "/", ",", -1), songInfo.SongName, songInfo.FileExt))
	err = os.Rename(cacheDir+"/"+fmt.Sprintf("%d-%s", timeStamp, path.Base(url)), cacheDir+"/"+fileName)
	if err != nil {
		fileName = fmt.Sprintf("%d-%s", timeStamp, path.Base(url))
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

	if !isAprilFool {
		err = db.Create(&songInfo).Error // 写入歌曲缓存
		if err != nil {
			return err
		}
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
	userSetting, err := getSettings(UserSetting, message.From.ID)
	if err != nil {
		return audio, err
	}
	chatSetting, err := getSettings(ChatSetting, message.Chat.ID)
	if err != nil {
		return audio, err
	}
	globalSetting, err := getSettings(GlobalSetting, 0)
	if err != nil {
		return audio, err
	}
	var numericKeyboard tgbotapi.InlineKeyboardMarkup
	if userSetting.ShareKey && globalSetting.ShareKey && chatSetting.ShareKey {
		numericKeyboard = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonURL(fmt.Sprintf("%s- %s", songInfo.SongName, songInfo.SongArtists), fmt.Sprintf("https://music.163.com/song?id=%d", songInfo.MusicID)),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonSwitch(sendMeTo, fmt.Sprintf("https://music.163.com/song?id=%d", songInfo.MusicID)),
			),
		)
	} else {
		numericKeyboard = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonURL(fmt.Sprintf("%s- %s", songInfo.SongName, songInfo.SongArtists), fmt.Sprintf("https://music.163.com/song?id=%d", songInfo.MusicID)),
			),
		)
	}
	var newAudio tgbotapi.AudioConfig
	if songInfo.FileID != "" {
		newAudio = tgbotapi.NewAudio(message.Chat.ID, tgbotapi.FileID(songInfo.FileID))
	} else {
		newAudio = tgbotapi.NewAudio(message.Chat.ID, tgbotapi.FilePath(musicPath))
	}
	if userSetting.SourceInfo && globalSetting.SourceInfo && chatSetting.SourceInfo {
		newAudio.Caption = fmt.Sprintf(musicInfo, songInfo.SongName, songInfo.SongArtists, songInfo.SongAlbum, songInfo.FileExt, float64(songInfo.MusicSize+songInfo.EmbPicSize)/1024/1024, float64(songInfo.BitRate)/1000, botName)
	} else {
		newAudio.Caption = fmt.Sprintf(musicInfoNoVia, songInfo.SongName, songInfo.SongArtists, songInfo.SongAlbum, songInfo.FileExt, float64(songInfo.MusicSize+songInfo.EmbPicSize)/1024/1024, float64(songInfo.BitRate)/1000)
	}
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
