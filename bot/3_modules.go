package bot

import (
	"errors"
	"fmt"
	"github.com/XiaoMengXinX/Music163Api-Go/api"
	"github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"strconv"
	"time"
)

// 限制查询速度及并发
var statLimiter = make(chan bool, 1)

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

func processCallbackMusic(args []string, updateQuery tgbotapi.CallbackQuery, bot *tgbotapi.BotAPI) (err error) {
	musicID, _ := strconv.Atoi(args[1])
	if updateQuery.Message.Chat.IsPrivate() {
		callback := tgbotapi.NewCallback(updateQuery.ID, callbackText)
		_, err = bot.Request(callback)
		if err != nil {
			return err
		}
		message := *updateQuery.Message
		return processMusic(musicID, message, bot)
	}
	callback := tgbotapi.NewCallback(updateQuery.ID, callbackText)
	callback.URL = fmt.Sprintf("t.me/%s?start=%d", botName, musicID)
	_, err = bot.Request(callback)
	return err
}

func processRmCache(message tgbotapi.Message, bot *tgbotapi.BotAPI) (err error) {
	musicID := parseID(message.CommandArguments())
	if musicID == 0 {
		return err
	}
	db := MusicDB.Session(&gorm.Session{})
	var songInfo SongInfo
	err = db.Where("music_id = ?", musicID).First(&songInfo).Error
	if !errors.Is(err, logger.ErrRecordNotFound) {
		db.Where("music_id = ?", musicID).Delete(&songInfo)
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
	musicID, _ := strconv.Atoi(message.CommandArguments())
	if musicID != 0 {
		err = processMusic(musicID, message, bot)
		return err
	}
	searchResult, _ := api.SearchSong(data, api.SearchSongConfig{
		Keyword: message.CommandArguments(),
		Limit:   10,
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

func processStatus(message tgbotapi.Message, bot *tgbotapi.BotAPI) (err error) {
	statLimiter <- true
	defer func() {
		time.Sleep(time.Millisecond * 500)
		<-statLimiter
	}()
	db := MusicDB.Session(&gorm.Session{})
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
