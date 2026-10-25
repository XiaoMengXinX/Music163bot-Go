package bot

import (
	"fmt"

	"github.com/go-telegram/bot/models"

	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/render"
)

// musicKeyboard 音频消息下方的按钮：歌曲链接 + 「Send me to...」
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

// searchKeyboard 搜索结果的单行数字按钮，回调数据为 "music <id>"
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
