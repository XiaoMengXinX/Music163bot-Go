package bot

import (
	"encoding/json"
	"fmt"
	"github.com/XiaoMengXinX/CloudMusicDownloader/downloader"
	"github.com/XiaoMengXinX/Music163Api-Go/api"
	"github.com/XiaoMengXinX/Music163Api-Go/types"
	"github.com/XiaoMengXinX/Music163bot-Go/v2/util"
	"github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"os"
	"path"
	"strings"
	"time"
)

var limiter = make(chan bool, 4)

func processMusic(musicID int, message tgbotapi.Message, bot *tgbotapi.BotAPI) (err error) {
	defer func() {
		e := recover()
		if e != nil {
			err = fmt.Errorf("%v", e)
		}
	}()
	var songInfo util.SongInfo
	var msgResult tgbotapi.Message
	sendFailed := func(err error) {
		editMsg := tgbotapi.NewEditMessageText(message.Chat.ID, msgResult.MessageID, fmt.Sprintf(musicInfoMsg+uploadFailed, songInfo.SongName, songInfo.SongAlbum, songInfo.FileExt, songInfo.FileSize, err))
		_, err = bot.Send(editMsg)
		if err != nil {
			logrus.Errorln(err)
		}
	}

	db := DB.Session(&gorm.Session{})
	err = db.Where("music_id = ?", musicID).First(&songInfo).Error
	if err == nil {
		msg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf(musicInfoMsg+hitCache, songInfo.SongName, songInfo.SongAlbum, songInfo.FileExt, songInfo.FileSize))
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

	limiter <- true
	defer func() {
		<-limiter
	}()

	err = db.Where("music_id = ?", musicID).First(&songInfo).Error
	if err == nil {
		editMsg := tgbotapi.NewEditMessageText(message.Chat.ID, msgResult.MessageID, fmt.Sprintf(musicInfoMsg+hitCache, songInfo.SongName, songInfo.SongAlbum, songInfo.FileExt, songInfo.FileSize))
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
	_, _, err = b.Do(data)
	if err != nil {
		return err
	}
	result := b.Parse()

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
	songInfo.FileSize = fmt.Sprintf("%.2f", float64(songURL.Data[0].Size)/1024/1024)
	songInfo.BitRate = 8 * songURL.Data[0].Size / (songDetail.Songs[0].Dt / 1000)

	editMsg = tgbotapi.NewEditMessageText(message.Chat.ID, msgResult.MessageID, fmt.Sprintf(musicInfoMsg+downloading, songInfo.SongName, songInfo.SongAlbum, songInfo.FileExt, songInfo.FileSize))
	_, err = bot.Send(editMsg)
	if err != nil {
		return err
	}

	timeStramp := time.Now().UnixMicro()

	d, err := New(url, fmt.Sprintf("%d-%s", timeStramp, path.Base(url)), 8)
	if err != nil {
		sendFailed(err)
		return err
	}
	err = d.Download()
	if err != nil {
		sendFailed(err)
		return err
	}

	var picPath, resizePicPath string
	p, _ := New(songDetail.Songs[0].Al.PicUrl, fmt.Sprintf("%d-%s", timeStramp, path.Base(songDetail.Songs[0].Al.PicUrl)), 2)
	err = p.Download()
	if err != nil {
		logrus.Errorln(err)
	} else {
		picPath = cacheDir + "/" + fmt.Sprintf("%d-%s", timeStramp, path.Base(songDetail.Songs[0].Al.PicUrl))
		var err error
		resizePicPath, err = resizeImg(cacheDir + "/" + fmt.Sprintf("%d-%s", timeStramp, path.Base(songDetail.Songs[0].Al.PicUrl)))
		if err != nil {
			logrus.Errorln(err)
		}
	}

	editMsg = tgbotapi.NewEditMessageText(message.Chat.ID, msgResult.MessageID, fmt.Sprintf(musicInfoMsg+uploading, songInfo.SongName, songInfo.SongAlbum, songInfo.FileExt, songInfo.FileSize))
	_, err = bot.Send(editMsg)
	if err != nil {
		return err
	}

	marker, _ := downloader.CreateMarker(songDetail.Songs[0], songURL.Data[0])
	switch path.Ext(path.Base(url)) {
	case ".mp3":
		err = downloader.AddMp3Id3v2(cacheDir+"/"+fmt.Sprintf("%d-%s", timeStramp, path.Base(url)), picPath, marker, songDetail.Songs[0])
	case ".flac":
		err = downloader.AddFlacId3v2(cacheDir+"/"+fmt.Sprintf("%d-%s", timeStramp, path.Base(url)), picPath, marker, songDetail.Songs[0])
	default:
		err = downloader.AddMp3Id3v2(cacheDir+"/"+fmt.Sprintf("%d-%s", timeStramp, path.Base(url)), picPath, marker, songDetail.Songs[0])
	}
	if err != nil {
		sendFailed(err)
		return err
	}

	var replacer = strings.NewReplacer("/", " ", "?", " ", "*", " ", ":", " ", "|", " ", "\\", " ", "<", " ", ">", " ", "\"", " ")
	fileName := replacer.Replace(fmt.Sprintf("%v - %v.%v", strings.Replace(songInfo.SongArtists, "/", ",", -1), songInfo.SongName, songInfo.FileExt))
	err = os.Rename(cacheDir+"/"+fmt.Sprintf("%d-%s", timeStramp, path.Base(url)), cacheDir+"/"+fileName)
	if err != nil {
		sendFailed(err)
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

	dbResult := db.Create(&songInfo) // 写入歌曲缓存
	if dbResult.Error != nil {
		return dbResult.Error
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

func sendMusic(songInfo util.SongInfo, musicPath, picPath string, message tgbotapi.Message, bot *tgbotapi.BotAPI) (audio tgbotapi.Message, err error) {
	numericKeyboard := tgbotapi.NewInlineKeyboardMarkup(
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
		newAudio = tgbotapi.NewAudio(message.Chat.ID, musicPath)
	}
	newAudio.Caption = fmt.Sprintf(musicInfo, songInfo.SongName, songInfo.SongArtists, songInfo.SongAlbum, songInfo.FileExt, float64(songInfo.BitRate)/1000, botName)
	newAudio.Title = fmt.Sprintf("%s", songInfo.SongName)
	newAudio.Performer = songInfo.SongArtists
	newAudio.Duration = songInfo.Duration
	newAudio.ReplyMarkup = numericKeyboard
	if songInfo.ThumbFileID != "" {
		newAudio.Thumb = tgbotapi.FileID(songInfo.ThumbFileID)
	}
	if picPath != "" {
		newAudio.Thumb = picPath
	}
	audio, err = bot.Send(newAudio)
	return audio, err
}
