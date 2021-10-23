package bot

import (
	"fmt"
	"github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	"time"
)

func Start(config map[string]string, ext func(*tgbotapi.BotAPI, tgbotapi.Update) error) (actionCode int) {
	_ = tgbotapi.SetLogger(logrus.StandardLogger())
	bot, err := tgbotapi.NewBotAPI(config["BOT_TOKEN"])
	if err != nil {
		logrus.Fatal(err)
	}
	bot.Debug = true
	logrus.Printf("Authorized on account %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil { // ignore any non-Message Updates
			continue
		}

		switch update.Message.Command() {
		case "ping":
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, "pong")
			msg.ReplyToMessageID = update.Message.MessageID
			_, err := bot.Send(msg)
			if err != nil {
				logrus.Errorln(err)
			}
		case "loadExt":
			extFile := update.Message.CommandArguments()
			extData := update.Message.ReplyToMessage.Text
			err := ioutil.WriteFile(fmt.Sprintf("%s/%s", config["ExtPath"], extFile), []byte(extData), 0644)
			if err != nil {
				logrus.Errorln(err)
			} else {
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Extension saved to %s/%s", config["ExtPath"], extFile))
				msg.ReplyToMessageID = update.Message.MessageID
				_, _ = bot.Send(msg)
			}
		case "reload":
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Reloading...")
			msg.ReplyToMessageID = update.Message.MessageID
			_, _ = bot.Send(msg)
			bot.StopReceivingUpdates()
			time.Sleep(2 * time.Second)
			return 2
		}

		func() {
			defer func() {
				if err := recover(); err != nil {
					logrus.Errorln(err)
				}
			}()
			err := ext(bot, update)
			if err != nil {
				logrus.Errorln(err)
			}
		}()
	}

	return 0
}
