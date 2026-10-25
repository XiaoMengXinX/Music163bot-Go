package bot

import (
	"context"
	"strings"

	tgb "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// 消息发送 helper，统一 HTML parse mode

func (a *App) sendMessage(ctx context.Context, chatID int64, text string, replyToMessageID int) (*models.Message, error) {
	params := &tgb.SendMessageParams{
		ChatID:    chatID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
	}
	if replyToMessageID != 0 {
		params.ReplyParameters = &models.ReplyParameters{MessageID: replyToMessageID}
	}
	return a.tg.SendMessage(ctx, params)
}

// reply 回复某条消息
func (a *App) reply(ctx context.Context, m *models.Message, text string) (*models.Message, error) {
	return a.sendMessage(ctx, m.Chat.ID, text, m.ID)
}

func (a *App) editMessage(ctx context.Context, chatID int64, messageID int, text string, markup models.ReplyMarkup) error {
	_, err := a.tg.EditMessageText(ctx, &tgb.EditMessageTextParams{
		ChatID:      chatID,
		MessageID:   messageID,
		Text:        text,
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: markup,
	})
	if err != nil && strings.Contains(err.Error(), "message is not modified") {
		return nil
	}
	return err
}

func (a *App) deleteMessage(ctx context.Context, chatID int64, messageID int) error {
	_, err := a.tg.DeleteMessage(ctx, &tgb.DeleteMessageParams{ChatID: chatID, MessageID: messageID})
	return err
}
