package bot

import (
	"context"
	"time"

	tgb "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/config"
	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/render"
)

// cookie 监控：失效暂停下载并提醒管理员，重新登录后恢复

const (
	cookieCheckInterval = 10 * time.Minute
	cookieWarnInterval  = 10 * time.Minute
)

// watchCookie 周期校验登录态，失效即暂停下载
func (a *App) watchCookie(ctx context.Context) {
	ticker := time.NewTicker(cookieCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			valid, err := a.api.LoginStatusValid()
			if err != nil {
				// 网络原因无法判定时跳过本轮
				continue
			}
			if !valid {
				a.pauseDownloads(ctx)
			}
		}
	}
}

// pauseDownloads 暂停新歌下载并启动管理员告警循环
func (a *App) pauseDownloads(ctx context.Context) {
	if !a.cookieOK.CompareAndSwap(true, false) {
		return // 已处于暂停状态
	}
	a.log.Warnln("MUSIC_U 已失效，暂停新歌下载")
	a.notifyAdmins(ctx, render.CookieExpiredAdminWarn, nil)

	warnCtx, cancel := context.WithCancel(context.Background())
	a.warnMu.Lock()
	a.warnCancel = cancel
	a.warnMu.Unlock()
	go func() {
		ticker := time.NewTicker(cookieWarnInterval)
		defer ticker.Stop()
		for {
			select {
			case <-warnCtx.Done():
				return
			case <-ctx.Done():
				return // bot 关机/重载
			case <-ticker.C:
				if !a.cookieOK.Load() {
					a.notifyAdmins(ctx, render.CookieExpiredAdminWarn, nil)
				}
			}
		}
	}()
}

// resumeDownloads 恢复下载；此前由监控暂停过（告警循环在跑）时通知管理员
func (a *App) resumeDownloads(ctx context.Context) {
	a.warnMu.Lock()
	warnCancel := a.warnCancel
	a.warnCancel = nil
	a.warnMu.Unlock()
	if warnCancel != nil {
		warnCancel()
	}
	resumed := a.cookieOK.CompareAndSwap(false, true)
	if warnCancel != nil && resumed {
		a.log.Println("MUSIC_U 已生效，恢复新歌下载")
		a.notifyAdmins(ctx, render.CookieResumedAdmin, nil)
	}
}

// finishLogin 登录核心：热更新 cookie、回写配置、恢复下载。
// 返回附加提示（如写回失败）
func (a *App) finishLogin(musicU, nickname string) string {
	a.api.SetMUSICU(musicU)
	a.cfg.Netease.MUSICU = musicU
	note := ""
	if err := config.UpdateMUSICU(a.cfgPath, musicU); err != nil {
		a.log.Errorln(err)
		note = "\n配置文件写入失败: " + err.Error() + "\n本次登录已生效，重启后将失效"
	}
	a.resumeDownloads(context.Background())
	a.log.Printf("网易云扫码登录成功: %s", nickname)
	return note
}

// notifyAdmins 给所有管理员发通知（发送失败仅记日志）
func (a *App) notifyAdmins(ctx context.Context, text string, markup models.ReplyMarkup) {
	for _, adminID := range a.cfg.Bot.Admin {
		_, err := a.tg.SendMessage(ctx, &tgb.SendMessageParams{
			ChatID:      adminID,
			Text:        text,
			ParseMode:   models.ParseModeHTML,
			ReplyMarkup: markup,
		})
		if err != nil {
			a.log.Warnf("通知管理员 %d 失败: %v", adminID, err)
		}
	}
}
