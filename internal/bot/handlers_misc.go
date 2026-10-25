package bot

import (
	"context"
	"time"

	tgb "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/render"
	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/textx"
)

// handleAbout /about
func (a *App) handleAbout(ctx context.Context, tg *tgb.Bot, update *models.Update) {
	m := update.Message
	if _, err := a.reply(ctx, m, render.About(a.build.RuntimeVer, a.build.VersionName, a.build.CommitSHA, a.build.BuildTime, a.build.BuildArch)); err != nil {
		a.log.Errorln(err)
	}
}

// handleStatus /status 统计信息（串行 + 500ms 限流）
func (a *App) handleStatus(ctx context.Context, tg *tgb.Bot, update *models.Update) {
	m := update.Message
	if err := a.statusSem.Acquire(ctx, 1); err != nil {
		return
	}
	defer func() {
		time.Sleep(500 * time.Millisecond)
		a.statusSem.Release(1)
	}()

	total, byUser, byChat := a.store.Counts(m.From.ID, m.Chat.ID)
	text := render.StatusInfo(m.Chat.Title, m.Chat.Username, m.Chat.ID, m.From.ID, total, byChat, byUser)
	if _, err := a.reply(ctx, m, text); err != nil {
		a.log.Errorln(err)
	}
}

// handleRmCache /rmcache（管理员）：清除指定歌曲缓存
func (a *App) handleRmCache(ctx context.Context, tg *tgb.Bot, update *models.Update) {
	m := update.Message
	args := commandArguments(m)
	musicID := textx.ParseMusicID(args)
	if musicID == 0 {
		if pid := textx.ParseProgramID(args); pid != 0 {
			musicID = a.api.ProgramRealID(pid)
		}
	}
	if musicID == 0 {
		text := render.NoResults
		if args == "" {
			text = render.InputIDorLink
		}
		if _, err := a.reply(ctx, m, text); err != nil {
			a.log.Errorln(err)
		}
		return
	}
	name, ok, err := a.store.DeleteByMusicID(musicID)
	if err != nil {
		a.log.Errorln(err)
		return
	}
	text := render.NoCache
	if ok {
		text = render.RmcacheReport(name)
	}
	if _, err := a.reply(ctx, m, text); err != nil {
		a.log.Errorln(err)
	}
}

// handleReload /reload（管理员）：取消当前运行，由 main 重新加载配置后再启动
func (a *App) handleReload(ctx context.Context, tg *tgb.Bot, update *models.Update) {
	m := update.Message
	if _, err := a.reply(ctx, m, render.Reloading); err != nil {
		a.log.Errorln(err)
	}
	a.reload(ErrReload)
}
