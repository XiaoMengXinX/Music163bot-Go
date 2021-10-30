package bot

import (
	"errors"
	"fmt"
	"github.com/XiaoMengXinX/Music163bot-Go/v2/util"
	"github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func printAbout(message tgbotapi.Message, bot *tgbotapi.BotAPI) (err error) {
	if config["VersionName"] == "" {
		config["VersionName"] = config["BinVersionName"]
	}
	newMsg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf(aboutText, config["runtimeVer"], config["BinVersionName"], config["commitSHA"], config["buildTime"], config["buildOS"], config["buildArch"], config["VersionName"]))
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
	} else {
		newMsg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf(noCache))
		newMsg.ReplyToMessageID = message.MessageID
		message, err = bot.Send(newMsg)
		if err != nil {
			return err
		}
	}

	return err
}
