package bot

import (
	"fmt"

	"github.com/XiaoMengXinX/Music163Api-Go/types"
	"github.com/go-telegram/bot/models"

	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/render"
	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/store"
)

// prog 非 nil 时打开节目页；inline 只能按歌曲链接查缓存，分享按钮始终使用歌曲链接
func musicKeyboard(songInfo store.SongInfo, prog *types.ProgramDetailData) models.InlineKeyboardMarkup {
	songURL := fmt.Sprintf("https://music.163.com/song?id=%d", songInfo.MusicID)
	link := songURL
	if prog != nil {
		link = render.ProgramLink(int(prog.Program.ID))
	}
	return models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{{
				Text: fmt.Sprintf("%s- %s", songInfo.SongName, songInfo.SongArtists),
				URL:  link,
			}},
			{{
				Text:              render.SendMeTo,
				SwitchInlineQuery: &songURL,
			}},
		},
	}
}

func linkKeyboard(link string) models.InlineKeyboardMarkup {
	return models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{
		Text: "在网易云音乐中打开",
		URL:  link,
	}}}}
}

func (a *App) guestKeyboard(link string, musicID int) models.InlineKeyboardMarkup {
	keyboard := linkKeyboard(link)
	keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, []models.InlineKeyboardButton{{
		Text: "在 Bot 中获取歌曲",
		URL:  fmt.Sprintf("https://t.me/%s?start=%d", a.me.Username, musicID),
	}})
	return keyboard
}

func searchKeyboard(musicIDs []int) models.InlineKeyboardMarkup {
	buttons := make([]models.InlineKeyboardButton, 0, len(musicIDs))
	for i, id := range musicIDs {
		buttons = append(buttons, models.InlineKeyboardButton{
			Text:         fmt.Sprintf("%d", i+1),
			CallbackData: fmt.Sprintf("music %d", id),
		})
	}
	return models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{buttons}}
}
