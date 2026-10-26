package bot

import (
	"bytes"
	"context"
	"errors"
	"math"
	"time"

	tgb "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/skip2/go-qrcode"

	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/netease"
	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/render"
)

var errLoginCancelled = errors.New("login cancelled")

func (a *App) handleLogin(ctx context.Context, tg *tgb.Bot, update *models.Update) {
	m := update.Message
	if !a.loginMu.TryLock() {
		if _, err := a.reply(ctx, m, render.LoginBusy); err != nil {
			a.log.Errorln(err)
		}
		return
	}
	defer a.loginMu.Unlock()

	key, err := a.api.CreateQRLoginKey()
	if err != nil {
		a.log.Errorln(err)
		if _, err := a.reply(ctx, m, render.LoginKeyFailed); err != nil {
			a.log.Errorln(err)
		}
		return
	}
	png, err := qrcode.Encode(netease.QRLoginContent(key), qrcode.High, 512)
	if err != nil {
		a.log.Errorln(err)
		if _, err := a.reply(ctx, m, render.LoginKeyFailed); err != nil {
			a.log.Errorln(err)
		}
		return
	}
	photo, err := a.tg.SendPhoto(ctx, &tgb.SendPhotoParams{
		ChatID:          m.Chat.ID,
		Photo:           &models.InputFileUpload{Filename: "login-qr.png", Data: bytes.NewReader(png)},
		Caption:         render.LoginQRCaption(int(loginQRValidity / time.Second)),
		ParseMode:       models.ParseModeHTML,
		ReplyParameters: &models.ReplyParameters{MessageID: m.ID},
		ReplyMarkup:     cancelLoginKeyboard(),
	})
	if err != nil {
		a.log.Errorln(err)
		return
	}

	sessCtx, cancelSession := context.WithCancelCause(ctx)
	defer cancelSession(nil)
	a.setLoginCancel(cancelSession)
	defer a.setLoginCancel(nil)

	editCaption := func(text string, markup models.ReplyMarkup) error {
		_, err := a.tg.EditMessageCaption(ctx, &tgb.EditMessageCaptionParams{
			ChatID:      m.Chat.ID,
			MessageID:   photo.ID,
			Caption:     text,
			ParseMode:   models.ParseModeHTML,
			ReplyMarkup: markup,
		})
		return err
	}

	a.pollQRLogin(sessCtx, key, editCaption)
}

func cancelLoginKeyboard() models.InlineKeyboardMarkup {
	return models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{{
			{Text: render.CancelLogin, CallbackData: "cancellogin"},
		}},
	}
}

// emptyKeyboard 编辑消息时用于移除 inline 按钮
func emptyKeyboard() models.InlineKeyboardMarkup {
	return models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{}}
}

// loginMu 由会话持有，取消函数须用独立锁保护
func (a *App) setLoginCancel(cancel context.CancelCauseFunc) {
	a.loginCancelMu.Lock()
	defer a.loginCancelMu.Unlock()
	a.loginCancel = cancel
}

func (a *App) cancelLoginSession() bool {
	a.loginCancelMu.Lock()
	defer a.loginCancelMu.Unlock()
	if a.loginCancel == nil {
		return false
	}
	a.loginCancel(errLoginCancelled)
	return true
}

func (a *App) handleCallbackCancelLogin(ctx context.Context, tg *tgb.Bot, update *models.Update) {
	query := update.CallbackQuery
	if !a.cancelLoginSession() {
		text := render.LoginNoSession
		if msg := query.Message.Message; msg != nil {
			if _, err := a.tg.EditMessageCaption(ctx, &tgb.EditMessageCaptionParams{
				ChatID:      msg.Chat.ID,
				MessageID:   msg.ID,
				Caption:     render.LoginSessionOver,
				ParseMode:   models.ParseModeHTML,
				ReplyMarkup: emptyKeyboard(),
			}); err != nil {
				a.log.Errorln(err)
			}
		}
		if _, err := a.tg.AnswerCallbackQuery(ctx, &tgb.AnswerCallbackQueryParams{
			CallbackQueryID: query.ID,
			Text:            text,
		}); err != nil {
			a.log.Errorln(err)
		}
		return
	}
	if _, err := a.tg.AnswerCallbackQuery(ctx, &tgb.AnswerCallbackQueryParams{
		CallbackQueryID: query.ID,
		Text:            render.LoginCancelled,
	}); err != nil {
		a.log.Errorln(err)
	}
}

// loginQRValidity 仅用于倒计时，以服务端过期状态为准
const loginQRValidity = 3 * time.Minute

func (a *App) pollQRLogin(ctx context.Context, key string, editCaption func(string, models.ReplyMarkup) error) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	countdown := time.NewTicker(10 * time.Second)
	defer countdown.Stop()
	expiresAt := time.Now().Add(loginQRValidity)
	deadline := time.After(5 * time.Minute) // 二维码约 3 分钟过期，此处兜底
	// 803 可能不带昵称，保留 802 的昵称
	var scannedNick string
	var failures int
	for {
		select {
		case <-ctx.Done():
			if errors.Is(context.Cause(ctx), errLoginCancelled) {
				_ = editCaption(render.LoginCancelled, emptyKeyboard())
			}
			return
		case <-deadline:
			_ = editCaption(render.LoginExpired, emptyKeyboard())
			return
		case <-countdown.C:
			remain := int(math.Ceil(time.Until(expiresAt).Seconds()))
			if remain < 0 {
				remain = 0
			}
			if scannedNick == "" {
				_ = editCaption(render.LoginQRCaption(remain), cancelLoginKeyboard())
			}
		case <-ticker.C:
			status, nickname, musicU, err := a.api.CheckQRLogin(key)
			if err != nil {
				failures++
				if failures >= 10 {
					a.log.Errorln(err)
					_ = editCaption(render.LoginCheckFailed, emptyKeyboard())
					return
				}
				continue
			}
			failures = 0
			switch status {
			case netease.QRWaiting:
			case netease.QRScanned:
				if scannedNick == "" {
					scannedNick = nickname
					if err := editCaption(render.LoginScanned(nickname), cancelLoginKeyboard()); err != nil {
						a.log.Errorln(err)
					}
				}
			case netease.QRSuccess:
				if nickname == "" {
					nickname = scannedNick
				}
				note := a.finishLogin(musicU, nickname)
				_ = editCaption(render.LoginOK(nickname)+note, emptyKeyboard())
				return
			case netease.QRExpired:
				_ = editCaption(render.LoginExpired, emptyKeyboard())
				return
			}
		}
	}
}
