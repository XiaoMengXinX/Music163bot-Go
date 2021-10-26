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
)

var bot *tgbotapi.BotAPI
var botAdmin []string

func Start(conf map[string]string, ext func(*tgbotapi.BotAPI, tgbotapi.Update) error) (actionCode int) {
	config = conf
	defer func() {
		e := recover()
		if e != nil {
			logrus.Errorln(e)
			actionCode = 1
		} else {
			bot.StopReceivingUpdates()
		}
	}()
	botAdmin = strings.Split(config["BotAdmin"], ",")

	initDB(config)

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

	err := tgbotapi.SetLogger(logrus.StandardLogger())
	bot, err = tgbotapi.NewBotAPI(config["BOT_TOKEN"])
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
				go func() {
					err := processMusic(musicid, update, bot)
					if err != nil {
						logrus.Errorln(err)
					}
				}()
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
					return 2
				case "update":
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Trying update...")
					msg.ReplyToMessageID = update.Message.MessageID
					_, _ = bot.Send(msg)
					return 3
				case "stop":
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Stoping main thread...")
					msg.ReplyToMessageID = update.Message.MessageID
					_, _ = bot.Send(msg)
					return 0
				case "panic":
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Panic: %s", update.Message.CommandArguments()))
					msg.ReplyToMessageID = update.Message.MessageID
					_, _ = bot.Send(msg)
					panic(update.Message.CommandArguments())
				}
			}
		case strings.Contains(update.Message.Text, "music.163.com"):
			var replacer = strings.NewReplacer("\n", "", " ", "")
			messageText := replacer.Replace(update.Message.Text) // 去除消息内空格和换行 避免不必要的麻烦（
			musicid, _ := strconv.Atoi(linkTest(messageText))
			if musicid == 0 {
				continue
			}
			go func() {
				err := processMusic(musicid, update, bot)
				if err != nil {
					logrus.Errorln(err)
				}
			}()
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
