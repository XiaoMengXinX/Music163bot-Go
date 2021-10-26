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

func processMusic(musicid int, update tgbotapi.Update, bot *tgbotapi.BotAPI) (err error) {
	defer func() {
		e := recover()
		if e != nil {
			err = fmt.Errorf("%v", e)
		}
	}()
	msg := tgbotapi.NewMessage(update.Message.Chat.ID, fetchInfo)
	msg.ReplyToMessageID = update.Message.MessageID
	msgResult, err := bot.Send(msg)
	if err != nil {
		return err
	}

	b := api.NewBatch(
		api.BatchAPI{
			Key:  api.SongDetailAPI,
			Json: api.CreateSongDetailReqJson([]int{musicid}),
		},
		api.BatchAPI{
			Key:  api.SongUrlAPI,
			Json: api.CreateSongURLJson(api.SongURLConfig{Ids: []int{musicid}}),
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
		editMsg := tgbotapi.NewEditMessageText(update.Message.Chat.ID, msgResult.MessageID, fetchInfoFailed)
		_, err = bot.Send(editMsg)
		if err != nil {
			logrus.Errorln(err)
		}
		return err
	}
	if songURL.Data[0].Url == "" {
		editMsg := tgbotapi.NewEditMessageText(update.Message.Chat.ID, msgResult.MessageID, getUrlFailed)
		_, err = bot.Send(editMsg)
		if err != nil {
			logrus.Errorln(err)
		}
		return err
	}

	var songInfo util.SongInfo

	songInfo.FromChatID = update.Message.Chat.ID
	if update.Message.Chat.IsPrivate() {
		songInfo.FromChatName = update.Message.Chat.UserName
	} else {
		songInfo.FromChatName = update.Message.Chat.Title
	}
	songInfo.FromUserID = update.Message.From.ID
	songInfo.FromUserName = update.Message.From.UserName

	songInfo.MusicID = musicid
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

	newEditMsg := tgbotapi.NewEditMessageText(update.Message.Chat.ID, msgResult.MessageID, fmt.Sprintf(musicInfoMsg+downloading, songInfo.SongName, songInfo.SongAlbum, songInfo.FileExt, songInfo.FileSize))
	_, err = bot.Send(newEditMsg)
	if err != nil {
		return err
	}

	downFailed := func() {
		newMsg := tgbotapi.NewEditMessageText(update.Message.Chat.ID, msgResult.MessageID, fmt.Sprintf(musicInfoMsg+downloadFailed, songInfo.SongName, songInfo.SongAlbum, songInfo.FileExt, songInfo.FileSize))
		_, err := bot.Send(newMsg)
		if err != nil {
			logrus.Errorln(err)
		}
	}

	timeStramp := time.Now().UnixMicro()

	d, err := New(url, fmt.Sprintf("%d-%s", timeStramp, path.Base(url)), 8)
	if err != nil {
		downFailed()
		return err
	}
	err = d.Download()
	if err != nil {
		downFailed()
		return err
	}

	var picPath string
	p, _ := New(songDetail.Songs[0].Al.PicUrl, fmt.Sprintf("%d-%s", timeStramp, path.Base(songDetail.Songs[0].Al.PicUrl)), 8)
	err = p.Download()
	if err != nil {
		logrus.Errorln(err)
	} else {
		var err error
		picPath, err = resizeImg(cacheDir + "/" + fmt.Sprintf("%d-%s", timeStramp, path.Base(songDetail.Songs[0].Al.PicUrl)))
		if err != nil {
			logrus.Errorln(err)
		}
	}

	editMsg := tgbotapi.NewEditMessageText(update.Message.Chat.ID, msgResult.MessageID, fmt.Sprintf(musicInfoMsg+uploading, songInfo.SongName, songInfo.SongAlbum, songInfo.FileExt, songInfo.FileSize))
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
		downFailed()
		return err
	}

	var replacer = strings.NewReplacer("/", " ", "?", " ", "*", " ", ":", " ", "|", " ", "\\", " ", "<", " ", ">", " ", "\"", " ")
	fileName := replacer.Replace(fmt.Sprintf("%v - %v.%v", strings.Replace(songInfo.SongArtists, "/", ",", -1), songInfo.SongName, songInfo.FileExt))
	err = os.Rename(cacheDir+"/"+fmt.Sprintf("%d-%s", timeStramp, path.Base(url)), cacheDir+"/"+fileName)
	if err != nil {
		downFailed()
		return err
	}

	numericKeyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL(fmt.Sprintf("%s- %s", songInfo.SongName, songInfo.SongArtists), fmt.Sprintf("https://music.163.com/#/song/%d/", songInfo.MusicID)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonSwitch(sendMeTo, fmt.Sprintf("https://music.163.com/#/song/%d/", songInfo.MusicID)),
		),
	)

	newAudio := tgbotapi.NewAudio(update.Message.Chat.ID, cacheDir+"/"+fileName)
	newAudio.Caption = fmt.Sprintf(musicInfo, songInfo.SongName, songInfo.SongArtists, songInfo.SongAlbum, songInfo.FileExt, float64(songInfo.BitRate)/1000, botName)
	newAudio.Title = fmt.Sprintf("%s", songInfo.SongName)
	newAudio.Performer = songInfo.SongArtists
	newAudio.Duration = songDetail.Songs[0].Dt / 1000
	newAudio.ReplyMarkup = numericKeyboard
	if picPath != "" {
		newAudio.Thumb = picPath
	}

	audio, err := bot.Send(newAudio)
	if err != nil {
		editMsg = tgbotapi.NewEditMessageText(update.Message.Chat.ID, msgResult.MessageID, fmt.Sprintf(musicInfoMsg+uploadFailed, songInfo.SongName, songInfo.SongAlbum, songInfo.FileExt, songInfo.FileSize, err))
		_, err = bot.Send(editMsg)
		if err != nil {
			logrus.Errorln(err)
		}
		return err
	}

	songInfo.FileID = audio.Audio.FileID
	if audio.Audio.Thumbnail != nil {
		songInfo.ThumbFileID = audio.Audio.Thumbnail.FileID
	}

	db := DB.Session(&gorm.Session{})
	dbResult := db.Create(&songInfo) // 写入歌曲缓存
	if dbResult.Error != nil {
		return dbResult.Error
	}

	return err
}
