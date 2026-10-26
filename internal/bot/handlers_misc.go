package bot

import (
	"context"
	"strconv"
	"time"

	tgb "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/config"
	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/render"
	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/textx"
)

func (a *App) handleAbout(ctx context.Context, tg *tgb.Bot, update *models.Update) {
	m := update.Message
	if _, err := a.reply(ctx, m, render.About(a.build.RuntimeVer, a.build.VersionName, a.build.CommitSHA, a.build.BuildTime, a.build.BuildArch)); err != nil {
		a.log.Errorln(err)
	}
}

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

func (a *App) handleRmCache(ctx context.Context, tg *tgb.Bot, update *models.Update) {
	m := update.Message
	args := commandArguments(m)
	var musicID int
	if args == "" && m.ReplyToMessage != nil {
		id, _, err := a.messageMusic(m.ReplyToMessage)
		if err != nil {
			a.log.Errorln(err)
			if _, err := a.reply(ctx, m, render.FetchInfoFailed); err != nil {
				a.log.Errorln(err)
			}
			return
		}
		musicID = id
	} else {
		musicID = textx.ParseMusicID(args)
		if musicID == 0 {
			if pid := textx.ParseProgramID(args); pid != 0 {
				musicID = a.api.ProgramRealID(pid)
			}
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

func (a *App) handleReload(ctx context.Context, tg *tgb.Bot, update *models.Update) {
	m := update.Message
	if _, err := a.reply(ctx, m, render.Reloading); err != nil {
		a.log.Errorln(err)
	}
	a.reload(ErrReload)
}

// handleSetCacheChat 无参数时设为当前聊天，参数为 0 时关闭
func (a *App) handleSetCacheChat(ctx context.Context, tg *tgb.Bot, update *models.Update) {
	m := update.Message
	chatID := m.Chat.ID
	if args := commandArguments(m); args != "" {
		id, err := strconv.ParseInt(args, 10, 64)
		if err != nil {
			if _, err := a.reply(ctx, m, render.InputChatID); err != nil {
				a.log.Errorln(err)
			}
			return
		}
		chatID = id
	}
	if chatID != 0 && chatID != m.Chat.ID {
		if _, err := a.sendMessage(ctx, chatID, render.CacheChatTest, 0); err != nil {
			a.log.Errorln(err)
			if _, err := a.reply(ctx, m, render.CacheChatUnreachable); err != nil {
				a.log.Errorln(err)
			}
			return
		}
	}

	a.cacheChat.Store(chatID)
	text := render.CacheChatDisabled
	if chatID != 0 {
		text = render.CacheChatSet(chatID)
	}
	if err := config.UpdateCacheChat(a.cfgPath, chatID); err != nil {
		a.log.Errorln(err)
		text += "\n配置文件写入失败: " + render.Escape(err.Error()) + "\n本次设置已生效，重启后将失效"
	}
	if _, err := a.reply(ctx, m, text); err != nil {
		a.log.Errorln(err)
	}
}
