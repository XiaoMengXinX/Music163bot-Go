package bot

import (
	"github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sirupsen/logrus"
	"time"
)

func Start() {
	tgbotapi.SetLogger(logrus.StandardLogger())
	bot, err := tgbotapi.NewBotAPI("1701038738:AAHUYczwWO-8eLZVjogSQfnqM8gDYc09HlY1")
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

		logrus.Printf("[%s] %s", update.Message.From.UserName, update.Message.Text)

		msg := tgbotapi.NewMessage(update.Message.Chat.ID, update.Message.Text)
		msg.ReplyToMessageID = update.Message.MessageID

		time.Sleep(time.Duration(5) * time.Second)

		a, _ := bot.Send(msg)

		time.Sleep(time.Duration(5) * time.Second)

		editmsg := tgbotapi.NewEditMessageText(update.Message.Chat.ID, a.MessageID, "update.Message.Text")
		bot.Send(editmsg)
	}
}
