package bot

import (
	"fmt"
	"net/http"
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
	db, _ := MusicDB.DB()
	db.SetMaxOpenConns(1)
	defer db.Close()

	// 初始化点播统计数据库（独立文件，失败不影响 bot 主功能）
	if err = initStats(config); err != nil {
		logrus.Errorln("初始化统计数据库失败:", err)
	}

	// PanelOnly = true：只启动统计面板（不连接 Telegram），用于离线调试面板与回填
	if strings.EqualFold(strings.TrimSpace(config["PanelOnly"]), "true") {
		statsPanelStart(config)
		logrus.Infoln("PanelOnly 模式：仅运行统计面板与统计库，不连接 Telegram")
		select {}
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

	// 启动统计面板（可在 config.ini 中用 PanelEnable = false 关闭）
	statsPanelStart(config)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)
	defer bot.StopReceivingUpdates()

	for update := range updates {
		if update.Message == nil && update.CallbackQuery == nil && update.InlineQuery == nil &&
			update.MyChatMember == nil && update.ChosenInlineResult == nil { // ignore any non-Message Updates
			continue
		}
		switch {
		case update.Message != nil:
			updateMsg := *update.Message
			recordChatSeen(updateMsg.Chat) // 统计：收到消息即证明 bot 在该会话内
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
				case "recognize":
					go func() {
						err := recognizeMusic(updateMsg, bot)
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
				case "panel":
					// 管理员私聊申请统计面板的一次性访问令牌
					go func() {
						err := processPanelCommand(updateMsg, bot)
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
					}
				}
			} else if strings.Contains(update.Message.Text, "music.163.com") || strings.Contains(update.Message.Text, "163cn.tv") || strings.Contains(update.Message.Text, "163cn.link") {
				go func() {
					var text = update.Message.Text
					if strings.Contains(update.Message.Text, "163cn.tv") || strings.Contains(update.Message.Text, "163cn.link") {
						text = getRedirectUrl(text)
					}
					id := parseMusicID(text)
					if id != 0 {
						err := processMusic(id, updateMsg, bot)
						if err != nil {
							logrus.Errorln(err)
						}
					} else if id = parseProgramID(text); id != 0 {
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
			if updateQuery.Message != nil {
				recordChatSeen(updateQuery.Message.Chat)
			}
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
		case update.ChosenInlineResult != nil:
			// inline 模式下用户选用了 bot 的歌曲：可统计曲目，但拿不到所在会话
			updateResult := *update.ChosenInlineResult
			go recordChosenInline(updateResult.ResultID, updateResult.From)
		case update.MyChatMember != nil:
			// bot 自身被加入/移出群：判断「在哪些群」最准确的依据
			updateMember := *update.MyChatMember
			status := updateMember.NewChatMember.Status
			inChat := status == "member" || status == "administrator" || status == "creator" || status == "restricted"
			recordChatStatus(&updateMember.Chat, inChat)
			if !inChat {
				logrus.Infof("bot 已离开会话 %v (%s)", updateMember.Chat.ID, chatTitleOf(&updateMember.Chat))
			}
		}
	}
	return 0
}
