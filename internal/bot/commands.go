package bot

import (
	"context"

	tgb "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// setupCommands 启动时注册指令菜单，覆盖 BotFather 的手动设置。
// 管理员私聊必须注册「公共 + 管理员」合并列表：
// chat scope 会整体覆盖 default scope，只注册管理员指令会让管理员看不到公共指令。
func (a *App) setupCommands(ctx context.Context, tg *tgb.Bot) {
	userCommands := []models.BotCommand{
		{Command: "music", Description: "点歌（ID 或关键词）"},
		{Command: "netease", Description: "点歌（ID 或关键词）"},
		{Command: "search", Description: "搜索歌曲"},
		{Command: "lyric", Description: "获取歌词"},
		{Command: "program", Description: "电台节目点歌"},
		{Command: "recognize", Description: "语音识别搜歌"},
		{Command: "status", Description: "缓存统计"},
		{Command: "about", Description: "关于本 bot"},
	}
	if _, err := tg.SetMyCommands(ctx, &tgb.SetMyCommandsParams{
		Commands: userCommands,
		Scope:    &models.BotCommandScopeDefault{},
	}); err != nil {
		a.log.Warnf("注册指令菜单失败: %v", err)
	}

	// 管理员私聊：公共指令 + 管理员指令
	adminCommands := append(append([]models.BotCommand{}, userCommands...),
		models.BotCommand{Command: "login", Description: "网易云扫码登录"},
		models.BotCommand{Command: "rmcache", Description: "清除歌曲缓存"},
		models.BotCommand{Command: "reload", Description: "重载配置"},
	)
	for _, adminID := range a.cfg.Bot.Admin {
		if _, err := tg.SetMyCommands(ctx, &tgb.SetMyCommandsParams{
			Commands: adminCommands,
			Scope:    &models.BotCommandScopeChat{ChatID: adminID},
		}); err != nil {
			a.log.Warnf("注册管理员指令失败 (chat %d): %v（该管理员可能尚未与 bot 私聊过，私聊一次后重启即可）", adminID, err)
		}
	}
}
