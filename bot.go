package main

import (
	"errors"
	"fmt"
	downloader "github.com/XiaoMengXinX/NeteaseCloudApi-Go/tools/SongDownloader/utils"
	"github.com/XiaoMengXinX/NeteaseCloudApi-Go/utils"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"path"
	"strconv"
	"strings"
	"time"
)

func main() {
	options = make(map[string]interface{})
	cookies = make(map[string]interface{}) // 初始化 cookie

	cookies["MUSIC_U"] = config["MUSIC_U"]
	options["cookie"] = cookies
	options["savePath"] = musicPath
	options["pic"+
		"Path"] = picPath
	options["fileNameStyle"] = fileNameStyle
	options["disableBar"] = true // 禁用下载进度条

	downloader.CheckPathExists(picPath) // 检查缓存路径
	downloader.CheckPathExists(musicPath)

	bot, err := tgbotapi.NewBotAPI(config["BOT_TOKEN"])
	if config["BotAPI"] != "" {
		bot.SetAPIEndpoint(config["BotAPI"] + "/bot%s/%s")
	}
	if err != nil {
		log.Panic(err)
	}

	if config["BotApiDebug"] == "true" {
		bot.Debug = true
	}

	log.Printf("%s 验证成功", bot.Self.UserName)
	botName = bot.Self.UserName // 自动获取 botName

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)
	if err != nil {
		log.Error(err)
	}

	for update := range updates {
		if update.Message != nil { // 忽略空 update
			switch {
			case update.Message.Command() != "":
				switch update.Message.Command() {
				case "musicid", "netease", "start":
					musicid := update.Message.CommandArguments()

					_, err := strconv.ParseFloat(musicid, 64)
					if err != nil {
						continue
					}

					message := *update.Message
					err = processMusic(musicid, message, *bot)
					if err != nil {
						log.Errorln(err)
					}
				case "search":
					searchArg := update.Message.CommandArguments()
					var replacer = strings.NewReplacer("\n", "")
					searchArg = replacer.Replace(searchArg)

					if searchArg != "" {
						message := *update.Message
						err = processSearch(searchArg, message, *bot)
						if err != nil {
							log.Errorln(err)
						}
					}
				case "about":
					func() {
						defer func() {
							err := recover()
							if err != nil {
								log.Errorln(err)
							}
						}()
						message := *update.Message
						newMsg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf(
							"*Music163bot-Go %s*\n"+
								"Github: https://github.com/XiaoMengXinX/Music163bot-Go\n\n"+
								"\\[编译环境] %s\n"+
								"\\[程序版本] %s\n"+
								"\\[编译哈希] %s\n"+
								"\\[编译日期] %s\n"+
								"\\[编译系统] %s\n"+
								"\\[运行环境] %s",
							VERSION, RUNTIME_VERSION, VERSION, COMMIT_SHA, BUILD_TIME, BUILD_OS, BUILD_ARCH),
						)
						newMsg.ParseMode = tgbotapi.ModeMarkdown
						newMsg.ReplyToMessageID = message.MessageID
						message, err = bot.Send(newMsg)
						if err != nil {
							log.Errorln(err)
						}
					}()
				case "rmcache":
					if config["BotDebug"] == "true" {
						func() {
							defer func() {
								err := recover()
								if err != nil {
									log.Errorln(err)
								}
							}()
							db := DB.Session(&gorm.Session{AllowGlobalUpdate: true})
							if update.Message.CommandArguments() == "all" {
								err := db.Delete(&SongInfo{}).Error
								if err != nil {
									log.Errorln(err)
									return
								}
								message := *update.Message
								newMsg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("[DEBUG] 清除全部数据库成功"))
								newMsg.ReplyToMessageID = message.MessageID
								message, err = bot.Send(newMsg)
								if err != nil {
									log.Errorln(err)
								}
							} else {
								var SongInfo SongInfo
								musicid := reg4.ReplaceAllString(reg3.ReplaceAllString(reg2.ReplaceAllString(reg1.ReplaceAllString(update.Message.CommandArguments(), ""), ""), ""), "")
								db.Where("music_id = ?", musicid).Delete(&SongInfo)
								message := *update.Message
								newMsg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("[DEBUG] 清除 musicid : %s 缓存成功", musicid))
								newMsg.ReplyToMessageID = message.MessageID
								message, err = bot.Send(newMsg)
								if err != nil {
									log.Errorln(err)
								}
							}
						}()
					}
				}
			case strings.Contains(update.Message.Text, "music.163.com"):
				var replacer = strings.NewReplacer("\n", "", " ", "")
				messageText := replacer.Replace(update.Message.Text) // 去除消息内空格和换行 避免不必要的麻烦（

				musicid := reg4.ReplaceAllString(reg3.ReplaceAllString(reg2.ReplaceAllString(reg1.ReplaceAllString(messageText, ""), ""), ""), "")
				// 自动嗅探分享链接

				_, err := strconv.ParseFloat(musicid, 64) // 检测处理后的 musicid 是否为数字 （防止嗅探错误）
				if err != nil {
					continue
				}

				message := *update.Message
				go func() {
					err := processMusic(musicid, message, *bot)
					if err != nil {
						log.Errorln(err)
					}
				}()
			}
		} else if update.CallbackQuery != nil {
			musicid := reg4.ReplaceAllString(reg3.ReplaceAllString(reg2.ReplaceAllString(reg1.ReplaceAllString(update.CallbackQuery.Data, ""), ""), ""), "")

			_, err := strconv.ParseFloat(update.CallbackQuery.Data, 64)
			if err != nil {
				continue
			}

			if update.CallbackQuery.Message.Chat.IsPrivate() {
				callback := tgbotapi.NewCallback(update.CallbackQuery.ID, "Success")
				_, err := bot.Request(callback)
				if err != nil {
					log.Errorln(err)
				}
				message := *update.CallbackQuery.Message
				err = processMusic(musicid, message, *bot)
				if err != nil {
					log.Errorln(err)
				}
			} else {
				callback := tgbotapi.NewCallback(update.CallbackQuery.ID, "Success")
				callback.URL = fmt.Sprintf("t.me/%s?start=%s", botName, musicid)
				_, err := bot.Request(callback)
				if err != nil {
					log.Errorln(err)
				}
			}

		} else if update.InlineQuery != nil {
			musicid := reg4.ReplaceAllString(reg3.ReplaceAllString(reg2.ReplaceAllString(reg1.ReplaceAllString(update.InlineQuery.Query, ""), ""), ""), "")

			_, err := strconv.ParseFloat(musicid, 64)
			if err != nil {
				continue
			}

			query := *update.InlineQuery
			err = processInlineMusic(musicid, query, *bot)
			if err != nil {
				log.Errorln(err)
			}
		}
	}
}

func processSearch(searchArg string, message tgbotapi.Message, bot tgbotapi.BotAPI) (err error) {
	defer func() {
		processTreads.Done()
		err := recover()
		if err != nil {
			log.Errorln(err)
			func() {
				defer func() {
					err := recover()
					if err != nil {
						log.Errorln(err)
					}
				}()
				newEditMsg := tgbotapi.NewEditMessageText(message.Chat.ID, message.MessageID, fmt.Sprintf("搜索失败"))
				message, err = bot.Send(newEditMsg)
				if err != nil {
					log.Errorln(err)
				}
			}()
		}
	}()
	processTreads.Add(1)

	newMsg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("搜索中..."))
	newMsg.ReplyToMessageID = message.MessageID
	message, err = bot.Send(newMsg)
	if err != nil {
		return err
	}

	searchResult := utils.SearchSong(searchArg, options)
	var inlineButton []tgbotapi.InlineKeyboardButton
	textMessage := ""

	if _, ok := searchResult["body"].(map[string]interface{})["result"].(map[string]interface{})["songs"].([]interface{}); ok {
		for i := 0; i < len(searchResult["body"].(map[string]interface{})["result"].(map[string]interface{})["songs"].([]interface{})); i++ {
			songDetail := make(map[string]interface{})
			songDetail["body"] = make(map[string]interface{})
			songDetail["body"].(map[string]interface{})["songs"] = searchResult["body"].(map[string]interface{})["result"].(map[string]interface{})["songs"].([]interface{})
			songID := fmt.Sprintf("%v", int64(searchResult["body"].(map[string]interface{})["result"].(map[string]interface{})["songs"].([]interface{})[i].(map[string]interface{})["id"].(float64)))
			songName := downloader.ParseName("", i, songDetail)
			songArtists, _ := downloader.ParseArtist(i, songDetail)
			inlineButton = append(inlineButton, tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("%v", i+1), songID))
			textMessage = fmt.Sprintf("%s%d.「%s」 - %s\n", textMessage, i+1, songName, songArtists)
		}
	} else {
		newEditMsg := tgbotapi.NewEditMessageText(message.Chat.ID, message.MessageID, fmt.Sprintf("未找到结果"))
		message, err = bot.Send(newEditMsg)
		if err != nil {
			log.Errorln(err)
		}
		return nil
	}

	var numericKeyboard = tgbotapi.NewInlineKeyboardMarkup(inlineButton)

	newEditMsg := tgbotapi.NewEditMessageText(message.Chat.ID, message.MessageID, textMessage)
	newEditMsg.ReplyMarkup = &numericKeyboard
	message, err = bot.Send(newEditMsg)
	if err != nil {
		return err
	}

	return nil
}

func processInlineMusic(musicid string, message tgbotapi.InlineQuery, bot tgbotapi.BotAPI) (err error) {
	defer func() {
		err := recover()
		if err != nil {
			log.Errorln(err)
		}
	}()
	var songInfo SongInfo
	db := DB.Session(&gorm.Session{})
	err = db.Where("music_id = ?", musicid).First(&songInfo).Error // 查找是否有缓存数据
	if err == nil {                                                // 从缓存数据回应 inlineQuery
		if songInfo.FileID != "" && songInfo.SongName != "" {
			numericKeyboard := tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonURL(fmt.Sprintf("%s- %s", songInfo.SongName, songInfo.SongArtists), fmt.Sprintf("https://music.163.com/#/song/%s/", songInfo.MusicID)),
					tgbotapi.NewInlineKeyboardButtonSwitch("Send me to...", fmt.Sprintf("https://music.163.com/#/song/%s/", songInfo.MusicID)),
				),
			)

			newAudio := tgbotapi.NewInlineQueryResultCachedDocument(message.ID, songInfo.FileID, fmt.Sprintf("%s - %s", songInfo.SongArtists, songInfo.SongName))
			newAudio.Caption = fmt.Sprintf("「%s」- %s\n专辑： %s\n#网易云音乐 #%s %.2fkpbs\nvia @%s", songInfo.SongName, songInfo.SongArtists, songInfo.SongAlbum, songInfo.FileExt, float64(songInfo.BitRate)/1000, botName)
			newAudio.ReplyMarkup = &numericKeyboard
			newAudio.Description = songInfo.SongAlbum

			inlineConf := tgbotapi.InlineConfig{
				InlineQueryID: message.ID,
				Results:       []interface{}{newAudio},
				IsPersonal:    false,
				CacheTime:     3600,
			}

			_, err := bot.Request(inlineConf)
			if err != nil {
				return err
			}
		}
	} else {
		if !errors.Is(err, logger.ErrRecordNotFound) {
			log.Errorln(err)
		}

		inlineMsg := tgbotapi.NewInlineQueryResultArticle(message.ID, "歌曲未缓存", message.Query)
		inlineMsg.Description = "点击上方按钮缓存歌曲"

		inlineConf := tgbotapi.InlineConfig{
			InlineQueryID:     message.ID,
			IsPersonal:        false,
			Results:           []interface{}{inlineMsg},
			CacheTime:         3600,
			SwitchPMText:      "点我缓存歌曲",
			SwitchPMParameter: fmt.Sprintf("%s", musicid),
		}

		_, err := bot.Request(inlineConf)
		if err != nil {
			return err
		}
	}
	return nil
}

func processMusic(musicid string, message tgbotapi.Message, bot tgbotapi.BotAPI) (err error) {
	defer func() {
		processTreads.Done()
		err := recover()
		if err != nil {
			log.Errorln(err)
			func() {
				defer func() {
					err := recover()
					if err != nil {
						log.Errorln(err)
					}
				}()
				db := DB.Session(&gorm.Session{AllowGlobalUpdate: true})
				db.Where("music_id = ?", musicid).Delete(&DownloadingMusic{})
				newEditMsg := tgbotapi.NewEditMessageText(message.Chat.ID, message.MessageID, fmt.Sprintf("未知错误 0"))
				message, err = bot.Send(newEditMsg) // 方便错误定位
				if err != nil {
					log.Errorln(err)
				}
			}()
		}
	}()
	processTreads.Add(1) // 添加 协程任务

	var songInfo SongInfo
	db := DB.Session(&gorm.Session{}) // 创建 sql 会话
	var times []RequestTimes          // 读取当前 Chat 的每分钟请求次数，若大于 minuteLimitation 则无视
	dbResult := db.Find(&times, fmt.Sprintf("%d%d", time.Now().Hour(), time.Now().Minute()), message.Chat.ID)
	if dbResult.RowsAffected >= minuteLimitation {
		return nil
	}
	dbWrite := db.Create(&RequestTimes{ // 写入请求记录
		fmt.Sprintf("%d%d", time.Now().Hour(), time.Now().Minute()),
		message.Chat.ID,
	})
	if dbWrite.Error != nil {
		log.Errorln(err)
	}

	err = db.Where("music_id = ?", musicid).First(&DownloadingMusic{}).Error // 查找是否已经存在下载任务
	if err == nil {
		Msg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("存在下载任务，请稍候..."))
		Msg.ReplyToMessageID = message.MessageID
		msg, err := bot.Send(Msg)
		var IsDownloadFinished bool
		for !IsDownloadFinished {
			err := db.Where("music_id = ?", musicid).First(&DownloadingMusic{}).Error
			if err != nil {
				IsDownloadFinished = true
			}
			time.Sleep(time.Duration(100) * time.Millisecond)
		}
		time.Sleep(time.Duration(500) * time.Millisecond)
		deleteMsg := tgbotapi.NewDeleteMessage(msg.Chat.ID, msg.MessageID)
		_, err = bot.Request(deleteMsg)
		if err != nil {
			log.Errorln(err)
		}
	}

	err = db.Where("music_id = ?", musicid).First(&songInfo).Error // 查找是否有缓存数据
	if err == nil {                                                // 从缓存数据发送
		if songInfo.FileID != "" && songInfo.SongName != "" {
			newEditMsg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("「%s」- %s\n专辑： %s\n%s %sMB\n命中缓存，正在发送中...", songInfo.SongName, songInfo.SongArtists, songInfo.SongAlbum, songInfo.FileExt, songInfo.FileSize))
			newEditMsg.ReplyToMessageID = message.MessageID
			message, err = bot.Send(newEditMsg)
			if err != nil {
				log.Errorln(err)
			}

			numericKeyboard := tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonURL(fmt.Sprintf("%s- %s", songInfo.SongName, songInfo.SongArtists), fmt.Sprintf("https://music.163.com/#/song/%s/", songInfo.MusicID)),
				),
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonSwitch("Send me to...", fmt.Sprintf("https://music.163.com/#/song/%s/", songInfo.MusicID)),
				),
			)

			audioFile := tgbotapi.FileID(songInfo.FileID)
			newAudio := tgbotapi.NewAudio(message.Chat.ID, audioFile)
			newAudio.Caption = fmt.Sprintf("「%s」- %s\n专辑： %s\n#网易云音乐 #%s %.2fkpbs\nvia @%s", songInfo.SongName, songInfo.SongArtists, songInfo.SongAlbum, songInfo.FileExt, float64(songInfo.BitRate)/1000, botName)
			newAudio.Title = fmt.Sprintf("%s", songInfo.SongName)
			newAudio.Performer = songInfo.SongArtists
			newAudio.Duration = songInfo.Duration / 1000
			newAudio.ReplyMarkup = numericKeyboard
			if err == nil {
				thumbFile := tgbotapi.FileID(songInfo.ThumbFileID)
				newAudio.Thumb = thumbFile
			}
			_, err := bot.Send(newAudio)

			if err != nil {
				log.Errorln(err)
				newEditMsg := tgbotapi.NewEditMessageText(message.Chat.ID, message.MessageID, fmt.Sprintf("%s\n专辑： %s\n%s %sMB\n发送失败 %s", songInfo.SongName, songInfo.SongAlbum, songInfo.FileExt, songInfo.FileSize, err))
				message, err = bot.Send(newEditMsg)
				if err != nil {
					log.Errorln(err)
				}
				return nil
			}

			time.Sleep(time.Duration(1) * time.Second)

			deleteMsg := tgbotapi.NewDeleteMessage(message.Chat.ID, message.MessageID)
			_, err = bot.Request(deleteMsg)
			if err != nil {
				log.Errorln(err)
			}

			return nil
		}
	} else {
		if !errors.Is(err, logger.ErrRecordNotFound) {
			log.Errorln(err)
		}
	}

	var ids []string
	ids = append(ids, musicid)

	songInfo.FromChatID = message.Chat.ID
	if message.Chat.IsPrivate() {
		songInfo.FromChatName = message.Chat.UserName
	} else {
		songInfo.FromChatName = message.Chat.Title
	}
	songInfo.FromUserID = message.From.ID
	songInfo.FromUserName = message.From.UserName

	dbResult = db.Create(&DownloadingMusic{MusicID: musicid})
	if dbResult.Error != nil {
		log.Errorln(err)
	}

	newMsg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("正在获取歌曲信息..."))
	newMsg.ReplyToMessageID = message.MessageID
	message, err = bot.Send(newMsg)
	if err != nil {
		return err
	}

	SongDetail := utils.GetSongDetail(musicid, options)
	if len(SongDetail["body"].(map[string]interface{})["songs"].([]interface{})) != 1 {
		fmt.Println(len(SongDetail["body"].(map[string]interface{})["songs"].([]interface{})))
		newEditMsg := tgbotapi.NewEditMessageText(message.Chat.ID, message.MessageID, fmt.Sprintf("获取歌曲信息失败"))
		message, err = bot.Send(newEditMsg)
		return nil
	}

	SongUrl := utils.GetSongUrl(musicid, options)

	resultCache := make(map[string]interface{})
	resultCache["SongDetail"] = SongDetail
	resultCache["SongUrl"] = SongUrl

	songInfo.MusicID = musicid
	songInfo.SongName = downloader.ParseName(musicid, 0, SongDetail) // 解析歌曲信息
	songInfo.SongArtists, _ = downloader.ParseArtist(0, SongDetail)
	songInfo.SongAlbum, _, _, _ = downloader.ParseAlbum(musicid, 0, SongDetail)

	if SongUrl["body"].(map[string]interface{})["data"].([]interface{})[0].(map[string]interface{})["url"] != nil {
		url := SongUrl["body"].(map[string]interface{})["data"].([]interface{})[0].(map[string]interface{})["url"].(string)
		switch path.Ext(path.Base(url)) {
		case ".mp3":
			songInfo.FileExt = "mp3"
		case ".flac":
			songInfo.FileExt = "flac"
		case ".m4a":
			songInfo.FileExt = "m4a"
		default:
			songInfo.FileExt = "mp3"
		}
		songInfo.FileSize, err = getFileSize(url)
		if err != nil {
			return err
		}
	} else {
		newEditMsg := tgbotapi.NewEditMessageText(message.Chat.ID, message.MessageID, fmt.Sprintf("%s\n专辑： %s\n获取下载链接失败", songInfo.SongName, songInfo.SongAlbum))
		message, err = bot.Send(newEditMsg)
		if err != nil {
			log.Errorln(err)
		}
		return fmt.Errorf("获取 musicid : %s 下载链接失败", musicid)
	}

	newEditMsg := tgbotapi.NewEditMessageText(message.Chat.ID, message.MessageID, fmt.Sprintf("%s\n专辑： %s\n%s %sMB\n等待下载中...", songInfo.SongName, songInfo.SongAlbum, songInfo.FileExt, songInfo.FileSize))
	message, err = bot.Send(newEditMsg)
	if err != nil {
		log.Errorln(err)
	}

	go func() {
		err := handleMusic(ids, songInfo, message, bot, resultCache, options)
		if err != nil {
			log.Errorln(err)
			func() {
				defer func() {
					err := recover()
					if err != nil {
						log.Errorln(err)
					}
				}()
				newEditMsg := tgbotapi.NewEditMessageText(message.Chat.ID, message.MessageID, fmt.Sprintf("未知错误 1"))
				message, err = bot.Send(newEditMsg)
				if err != nil {
					log.Errorln(err)
				}
			}()
		}
	}()

	return
}

func handleMusic(ids []string, songInfo SongInfo, message tgbotapi.Message, bot tgbotapi.BotAPI, resultCache, options map[string]interface{}) (err error) {
	defer func() {
		downloadTreads.Done()
		db := DB.Session(&gorm.Session{AllowGlobalUpdate: true})
		db.Where("music_id = ?", songInfo.MusicID).Delete(&DownloadingMusic{})
		err := recover()
		if err != nil {
			log.Errorln(err)
			func() {
				defer func() {
					err := recover()
					if err != nil {
						log.Errorln(err)
					}
				}()
				newEditMsg := tgbotapi.NewEditMessageText(message.Chat.ID, message.MessageID, fmt.Sprintf("未知错误 2"))
				message, err = bot.Send(newEditMsg)
				if err != nil {
					log.Errorln(err)
				}
			}()
		}
	}()
	downloadTreads.Add(1)

	newMsg := tgbotapi.NewEditMessageText(message.Chat.ID, message.MessageID, fmt.Sprintf("%s\n专辑： %s\n%s %sMB\n下载中...", songInfo.SongName, songInfo.SongAlbum, songInfo.FileExt, songInfo.FileSize))
	message, err = bot.Send(newMsg)
	if err != nil {
		log.Errorln(err)
	}

	_, err = downloader.DownloadSongWithMetadata(ids, resultCache, options)
	if err != nil {
		return err
	}

	var replacer = strings.NewReplacer("/", " ", "?", " ", "*", " ", ":", " ", "|", " ", "\\", " ", "<", " ", ">", " ", "\"", " ")
	fileName := replacer.Replace(fmt.Sprintf("%v - %v.%v", strings.Replace(songInfo.SongArtists, "/", ",", -1), songInfo.SongName, songInfo.FileExt))
	if !utils.FileExists(musicPath + "/" + fileName) {
		return fmt.Errorf("Invaid file %s ", fileName)
	}

	newMsg = tgbotapi.NewEditMessageText(message.Chat.ID, message.MessageID, fmt.Sprintf("%s\n专辑： %s\n%s %sMB\n下载完成，发送中...", songInfo.SongName, songInfo.SongAlbum, songInfo.FileExt, songInfo.FileSize))
	message, err = bot.Send(newMsg)
	if err != nil {
		log.Errorln(err)
	}

	var bitRate, duration int
	switch songInfo.FileExt {
	case "mp3":
		bitRate, duration = downloader.GetMp3Info(fileName, options)
	case "flac":
		bitRate, duration = downloader.GetFlacInfo(fileName, options)
	}
	songInfo.BitRate, songInfo.Duration = bitRate, duration

	picPath := picPath + "/" + ids[0] + ".jpg"
	newPicPath, err := ResizeImg(picPath)

	numericKeyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL(fmt.Sprintf("%s- %s", songInfo.SongName, songInfo.SongArtists), fmt.Sprintf("https://music.163.com/#/song/%s/", songInfo.MusicID)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonSwitch("Send me to...", fmt.Sprintf("https://music.163.com/#/song/%s/", songInfo.MusicID)),
		),
	)

	newAudio := tgbotapi.NewAudio(message.Chat.ID, musicPath+"/"+fileName)
	newAudio.Caption = fmt.Sprintf("「%s」- %s\n专辑： %s\n#网易云音乐 #%s %.2fkpbs\nvia @%s", songInfo.SongName, songInfo.SongArtists, songInfo.SongAlbum, songInfo.FileExt, float64(bitRate)/1000, botName)
	newAudio.Title = fmt.Sprintf("%s", songInfo.SongName)
	newAudio.Performer = songInfo.SongArtists
	newAudio.Duration = duration / 1000
	newAudio.ReplyMarkup = numericKeyboard
	if err == nil {
		newAudio.Thumb = newPicPath
	}
	audio, err := bot.Send(newAudio)

	songInfo.FileID = audio.Audio.FileID
	songInfo.ThumbFileID = audio.Audio.Thumbnail.FileID

	if err != nil {
		log.Errorln(err)
		newMsg = tgbotapi.NewEditMessageText(message.Chat.ID, message.MessageID, fmt.Sprintf("%s\n专辑： %s\n%s %sMB\n发送失败 %s", songInfo.SongName, songInfo.SongAlbum, songInfo.FileExt, songInfo.FileSize, err))
		message, err = bot.Send(newMsg)
		if err != nil {
			log.Errorln(err)
		}
		return nil
	}

	db := DB.Session(&gorm.Session{})
	dbResult := db.Create(&songInfo) // 写入歌曲缓存
	if dbResult.Error != nil {
		log.Errorln(err)
	}

	deleteMsg := tgbotapi.NewDeleteMessage(message.Chat.ID, message.MessageID)
	_, err = bot.Request(deleteMsg)
	if err != nil {
		log.Errorln(err)
	}

	return nil
}
