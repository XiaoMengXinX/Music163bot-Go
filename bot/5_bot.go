package bot

import (
	"fmt"
	"github.com/XiaoMengXinX/Music163Api-Go/utils"
	"github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"
)

var bot *tgbotapi.BotAPI
var botAdmin []int
var botAdminStr []string

// Start bot entry
func Start(conf map[string]string, ext func(*tgbotapi.BotAPI, tgbotapi.Update) error) (actionCode int) {
	config = conf
	defer func() {
		e := recover()
		if e != nil {
			logrus.Errorln(e)
			actionCode = 1
		}
	}()
	botAdminStr = strings.Split(config["BotAdmin"], ",")
	if len(botAdminStr) == 0 && config["BotAdmin"] != "" {
		botAdminStr = []string{config["BotAdmin"]}
	}
	if len(botAdminStr) != 0 {
		for _, s := range botAdminStr {
			id, err := strconv.Atoi(s)
			if err == nil {
				botAdmin = append(botAdmin, id)
			}
		}
	}

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
	if err != nil {
		logrus.Fatal(err)
	}
	bot, err = tgbotapi.NewBotAPI(config["BOT_TOKEN"])
	if err != nil {
		logrus.Fatal(err)
	}

	if config["BotAPI"] != "" {
		bot.SetAPIEndpoint(config["BotAPI"] + "/bot%s/%s")
	}

	if config["BotDebug"] == "true" {
		bot.Debug = true
	}

	if maxRedownTimes, _ = strconv.Atoi(config["MaxRedownTimes"]); maxRedownTimes <= 0 {
		maxRedownTimes = 3
	}

	logrus.Printf("%s 验证成功", bot.Self.UserName)
	botName = bot.Self.UserName
	defer bot.StopReceivingUpdates()

	if config["AutoUpdate"] != "false" {
		err := fmt.Errorf("")
		var meta metadata
		meta, err = getUpdate()
		if err != nil {
			logrus.Errorf("%s, 尝试重新下载更新中", err)
			e := os.Remove(fmt.Sprintf("%s/version.json", config["SrcPath"]))
			if e != nil {
				logrus.Errorln(e)
			}
			return 2
		}
		if meta.VersionCode < 20200 {
			for _, i := range botAdmin {
				msg := tgbotapi.NewMessage(int64(i), updateBinVersion)
				_, _ = bot.Send(msg)
			}
		}
	}

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil && update.CallbackQuery == nil && update.InlineQuery == nil { // ignore any non-Message Updates
			continue
		}
		switch {
		case update.Message != nil:
			updateMsg := *update.Message
			if update.Message.Command() != "" {
				switch update.Message.Command() {
				case "musicid", "netease", "start":
					if updateMsg.Command() == "start" && !updateMsg.Chat.IsPrivate() {
						return
					}
					go func() {
						musicid, _ := strconv.Atoi(updateMsg.CommandArguments())
						if musicid == 0 {
							return
						}
						err := processMusic(musicid, updateMsg, bot)
						if err != nil {
							logrus.Errorln(err)
						}
					}()
				case "search":
					go func() {
						err := processSearch(updateMsg, bot)
						if err != nil {
							logrus.Errorln(err)
						}
					}()
				case "about":
					go func() {
						err := printAbout(updateMsg, bot)
						if err != nil {
							logrus.Errorln(err)
						}
					}()
				}
				if in(fmt.Sprintf("%d", update.Message.From.ID), botAdminStr) {
					switch update.Message.Command() {
					case "loadext":
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
					case "rmcache":
						go func() {
							var replacer = strings.NewReplacer("\n", "", " ", "")
							messageText := replacer.Replace(updateMsg.CommandArguments())
							musicid, _ := strconv.Atoi(linkTest(messageText))
							if musicid == 0 {
								return
							}
							err := rmCache(musicid, updateMsg, bot)
							if err != nil {
								logrus.Errorln(err)
							}
						}()
					case "reload":
						msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Reloading...")
						msg.ReplyToMessageID = update.Message.MessageID
						_, _ = bot.Send(msg)
						return 2
					case "update":
						msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Trying update...")
						msg.ReplyToMessageID = update.Message.MessageID
						_, _ = bot.Send(msg)
						_, err := getUpdate()
						if err != nil {
							return 2
						}
					case "stop":
						msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Stopping main thread...")
						msg.ReplyToMessageID = update.Message.MessageID
						_, _ = bot.Send(msg)
						return 0
					}
				} else if strings.Contains(update.Message.Text, "music.163.com") {
					go func() {
						var replacer = strings.NewReplacer("\n", "", " ", "")
						messageText := replacer.Replace(updateMsg.Text) // 去除消息内空格和换行 避免不必要的麻烦（
						musicid, _ := strconv.Atoi(linkTest(messageText))
						if musicid == 0 {
							return
						}
						err := processMusic(musicid, updateMsg, bot)
						if err != nil {
							logrus.Errorln(err)
						}
					}()
				}
			}
		case update.CallbackQuery != nil:
			updateQuery := *update.CallbackQuery
			go func() {
				musicid, _ := strconv.Atoi(updateQuery.Data)
				if updateQuery.Message.Chat.IsPrivate() {
					callback := tgbotapi.NewCallback(updateQuery.ID, "Success")
					_, err := bot.Request(callback)
					if err != nil {
						logrus.Errorln(err)
					}
					message := *updateQuery.Message
					err = processMusic(musicid, message, bot)
					if err != nil {
						logrus.Errorln(err)
					}
				} else {
					callback := tgbotapi.NewCallback(updateQuery.ID, "Success")
					callback.URL = fmt.Sprintf("t.me/%s?start=%d", botName, musicid)
					_, err := bot.Request(callback)
					if err != nil {
						logrus.Errorln(err)
					}
				}
			}()
		case update.InlineQuery != nil:
			updateQuery := *update.InlineQuery
			switch {
			case updateQuery.Query == "help":
				go func() {
					err = processInlineHelp(updateQuery, bot)
					if err != nil {
						logrus.Errorln(err)
					}
				}()
			case strings.Contains(updateQuery.Query, "search"):
				go func() {
					err = processInlineSearch(updateQuery, bot)
					if err != nil {
						logrus.Errorln(err)
					}
				}()
			default:
				go func() {
					musicid, _ := strconv.Atoi(linkTest(updateQuery.Query))
					if musicid != 0 {
						err = processInlineMusic(musicid, updateQuery, bot)
						if err != nil {
							logrus.Errorln(err)
						}
					} else {
						err = processEmptyInline(updateQuery, bot)
						if err != nil {
							logrus.Errorln(err)
						}
					}
				}()
			}
		}

		if config["EnableExt"] == "true" && ext != nil {
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
