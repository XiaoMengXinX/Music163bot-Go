package bot

import (
	"fmt"
	"github.com/XiaoMengXinX/Music163bot-Go/v2/util"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"gorm.io/gorm"
)

func processInlineMusic(musicid int, message tgbotapi.InlineQuery, bot *tgbotapi.BotAPI) (err error) {
	var songInfo util.SongInfo
	db := DB.Session(&gorm.Session{})
	err = db.Where("music_id = ?", musicid).First(&songInfo).Error // 查找是否有缓存数据
	if err == nil {                                                // 从缓存数据回应 inlineQuery
		if songInfo.FileID != "" && songInfo.SongName != "" {
			numericKeyboard := tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonURL(fmt.Sprintf("%s- %s", songInfo.SongName, songInfo.SongArtists), fmt.Sprintf("https://music.163.com/song?id=%d", songInfo.MusicID)),
				),
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonSwitch("Send me to...", fmt.Sprintf("https://music.163.com/song?id=%d", songInfo.MusicID)),
				),
			)

			newAudio := tgbotapi.NewInlineQueryResultCachedDocument(message.ID, songInfo.FileID, fmt.Sprintf("%s - %s", songInfo.SongArtists, songInfo.SongName))
			newAudio.Caption = fmt.Sprintf(musicInfo, songInfo.SongName, songInfo.SongArtists, songInfo.SongAlbum, songInfo.FileExt, float64(songInfo.BitRate)/1000, botName)
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
		inlineMsg := tgbotapi.NewInlineQueryResultArticle(message.ID, noCache, message.Query)
		inlineMsg.Description = tapToDownload

		inlineConf := tgbotapi.InlineConfig{
			InlineQueryID:     message.ID,
			IsPersonal:        false,
			Results:           []interface{}{inlineMsg},
			CacheTime:         3600,
			SwitchPMText:      tapMeToDown,
			SwitchPMParameter: fmt.Sprintf("%d", musicid),
		}

		_, err := bot.Request(inlineConf)
		if err != nil {
			return err
		}
	}
	return err
}
