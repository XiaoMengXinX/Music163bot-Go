package bot

import (
	"encoding/json"
	"fmt"
	"github.com/XiaoMengXinX/CloudMusicDownloader/downloader"
	"github.com/XiaoMengXinX/Music163Api-Go/api"
	"github.com/XiaoMengXinX/Music163Api-Go/types"
	"github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sirupsen/logrus"
	"path"
)

func processMusic(musicid int, update tgbotapi.Update, bot *tgbotapi.BotAPI) (err error) {
	defer func() {
		e := recover()
		if e != nil {
			err = fmt.Errorf("%v", e)
		}
	}()
	msg := tgbotapi.NewMessage(update.Message.Chat.ID, "正在获取歌曲信息...")
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

	if len(songURL.Data) != len(songDetail.Songs) || len(songDetail.Songs) == 0 || len(songURL.Data) == 0 {
		editMsg := tgbotapi.NewEditMessageText(update.Message.Chat.ID, msgResult.MessageID, "获取详细信息/下载链接失败")
		_, err = bot.Send(editMsg)
		if err != nil {
			logrus.Errorln(err)
		}
		return fmt.Errorf("获取 MusicID: %d 详细信息/下载链接失败", musicid)
	}

	var songInfo SongInfo

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
	songInfo.BitRate = songURL.Data[0].Br

	newEditMsg := tgbotapi.NewEditMessageText(update.Message.Chat.ID, msgResult.MessageID, fmt.Sprintf(musicInfoMsg+waitForDown, songInfo.SongName, songInfo.SongAlbum, songInfo.FileExt, songInfo.FileSize))
	_, err = bot.Send(newEditMsg)
	if err != nil {
		return err
	}

	d, err := New(url, 4)
	if err != nil {
		return err
	}
	err = d.Download()
	if err != nil {
		return err
	}

	editMsg := tgbotapi.NewEditMessageText(update.Message.Chat.ID, msgResult.MessageID, fmt.Sprintf(musicInfoMsg+uploading, songInfo.SongName, songInfo.SongAlbum, songInfo.FileExt, songInfo.FileSize))
	_, err = bot.Send(editMsg)
	if err != nil {
		return err
	}

	marker, _ := downloader.CreateMarker(songDetail.Songs[0], songURL.Data[0])
	switch path.Ext(path.Base(url)) {
	case ".mp3":
		err = downloader.AddMp3Id3v2(cacheDir+"/"+path.Base(url), "", marker, songDetail.Songs[0])
	case ".flac":
		err = downloader.AddFlacId3v2(cacheDir+"/"+path.Base(url), "", marker, songDetail.Songs[0])
	}
	if err != nil {
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

	newAudio := tgbotapi.NewAudio(update.Message.Chat.ID, cacheDir+"/"+path.Base(url))
	newAudio.Caption = fmt.Sprintf(musicInfo, songInfo.SongName, songInfo.SongArtists, songInfo.SongAlbum, songInfo.FileExt, float64(songInfo.BitRate)/1000, botName)
	newAudio.Title = fmt.Sprintf("%s", songInfo.SongName)
	newAudio.Performer = songInfo.SongArtists
	newAudio.Duration = songDetail.Songs[0].Dt / 1000
	newAudio.ReplyMarkup = numericKeyboard

	_, err = bot.Send(newAudio)
	if err != nil {
		editMsg = tgbotapi.NewEditMessageText(update.Message.Chat.ID, msgResult.MessageID, fmt.Sprintf(musicInfoMsg+uploadFailed, songInfo.SongName, songInfo.SongAlbum, songInfo.FileExt, songInfo.FileSize, err))
		_, err = bot.Send(editMsg)
		if err != nil {
			logrus.Errorln(err)
		}
		return err
	}
	return err
}
