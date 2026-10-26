package bot

import (
	"fmt"

	"github.com/go-telegram/bot/models"

	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/render"
)

func musicKeyboard(songName, songArtists string, musicID int) models.InlineKeyboardMarkup {
	songURL := fmt.Sprintf("https://music.163.com/song?id=%d", musicID)
	return models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{{
				Text: fmt.Sprintf("%s- %s", songName, songArtists),
				URL:  songURL,
			}},
			{{
				Text:              render.SendMeTo,
				SwitchInlineQuery: &songURL,
			}},
		},
	}
}

func songLinkKeyboard(musicID int) models.InlineKeyboardMarkup {
	return models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{
		Text: "在网易云音乐中打开",
		URL:  render.SongLink(musicID),
	}}}}
}

func (a *App) guestKeyboard(musicID int) models.InlineKeyboardMarkup {
	keyboard := songLinkKeyboard(musicID)
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
