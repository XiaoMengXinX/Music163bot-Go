package bot

import (
	"testing"

	"github.com/go-telegram/bot/models"
)

func TestMessageMusic(t *testing.T) {
	a := newTestApp(nil)
	audioKeyboard := musicKeyboard("Song", "Artist", 123)
	guestKeyboard := songLinkKeyboard(456)
	for _, tt := range []struct {
		name    string
		message *models.Message
		want    int
	}{
		{name: "text", message: &models.Message{Text: "分享 https://music.163.com/song?id=123&userid=1"}, want: 123},
		{name: "caption", message: &models.Message{Caption: "https://music.163.com/#/song?id=123"}, want: 123},
		{name: "caption text link", message: &models.Message{
			Caption:         "点我",
			CaptionEntities: []models.MessageEntity{{Type: models.MessageEntityTypeTextLink, URL: "https://music.163.com/song?id=123"}},
		}, want: 123},
		{name: "audio button", message: &models.Message{Caption: "「Song」- Artist\nvia @testbot", ReplyMarkup: &audioKeyboard}, want: 123},
		{name: "song link button", message: &models.Message{Text: "已识别到歌曲", ReplyMarkup: &guestKeyboard}, want: 456},
		{name: "text before button", message: &models.Message{Text: "https://music.163.com/song?id=789", ReplyMarkup: &audioKeyboard}, want: 789},
		{name: "no link", message: &models.Message{Text: "雨き声残響 2", Entities: []models.MessageEntity{{Type: models.MessageEntityTypeURL}}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, prog, err := a.messageMusic(tt.message)
			if err != nil || prog != nil || got != tt.want {
				t.Errorf("messageMusic = %d, %v, %v; want %d", got, prog, err, tt.want)
			}
		})
	}
}
