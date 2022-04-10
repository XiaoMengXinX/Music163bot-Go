package bot

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/XiaoMengXinX/Music163Api-Go/utils"
	"github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sirupsen/logrus"
)

// Start bot entry
func Start(conf map[string]string) (actionCode int) {
	config = conf
	defer func() {
		e := recover()
		if e != nil {
			logrus.Errorln(e)
			actionCode = 1
		}
	}()
	// 创建缓存文件夹
	dirExists(cacheDir)

	// 解析 bot 管理员配置
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

	// 初始化数据库
	err := initDB(config)
	if err != nil {
		logrus.Errorln(err)
		return 1
	}

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
	if config["BotAPI"] != "" {
		botAPI = config["BotAPI"]
	}

	if config["AutoUpdate"] != "false" {
		meta, isLatest, err := getUpdate()
		if err != nil {
			logrus.Errorf("%s, 尝试重新下载更新中", err)
			e := os.Remove(fmt.Sprintf("%s/version.json", config["SrcPath"]))
			if e != nil {
				logrus.Errorln(e)
			}
			return 2
		} else if !isLatest {
			return 2
		}
		if meta.Unsupported {
			for _, i := range botAdmin {
				msg := tgbotapi.NewMessage(int64(i), fmt.Sprintf(updateBinVer, config["repoPath"]))
				_, _ = bot.Send(msg)
			}
		}
	}

	if maxRetryTimes, _ = strconv.Atoi(config["MaxRetryTimes"]); maxRetryTimes <= 0 {
		maxRetryTimes = 3
	}
	if downloaderTimeout, _ = strconv.Atoi(config["DownloadTimeout"]); downloaderTimeout <= 0 {
		downloaderTimeout = 60
	}

	// 设置 bot 日志接口
	err = tgbotapi.SetLogger(logrus.StandardLogger())
	if err != nil {
		logrus.Errorln(err)
		return 1
	}
	// 配置 token、api、debug
	bot, err = tgbotapi.NewBotAPIWithAPIEndpoint(config["BOT_TOKEN"], botAPI+"/bot%s/%s")
	if err != nil {
		logrus.Errorln(err)
		return 1
	}
	if config["BotDebug"] == "true" {
		bot.Debug = true
	}

	logrus.Printf("%s 验证成功", bot.Self.UserName)
	botName = bot.Self.UserName

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)
	defer bot.StopReceivingUpdates()

	for update := range updates {
		if update.Message == nil && update.CallbackQuery == nil && update.InlineQuery == nil { // ignore any non-Message Updates
			continue
		}
		switch {
		case update.Message != nil:
			updateMsg := *update.Message
			if atStr := strings.ReplaceAll(update.Message.CommandWithAt(), update.Message.Command(), ""); update.Message.Command() != "" && (atStr == "" || atStr == "@"+botName) {
				switch update.Message.Command() {
				case "start":
					if !updateMsg.Chat.IsPrivate() {
						return
					}
					go func() {
						musicID, _ := strconv.Atoi(updateMsg.CommandArguments())
						if musicID == 0 {
							return
						}
						err := processMusic(musicID, updateMsg, bot)
						if err != nil {
							logrus.Errorln(err)
						}
					}()
				case "music", "netease":
					go func() {
						err := processAnyMusic(updateMsg, bot)
						if err != nil {
							logrus.Errorln(err)
						}
					}()
				case "program":
					go func() {
						id, _ := strconv.Atoi(updateMsg.CommandArguments())
						musicID := getProgramRealID(id)
						if musicID != 0 {
							err := processMusic(musicID, updateMsg, bot)
							if err != nil {
								logrus.Errorln(err)
							}
						}
					}()
				case "lyric":
					go func() {
						err := processLyric(updateMsg, bot)
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
				case "status":
					go func() {
						err := processStatus(updateMsg, bot)
						if err != nil {
							logrus.Errorln(err)
						}
					}()
				case "setting":
					go func() {
						err := processSettings(updateMsg, bot)
						if err != nil {
							logrus.Errorln(err)
						}
					}()
				}
				if in(fmt.Sprintf("%d", update.Message.From.ID), botAdminStr) {
					switch update.Message.Command() {
					case "rmcache":
						go func() {
							err := processRmCache(updateMsg, bot)
							if err != nil {
								logrus.Errorln(err)
							}
						}()
					case "reload":
						msg := tgbotapi.NewMessage(update.Message.Chat.ID, reloading)
						msg.ReplyToMessageID = update.Message.MessageID
						_, _ = bot.Send(msg)
						return 2
					case "update":
						msg := tgbotapi.NewMessage(update.Message.Chat.ID, checkingUpdate)
						msg.ReplyToMessageID = update.Message.MessageID
						msgResult, _ := bot.Send(msg)
						meta, isLatest, err := getUpdate()
						if err == nil {
							if isLatest {
								editMsg := tgbotapi.NewEditMessageText(msgResult.Chat.ID, msgResult.MessageID, fmt.Sprintf(isLatestVer, meta.Version, meta.VersionCode))
								_, _ = bot.Send(editMsg)
							} else {
								editMsg := tgbotapi.NewEditMessageText(msgResult.Chat.ID, msgResult.MessageID, fmt.Sprintf(updatedToVer, meta.Version, meta.VersionCode))
								_, _ = bot.Send(editMsg)
								return 2
							}
						} else {
							editMsg := tgbotapi.NewEditMessageText(msgResult.Chat.ID, msgResult.MessageID, fmt.Sprintln(err))
							_, _ = bot.Send(editMsg)
							logrus.Errorln(err)
						}
					}
				}
			} else if strings.Contains(update.Message.Text, "music.163.com") {
				go func() {
					id := parseMusicID(updateMsg.Text)
					if id != 0 {
						err := processMusic(id, updateMsg, bot)
						if err != nil {
							logrus.Errorln(err)
						}
					} else if id = parseProgramID(updateMsg.Text); id != 0 {
						if id = getProgramRealID(id); id != 0 {
							err := processMusic(id, updateMsg, bot)
							if err != nil {
								logrus.Errorln(err)
							}
						}
					}
				}()
			}
		case update.CallbackQuery != nil:
			updateQuery := *update.CallbackQuery
			args := strings.Split(updateQuery.Data, " ")
			if len(args) < 2 {
				continue
			}
			switch args[0] {
			case "music":
				go func() {
					err := processCallbackMusic(args, updateQuery, bot)
					if err != nil {
						logrus.Errorln(err)
					}
				}()
			case "get":
				go func() {
					err := processSettingGet(args, updateQuery, bot)
					if err != nil {
						logrus.Errorln(err)
					}
				}()
			case "set":
				go func() {
					err := processSettingSet(args, updateQuery, bot)
					if err != nil {
						logrus.Errorln(err)
					}
				}()
			}
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
					musicID, _ := strconv.Atoi(linkTestMusic(updateQuery.Query))
					if musicID != 0 {
						err = processInlineMusic(musicID, updateQuery, bot)
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
	}
	return 0
}
