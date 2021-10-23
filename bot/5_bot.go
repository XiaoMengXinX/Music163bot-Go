package bot

import (
	"fmt"
	"github.com/XiaoMengXinX/Music163Api-Go/utils"
	"github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var botAdmin []string

func Start(config map[string]string, ext func(*tgbotapi.BotAPI, tgbotapi.Update) error) (actionCode int) {
	botAdmin = strings.Split(config["BotAdmin"], ",")

	if config["MUSIC_U"] != "" {
		data = utils.RequestData{
			Cookies: []*http.Cookie{
				{
					Name:  "MUSIC_U",
					Value: config["MUSIC_U"],
				},
			},
		}
	}

	_ = tgbotapi.SetLogger(logrus.StandardLogger())
	bot, err := tgbotapi.NewBotAPI(config["BOT_TOKEN"])
	if err != nil {
		logrus.Fatal(err)
	}

	if config["BotDebug"] == "true" {
		bot.Debug = true
	}

	logrus.Printf("%s 验证成功", bot.Self.UserName)
	botName = bot.Self.UserName

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil { // ignore any non-Message Updates
			continue
		}
		switch {
		case update.Message.Command() != "":
			switch update.Message.Command() {
			case "musicid", "netease", "start":
				musicid, _ := strconv.Atoi(update.Message.CommandArguments())
				if musicid == 0 {
					continue
				}
				err := processMusic(musicid, update, bot)
				if err != nil {
					logrus.Errorln(err)
				}
			}

			if in(fmt.Sprintf("%d", update.Message.From.ID), botAdmin) {
				switch update.Message.Command() {
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
				case "update":
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Trying update...")
					msg.ReplyToMessageID = update.Message.MessageID
					_, _ = bot.Send(msg)
					bot.StopReceivingUpdates()
					time.Sleep(2 * time.Second)
					return 3
				}
			}
		}

		if config["EnableExt"] == "true" {
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
	}
	return 0
}
