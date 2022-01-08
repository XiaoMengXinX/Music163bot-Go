package bot

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/XiaoMengXinX/Music163Api-Go/api"
	"github.com/XiaoMengXinX/Music163Api-Go/types"
	"github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
	"strconv"
	"strings"
	"time"
)

func printAbout(message tgbotapi.Message, bot *tgbotapi.BotAPI) (err error) {
	if config["VersionName"] == "" {
		config["VersionName"] = config["BinVersionName"]
	}
	newMsg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf(aboutText, config["runtimeVer"], config["BinVersionName"], config["commitSHA"], config["buildTime"], config["buildArch"], config["VersionName"]))
	newMsg.ParseMode = tgbotapi.ModeMarkdown
	newMsg.ReplyToMessageID = message.MessageID
	message, err = bot.Send(newMsg)
	if err != nil {
		return err
	}
	return err
}

func rmCache(musicid int, message tgbotapi.Message, bot *tgbotapi.BotAPI) (err error) {
	db := DB.Session(&gorm.Session{})
	var songInfo SongInfo
	err = db.Where("music_id = ?", musicid).First(&songInfo).Error
	if !errors.Is(err, logger.ErrRecordNotFound) {
		db.Where("music_id = ?", musicid).Delete(&songInfo)
		newMsg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf(rmcacheReport, songInfo.SongName))
		newMsg.ReplyToMessageID = message.MessageID
		message, err = bot.Send(newMsg)
		if err != nil {
			return err
		}
		return err
	}
	newMsg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf(noCache))
	newMsg.ReplyToMessageID = message.MessageID
	message, err = bot.Send(newMsg)
	return err
}

func processAnyMusic(message tgbotapi.Message, bot *tgbotapi.BotAPI) (err error) {
	if message.CommandArguments() == "" {
		msg := tgbotapi.NewMessage(message.Chat.ID, inputIDorKeyword)
		msg.ReplyToMessageID = message.MessageID
		_, err = bot.Send(msg)
		return
	}
	musicid, _ := strconv.Atoi(message.CommandArguments())
	if musicid != 0 {
		err = processMusic(musicid, message, bot)
		return err
	}
	searchResult, _ := api.SearchSong(data, api.SearchSongConfig{
		Keyword: message.CommandArguments(),
		Limit:   5,
	})
	if len(searchResult.Result.Songs) == 0 {
		msg := tgbotapi.NewMessage(message.Chat.ID, noResults)
		msg.ReplyToMessageID = message.MessageID
		_, err = bot.Send(msg)
		return err
	}
	err = processMusic(searchResult.Result.Songs[0].Id, message, bot)
	return err
}

func processLyric(message tgbotapi.Message, bot *tgbotapi.BotAPI) (err error) {
	var msgResult tgbotapi.Message
	sendFailed := func() {
		editMsg := tgbotapi.NewEditMessageText(msgResult.Chat.ID, msgResult.MessageID, fmt.Sprintf(getLrcFailed))
		_, err = bot.Send(editMsg)
		if err != nil {
			logrus.Errorln(err)
		}
	}
	if message.CommandArguments() == "" && message.ReplyToMessage == nil {
		msg := tgbotapi.NewMessage(message.Chat.ID, inputContent)
		msg.ReplyToMessageID = message.MessageID
		_, err = bot.Send(msg)
		return err
	} else if message.CommandArguments() == "" && message.ReplyToMessage != nil {
		message = *message.ReplyToMessage
		if !message.IsCommand() && len(message.Entities) != 0 {
			message.Entities[0].Type = "bot_command"
			message.Entities[0].Length = -1
			message.Entities[0].Offset = 0
		} else if !message.IsCommand() && len(message.Entities) == 0 {
			message.Entities = []tgbotapi.MessageEntity{{Type: "bot_command", Length: -1, Offset: 0}}
		}
	}
	msg := tgbotapi.NewMessage(message.Chat.ID, fetchingLyric)
	msg.ReplyToMessageID = message.MessageID
	msgResult, err = bot.Send(msg)
	if err != nil {
		return err
	}

	var replacer = strings.NewReplacer("\n", "", " ", "")
	messageText := replacer.Replace(message.CommandArguments())
	musicid, _ := strconv.Atoi(linkTest(messageText))
	if musicid == 0 {
		searchResult, _ := api.SearchSong(data, api.SearchSongConfig{
			Keyword: message.CommandArguments(),
			Limit:   5,
		})
		if len(searchResult.Result.Songs) == 0 {
			editMsg := tgbotapi.NewEditMessageText(msgResult.Chat.ID, msgResult.MessageID, noResults)
			_, err = bot.Send(editMsg)
			return err
		}
		musicid = searchResult.Result.Songs[0].Id
	}

	b := api.NewBatch(api.BatchAPI{
		Key:  api.SongLyricAPI,
		Json: api.CreateSongLyricReqJson(musicid),
	}, api.BatchAPI{
		Key:  api.SongDetailAPI,
		Json: api.CreateSongDetailReqJson([]int{musicid}),
	}).Do(data)
	if b.Error != nil {
		sendFailed()
		return b.Error
	}

	_, result := b.Parse()
	var lyric types.SongLyricData
	var detail types.SongsDetailData
	_ = json.Unmarshal([]byte(result[api.SongLyricAPI]), &lyric)
	_ = json.Unmarshal([]byte(result[api.SongDetailAPI]), &detail)

	if lyric.Lrc.Lyric != "" && len(detail.Songs) != 0 {
		var replacer = strings.NewReplacer("/", " ", "?", " ", "*", " ", ":", " ", "|", " ", "\\", " ", "<", " ", ">", " ", "\"", " ")
		lrcPath := fmt.Sprintf("%s/%s - %s.lrc", cacheDir, replacer.Replace(parseArtist(detail.Songs[0])), replacer.Replace(detail.Songs[0].Name))
		file, err := os.OpenFile(lrcPath, os.O_WRONLY|os.O_CREATE, 0666)
		if err != nil {
			sendFailed()
			return err
		} else {
			defer func(file *os.File) {
				_ = file.Close()
			}(file)
			write := bufio.NewWriter(file)
			_, _ = write.WriteString(lyric.Lrc.Lyric)
			err = write.Flush()
			if err != nil {
				sendFailed()
				return err
			}
		}
		defer func(name string) {
			err := os.Remove(name)
			if err != nil {
				logrus.Errorln(err)
			}
		}(lrcPath)
		var newFile tgbotapi.DocumentConfig
		newFile = tgbotapi.NewDocument(message.Chat.ID, tgbotapi.FilePath(lrcPath))
		_, err = bot.Send(newFile)
		if err != nil {
			return err
		}
		deleteMsg := tgbotapi.NewDeleteMessage(msgResult.Chat.ID, msgResult.MessageID)
		_, err = bot.Request(deleteMsg)
		return err
	}
	sendFailed()
	return
}

var statusChan = make(chan bool, 1)

func processStatus(message tgbotapi.Message, bot *tgbotapi.BotAPI) (err error) {
	statusChan <- true
	defer func() {
		time.Sleep(time.Millisecond * 500)
		<-statusChan
	}()
	db := DB.Session(&gorm.Session{})
	var fromCount, chatCount int64
	var lastRecord SongInfo
	db.Where("from_user_id = ?", message.From.ID).Count(&fromCount)
	db.Where("from_chat_id = ?", message.Chat.ID).Count(&chatCount)
	db.Last(&lastRecord)

	var chatInfo string
	if message.Chat.UserName != "" && message.Chat.Title == "" {
		chatInfo = fmt.Sprintf("[%s](tg://user?id=%d)", mdV2Replacer.Replace(message.Chat.UserName), message.Chat.ID)
	} else if message.Chat.UserName != "" {
		chatInfo = fmt.Sprintf("[%s](https://t.me/%s)", mdV2Replacer.Replace(message.Chat.Title), message.Chat.UserName)
	} else {
		chatInfo = fmt.Sprintf("%s", mdV2Replacer.Replace(message.Chat.Title))
	}
	msgText := fmt.Sprintf(statusInfo, lastRecord.ID, chatInfo, chatCount, message.From.ID, message.From.ID, fromCount)
	msg := tgbotapi.NewMessage(message.Chat.ID, msgText)
	msg.ReplyToMessageID = message.MessageID
	msg.ParseMode = tgbotapi.ModeMarkdownV2
	_, err = bot.Send(msg)
	return err
}
