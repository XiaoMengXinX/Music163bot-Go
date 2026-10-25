package bot

import (
	"context"
	"runtime/debug"
	"slices"

	tgb "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// recoverMiddleware panic 恢复
func (a *App) recoverMiddleware(next tgb.HandlerFunc) tgb.HandlerFunc {
	return func(ctx context.Context, tg *tgb.Bot, update *models.Update) {
		defer func() {
			if r := recover(); r != nil {
				a.log.Errorf("panic: %v\n%s", r, debug.Stack())
			}
		}()
		next(ctx, tg, update)
	}
}

// adminOnly 管理员守卫
func (a *App) adminOnly(next tgb.HandlerFunc) tgb.HandlerFunc {
	return func(ctx context.Context, tg *tgb.Bot, update *models.Update) {
		var userID int64
		switch {
		case update.Message != nil && update.Message.From != nil:
			userID = update.Message.From.ID
		case update.CallbackQuery != nil:
			userID = update.CallbackQuery.From.ID
		default:
			return
		}
		if !slices.Contains(a.cfg.Bot.Admin, userID) {
			return
		}
		next(ctx, tg, update)
	}
}
