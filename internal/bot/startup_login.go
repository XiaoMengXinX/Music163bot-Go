package bot

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"time"

	tgb "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/skip2/go-qrcode"

	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/netease"
	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/render"
)

const (
	startupLoginBudget = 3 * time.Minute // 以服务端 800 过期信号为准，此处兜底
	tgCountdownTick    = 10 * time.Second
)

// startupSession 由终端和管理员共享同一二维码
type startupSession struct {
	png       []byte
	expiresAt time.Time
	prompts   map[int64]int // 管理员 chatID -> 提示消息 ID
	surfaces  map[int64]int // 管理员 chatID -> 二维码消息 ID
}

func (a *App) startupLogin(runCtx context.Context, cancelRun context.CancelCauseFunc, invalid bool) {
	if !a.loginMu.TryLock() {
		cancelRun(errLoginStopped)
		return
	}
	defer a.loginMu.Unlock()

	key, png, qr, err := a.newStartupQR()
	if err != nil {
		a.log.Errorln(err)
		cancelRun(errLoginStopped)
		return
	}
	sess := &startupSession{
		png:       png,
		expiresAt: time.Now().Add(startupLoginBudget),
		prompts:   map[int64]int{},
		surfaces:  map[int64]int{},
	}
	a.startupMu.Lock()
	a.startupQR = sess
	a.startupMu.Unlock()
	defer func() {
		a.startupMu.Lock()
		a.startupQR = nil
		a.startupMu.Unlock()
	}()

	remain := remainSeconds(sess)
	fmt.Printf("\n%s\n", render.LoginStartupTerminal(invalid, remain))
	fmt.Printf("若终端显示异常，可在手机浏览器打开: %s\n\n", netease.QRLoginContent(key))
	fmt.Print(qr.ToSmallString(false))
	a.sendStartupPrompts(runCtx, sess, invalid)

	sessCtx, cancelSession := context.WithCancelCause(runCtx)
	defer cancelSession(nil)
	a.setLoginCancel(cancelSession)
	defer a.setLoginCancel(nil)

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	countdown := time.NewTicker(30 * time.Second)
	defer countdown.Stop()
	tgCountdown := time.NewTicker(tgCountdownTick)
	defer tgCountdown.Stop()
	deadline := time.After(startupLoginBudget)
	// 803 可能不带昵称，保留 802 的昵称
	var scannedNick string
	var failures int
	for {
		select {
		case <-sessCtx.Done():
			if context.Cause(sessCtx) == errLoginCancelled {
				fmt.Println("\n已取消登录，bot 停止运行")
				a.finishStartupPrompts(runCtx, sess, render.LoginCancelledStopped)
				a.broadcastStartup(sess, render.LoginCancelledStopped, emptyKeyboard())
				cancelRun(errLoginCancelled)
			}
			return
		case <-runCtx.Done():
			return
		case <-deadline:
			a.startupLoginTimeout(runCtx, sess, cancelRun)
			return
		case <-countdown.C:
			fmt.Printf("二维码剩余有效期 %d 秒\n", remainSeconds(sess))
		case <-tgCountdown.C:
			remain := remainSeconds(sess)
			a.updateStartupPrompts(runCtx, sess, invalid, remain)
			if scannedNick == "" {
				a.broadcastStartup(sess, render.LoginQRCaption(remain), cancelLoginKeyboard())
			}
		case <-ticker.C:
			status, nickname, musicU, err := a.api.CheckQRLogin(key)
			if err != nil {
				failures++
				if failures >= 10 {
					a.log.Errorln(err)
					a.startupLoginTimeout(runCtx, sess, cancelRun)
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
					fmt.Printf("已扫码: %s，等待确认...\n", nickname)
					a.broadcastStartup(sess, render.LoginScanned(nickname), cancelLoginKeyboard())
				}
			case netease.QRSuccess:
				if nickname == "" {
					nickname = scannedNick
				}
				note := a.finishLogin(musicU, nickname)
				fmt.Printf("扫码登录成功: %s%s\n", nickname, note)
				a.finishStartupPrompts(runCtx, sess, render.LoginStartupDone)
				a.broadcastStartup(sess, render.LoginOK(nickname), emptyKeyboard())
				a.resumeDownloads(runCtx)
				go a.watchCookie(runCtx)
				return
			case netease.QRExpired:
				a.startupLoginTimeout(runCtx, sess, cancelRun)
				return
			}
		}
	}
}

func remainSeconds(sess *startupSession) int {
	// 向上取整，避免刚创建就少显示一秒
	remain := int(math.Ceil(time.Until(sess.expiresAt).Seconds()))
	if remain < 0 {
		remain = 0
	}
	return remain
}

func (a *App) startupLoginTimeout(runCtx context.Context, sess *startupSession, cancelRun context.CancelCauseFunc) {
	fmt.Println("\n二维码已过期，未登录，bot 停止运行")
	a.finishStartupPrompts(runCtx, sess, render.LoginTimeoutStopped)
	a.broadcastStartup(sess, render.LoginTimeoutStopped, emptyKeyboard())
	cancelRun(errLoginStopped)
}

func (a *App) newStartupQR() (key string, png []byte, qr *qrcode.QRCode, err error) {
	for attempt := 0; attempt < 3; attempt++ {
		key, err = a.api.CreateQRLoginKey()
		if err == nil {
			qr, err = qrcode.New(netease.QRLoginContent(key), qrcode.High)
			if err == nil {
				png, err = qr.PNG(512)
				if err == nil {
					return key, png, qr, nil
				}
			}
			return "", nil, nil, err
		}
		a.log.Errorln(err)
		time.Sleep(3 * time.Second)
	}
	return "", nil, nil, fmt.Errorf("获取扫码二维码失败")
}

func (a *App) sendStartupPrompts(ctx context.Context, sess *startupSession, invalid bool) {
	text := render.LoginStartupPrompt(invalid, remainSeconds(sess))
	for _, adminID := range a.cfg.Bot.Admin {
		msg, err := a.tg.SendMessage(ctx, &tgb.SendMessageParams{
			ChatID:      adminID,
			Text:        text,
			ParseMode:   models.ParseModeHTML,
			ReplyMarkup: loginPromptKeyboard(),
		})
		if err != nil {
			a.log.Warnf("通知管理员 %d 失败: %v", adminID, err)
			continue
		}
		a.startupMu.Lock()
		if a.startupQR == sess {
			sess.prompts[adminID] = msg.ID
		}
		a.startupMu.Unlock()
	}
}

func (a *App) updateStartupPrompts(ctx context.Context, sess *startupSession, invalid bool, remain int) {
	text := render.LoginStartupPrompt(invalid, remain)
	a.startupMu.Lock()
	prompts := make(map[int64]int, len(sess.prompts))
	for k, v := range sess.prompts {
		prompts[k] = v
	}
	a.startupMu.Unlock()
	for chatID, messageID := range prompts {
		_, err := a.tg.EditMessageText(ctx, &tgb.EditMessageTextParams{
			ChatID:      chatID,
			MessageID:   messageID,
			Text:        text,
			ParseMode:   models.ParseModeHTML,
			ReplyMarkup: loginPromptKeyboard(),
		})
		if err != nil {
			a.log.Errorln(err)
		}
	}
}

func (a *App) finishStartupPrompts(ctx context.Context, sess *startupSession, text string) {
	a.startupMu.Lock()
	prompts := make(map[int64]int, len(sess.prompts))
	for k, v := range sess.prompts {
		prompts[k] = v
	}
	a.startupMu.Unlock()
	for chatID, messageID := range prompts {
		_, err := a.tg.EditMessageText(ctx, &tgb.EditMessageTextParams{
			ChatID:      chatID,
			MessageID:   messageID,
			Text:        text,
			ParseMode:   models.ParseModeHTML,
			ReplyMarkup: emptyKeyboard(),
		})
		if err != nil {
			a.log.Errorln(err)
		}
	}
}

func (a *App) handleCallbackStartLogin(ctx context.Context, tg *tgb.Bot, update *models.Update) {
	query := update.CallbackQuery
	a.startupMu.Lock()
	sess := a.startupQR
	a.startupMu.Unlock()
	if sess == nil {
		_, _ = a.tg.AnswerCallbackQuery(ctx, &tgb.AnswerCallbackQueryParams{
			CallbackQueryID: query.ID,
			Text:            render.LoginNoSession,
		})
		return
	}
	photo, err := a.tg.SendPhoto(ctx, &tgb.SendPhotoParams{
		ChatID:      query.From.ID,
		Photo:       &models.InputFileUpload{Filename: "login-qr.png", Data: bytes.NewReader(sess.png)},
		Caption:     render.LoginQRCaption(remainSeconds(sess)),
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: cancelLoginKeyboard(),
	})
	if err != nil {
		a.log.Errorln(err)
		return
	}
	a.startupMu.Lock()
	if a.startupQR == sess {
		sess.surfaces[query.From.ID] = photo.ID
	}
	a.startupMu.Unlock()
	_, _ = a.tg.AnswerCallbackQuery(ctx, &tgb.AnswerCallbackQueryParams{
		CallbackQueryID: query.ID,
		Text:            render.LoginQRSent,
	})
}

func (a *App) broadcastStartup(sess *startupSession, text string, markup models.ReplyMarkup) {
	a.startupMu.Lock()
	surfaces := make(map[int64]int, len(sess.surfaces))
	for k, v := range sess.surfaces {
		surfaces[k] = v
	}
	a.startupMu.Unlock()
	for chatID, messageID := range surfaces {
		_, _ = a.tg.EditMessageCaption(context.Background(), &tgb.EditMessageCaptionParams{
			ChatID:      chatID,
			MessageID:   messageID,
			Caption:     text,
			ParseMode:   models.ParseModeHTML,
			ReplyMarkup: markup,
		})
	}
}

func loginPromptKeyboard() models.InlineKeyboardMarkup {
	return models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{{
			{Text: render.LoginStartButton, CallbackData: "startlogin"},
		}},
	}
}
