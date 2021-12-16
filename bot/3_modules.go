package bot

import (
	"errors"
	"fmt"
	"github.com/XiaoMengXinX/Music163Api-Go/api"
	"github.com/XiaoMengXinX/Music163bot-Go/v2/util"
	"github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"strconv"
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
	var songInfo util.SongInfo
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
