package ext

import (
	"github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// CustomScript A demo for extension
func CustomScript(bot *tgbotapi.BotAPI, update tgbotapi.Update) error {
	if update.Message.Text == "sdl" {
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "tql!")
		msg.ReplyToMessageID = update.Message.MessageID
		bot.Send(msg)
	}
	return nil
}
