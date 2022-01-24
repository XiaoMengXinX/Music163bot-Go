package bot

import (
	"fmt"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"gorm.io/gorm"
	"strconv"
)

func getSettings(sType int, ID int64) (settings Settings, err error) {
	db := SettingDB.Session(&gorm.Session{})
	err = db.Where("type = ? AND chat_id = ?", sType, ID).First(&settings).Error
	if err != nil {
		settings = Settings{
			Type:       sType,
			ChatID:     ID,
			SourceInfo: true,
			ShareKey:   true,
		}
		return settings, db.Create(&settings).Error
	}
	return settings, err
}

func saveSettings(settings Settings) (err error) {
	db := SettingDB.Session(&gorm.Session{})
	return db.Where("type = ? AND chat_id = ?", settings.Type, settings.ChatID).First(&Settings{}).Save(settings).Error
}

func processSettings(message tgbotapi.Message, bot *tgbotapi.BotAPI) (err error) {
	if !message.Chat.IsPrivate() {
		number, err := bot.GetChatMember(tgbotapi.GetChatMemberConfig{ChatConfigWithUser: tgbotapi.ChatConfigWithUser{
			ChatID: message.Chat.ID,
			UserID: message.From.ID,
		}})
		if err != nil {
			return err
		}
		if !number.IsAdministrator() && !in(fmt.Sprintf("%d", message.From.ID), botAdminStr) {
			msg := tgbotapi.NewMessage(message.Chat.ID, noPermission)
			msg.ReplyToMessageID = message.MessageID
			_, err = bot.Send(msg)
			return err
		}
	}
	var inlineKeys [][]tgbotapi.InlineKeyboardButton
	inlineKeys = append(inlineKeys, []tgbotapi.InlineKeyboardButton{tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf(chatSettingText), fmt.Sprintf("get %d %d", ChatSetting, message.Chat.ID))})
	if message.Chat.IsPrivate() {
		inlineKeys = append(inlineKeys, []tgbotapi.InlineKeyboardButton{tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf(userSettingText), fmt.Sprintf("get %d %d", UserSetting, message.From.ID))})
		if in(fmt.Sprintf("%d", message.From.ID), botAdminStr) {
			inlineKeys = append(inlineKeys, []tgbotapi.InlineKeyboardButton{tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf(globalSettingText), fmt.Sprintf("get %d %d", GlobalSetting, 0))})
		}
	}
	var numericKeyboard = tgbotapi.NewInlineKeyboardMarkup(inlineKeys...)
	if !message.From.IsBot {
		msg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf(settings, bot.Self.UserName))
		msg.ReplyMarkup = numericKeyboard
		msg.ReplyToMessageID = message.MessageID
		_, err = bot.Send(msg)
	} else {
		msg := tgbotapi.NewEditMessageText(message.Chat.ID, message.MessageID, fmt.Sprintf(settings, bot.Self.UserName))
		msg.ReplyMarkup = &numericKeyboard
		_, err = bot.Send(msg)
	}
	return err
}

func processSettingGet(args []string, query tgbotapi.CallbackQuery, bot *tgbotapi.BotAPI) (err error) {
	var msgText string
	var inlineKeys [][]tgbotapi.InlineKeyboardButton
	if len(args) < 3 {
		return err
	}
	sType, _ := strconv.Atoi(args[1])
	ID, _ := strconv.Atoi(args[2])
	var setting Settings
	switch sType {
	case UserSetting:
		if query.From.ID != int64(ID) {
			return err
		}
		msgText = userSettingText
	case ChatSetting:
		if !query.Message.Chat.IsPrivate() {
			number, err := bot.GetChatMember(tgbotapi.GetChatMemberConfig{ChatConfigWithUser: tgbotapi.ChatConfigWithUser{
				ChatID: query.Message.Chat.ID,
				UserID: query.From.ID,
			}})
			if err != nil {
				return err
			}
			if !number.IsAdministrator() && !in(fmt.Sprintf("%d", query.From.ID), botAdminStr) {
				callback := tgbotapi.NewCallback(query.ID, noPermission)
				_, err = bot.Request(callback)
				return err
			}
		}
		msgText = chatSettingText
	case GlobalSetting:
		if !in(fmt.Sprintf("%d", query.From.ID), botAdminStr) {
			callback := tgbotapi.NewCallback(query.ID, noPermission)
			_, err = bot.Request(callback)
			return err
		}
		msgText = globalSettingText
		ID = 0
	case 0:
		message := *query.Message
		message.From.ID = int64(ID)
		return processSettings(message, bot)
	default:
		return err
	}
	setting, err = getSettings(sType, int64(ID))
	if err != nil {
		return err
	}
	inlineKeys = append(inlineKeys, []tgbotapi.InlineKeyboardButton{tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf(sourceInfo, parseSettingBool(setting.SourceInfo)), fmt.Sprintf("set %d %d %s %t", sType, ID, setSourceInfo, !setting.SourceInfo))})
	inlineKeys = append(inlineKeys, []tgbotapi.InlineKeyboardButton{tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf(shareKey, parseSettingBool(setting.ShareKey)), fmt.Sprintf("set %d %d %s %t", sType, ID, setShareKey, !setting.ShareKey))})
	inlineKeys = append(inlineKeys, []tgbotapi.InlineKeyboardButton{tgbotapi.NewInlineKeyboardButtonData(back, fmt.Sprintf("get 0 %d", query.From.ID))})
	var numericKeyboard = tgbotapi.NewInlineKeyboardMarkup(inlineKeys...)
	msg := tgbotapi.NewEditMessageText(query.Message.Chat.ID, query.Message.MessageID, msgText)
	msg.ReplyMarkup = &numericKeyboard
	_, err = bot.Send(msg)
	return err
}

func processSettingSet(args []string, query tgbotapi.CallbackQuery, bot *tgbotapi.BotAPI) (err error) {
	if len(args) < 5 {
		return err
	}
	sType, _ := strconv.Atoi(args[1])
	ID, _ := strconv.Atoi(args[2])
	switch sType {
	case UserSetting:
		if query.From.ID != int64(ID) {
			return err
		}
	case ChatSetting:
		number, err := bot.GetChatMember(tgbotapi.GetChatMemberConfig{ChatConfigWithUser: tgbotapi.ChatConfigWithUser{
			ChatID: query.Message.Chat.ID,
			UserID: query.From.ID,
		}})
		if err != nil {
			return err
		}
		if !number.IsAdministrator() && !in(fmt.Sprintf("%d", query.From.ID), botAdminStr) {
			callback := tgbotapi.NewCallback(query.ID, noPermission)
			_, err = bot.Request(callback)
			return err
		}
	case GlobalSetting:
		if !in(fmt.Sprintf("%d", query.From.ID), botAdminStr) {
			callback := tgbotapi.NewCallback(query.ID, noPermission)
			_, err = bot.Request(callback)
			return err
		}
	default:
		return err
	}
	setArg := args[3]
	setBool, _ := strconv.ParseBool(args[4])
	setting, err := getSettings(sType, int64(ID))
	if err != nil {
		return err
	}
	switch setArg {
	case setSourceInfo:
		setting.SourceInfo = setBool
	case setShareKey:
		setting.ShareKey = setBool
	}
	err = saveSettings(setting)
	if err != nil {
		return err
	}
	callback := tgbotapi.NewCallback(query.ID, setSuccess)
	_, err = bot.Request(callback)
	return processSettingGet(args, query, bot)
}

func parseSettingBool(b bool) string {
	if b {
		return enabled
	}
	return disabled
}
