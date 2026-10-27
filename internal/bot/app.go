// Package bot 处理 Telegram 更新
package bot

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	tgb "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/semaphore"

	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/config"
	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/fetch"
	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/logging"
	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/netease"
	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/recognize"
	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/store"
)

// ErrReload /reload 触发的取消原因
var ErrReload = errors.New("reload requested")

var errLoginStopped = errors.New("startup login not completed")

// LoginState 启动时的 Cookie 状态
type LoginState int

const (
	LoginNone    LoginState = iota // 正常
	LoginEmpty                     // 未配置
	LoginInvalid                   // 已失效
)

// BuildInfo 编译信息，由 ldflags 注入
type BuildInfo struct {
	RuntimeVer  string
	VersionName string
	CommitSHA   string
	BuildTime   string
	BuildArch   string
}

type App struct {
	cfg           *config.Config
	cfgPath       string
	tg            *tgb.Bot
	me            *models.User
	store         *store.Store
	api           *netease.Client
	dl            *fetch.Downloader
	recog         *recognize.Client
	log           *logrus.Logger
	build         BuildInfo
	cacheDir      string
	http          *http.Client
	musicSem      *semaphore.Weighted
	statusSem     *semaphore.Weighted
	loginMu       sync.Mutex // 同时只允许一个扫码登录会话
	loginCancelMu sync.Mutex // 保护 loginCancel 读写
	loginCancel   context.CancelCauseFunc
	startupMu     sync.Mutex      // 保护 startupQR
	startupQR     *startupSession // 非 nil 表示启动登录进行中
	cookieOK      atomic.Bool     // false 时暂停新歌下载
	cacheChat     atomic.Int64    // 0 表示未设置缓存群
	warnMu        sync.Mutex      // 保护 warnCancel
	warnCancel    context.CancelFunc
	reload        context.CancelCauseFunc
	cooldownMu    sync.Mutex                // 保护 cooldowns
	cooldowns     map[cooldownKey]time.Time // 冷却截止时间
}

// Run 阻塞运行 bot，返回 true 表示需重载配置
// Cookie 缺失或失效时引导扫码登录，超时则停止
func Run(ctx context.Context, cfg *config.Config, cfgPath string, log *logrus.Logger, build BuildInfo, loginState LoginState, client *http.Client) (reloadRequested bool, err error) {
	cacheDir := "./cache"
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return false, err
	}

	st, err := store.Open(cfg.Database.Path, logging.GormLogger(log, cfg.Log.Level))
	if err != nil {
		return false, err
	}
	defer st.Close()

	runCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	a := &App{
		cfg:       cfg,
		cfgPath:   cfgPath,
		store:     st,
		api:       netease.New(cfg.Netease.MUSICU, cfg.Netease.Level, client),
		dl:        fetch.New(cacheDir, cfg.Download.Timeout.D(), cfg.Download.Retries, cfg.Download.ReverseProxy, cfg.Download.AutoRetry),
		recog:     recognize.New(client),
		log:       log,
		build:     build,
		cacheDir:  cacheDir,
		http:      &http.Client{Timeout: 60 * time.Second},
		musicSem:  semaphore.NewWeighted(int64(cfg.Download.Concurrency)),
		statusSem: semaphore.NewWeighted(1),
		reload:    cancel,
	}

	opts := []tgb.Option{
		tgb.WithServerURL(cfg.Bot.APIURL),
		tgb.WithHTTPClient(min(time.Minute, cfg.Bot.UploadTimeout.D()), &http.Client{Timeout: cfg.Bot.UploadTimeout.D()}),
		tgb.WithSkipGetMe(), // 统一在下方 GetMe 校验
		tgb.WithAllowedUpdates(tgb.AllowedUpdates{"message", "callback_query", "inline_query", "guest_message"}), // 避免沿用服务端设置而漏收回调

		tgb.WithMiddlewares(a.recoverMiddleware),
		tgb.WithDefaultHandler(a.handleDefault),
		tgb.WithErrorsHandler(func(err error) {
			// 停止或重载时中断的轮询请求不是错误
			if runCtx.Err() == nil {
				log.Errorf("telegram: %v", err)
			}
		}),
	}
	if cfg.Bot.Debug {
		opts = append(opts, tgb.WithDebug())
	}
	tg, err := tgb.New(cfg.Bot.Token, opts...)
	if err != nil {
		return false, err
	}
	a.tg = tg
	a.cacheChat.Store(cfg.Bot.CacheChat)

	me, err := tg.GetMe(runCtx)
	if err != nil {
		return false, err
	}
	a.me = me
	log.Printf("%s 验证成功", me.Username)

	a.registerHandlers(tg)
	a.setupCommands(runCtx, tg)
	if loginState != LoginNone {
		a.cookieOK.Store(false)
		go a.startupLogin(runCtx, cancel, loginState == LoginInvalid)
	} else {
		a.cookieOK.Store(true)
		go a.watchCookie(runCtx)
	}
	tg.Start(runCtx)

	cause := context.Cause(runCtx)
	if errors.Is(cause, ErrReload) {
		return true, nil
	}
	if errors.Is(cause, errLoginStopped) {
		log.Println("启动登录未完成，bot 已停止")
		return false, nil
	}
	return false, nil
}

func (a *App) registerHandlers(tg *tgb.Bot) {
	a.command(tg, "start", a.handleStart)
	a.command(tg, "music", a.handleMusic)
	a.command(tg, "netease", a.handleMusic)
	a.command(tg, "program", a.handleProgram)
	a.command(tg, "lyric", a.handleLyric)
	a.command(tg, "search", a.handleSearch)
	a.command(tg, "recognize", a.handleRecognize)
	a.command(tg, "about", a.handleAbout)
	a.command(tg, "status", a.handleStatus)
	a.command(tg, "rmcache", a.handleRmCache, a.adminOnly)
	a.command(tg, "reload", a.handleReload, a.adminOnly)
	a.command(tg, "login", a.handleLogin, a.adminOnly)
	a.command(tg, "setcachechat", a.handleSetCacheChat, a.adminOnly)

	tg.RegisterHandler(tgb.HandlerTypeCallbackQueryData, "music ", tgb.MatchTypePrefix, a.handleCallbackMusic)
	tg.RegisterHandler(tgb.HandlerTypeCallbackQueryData, "search ", tgb.MatchTypePrefix, a.handleCallbackSearch)
	tg.RegisterHandler(tgb.HandlerTypeCallbackQueryData, "cancellogin", tgb.MatchTypeExact, a.handleCallbackCancelLogin, a.adminOnly)
	tg.RegisterHandler(tgb.HandlerTypeCallbackQueryData, "startlogin", tgb.MatchTypeExact, a.handleCallbackStartLogin, a.adminOnly)
	tg.RegisterHandlerMatchFunc(func(u *models.Update) bool { return u.InlineQuery != nil }, a.handleInline)
	tg.RegisterHandlerMatchFunc(func(u *models.Update) bool { return u.GuestMessage != nil }, a.handleGuest)
}

func (a *App) command(tg *tgb.Bot, name string, h tgb.HandlerFunc, m ...tgb.Middleware) {
	tg.RegisterHandlerMatchFunc(matchCommand(name, a.me.Username), h, m...)
}

func matchCommand(name, botUsername string) tgb.MatchFunc {
	return func(update *models.Update) bool {
		msg := update.Message
		if msg == nil || len(msg.Entities) == 0 {
			return false
		}
		e := msg.Entities[0]
		if e.Offset != 0 || e.Type != models.MessageEntityTypeBotCommand {
			return false
		}
		cmd := msg.Text[1:e.Length]
		if i := strings.IndexByte(cmd, '@'); i >= 0 {
			return cmd[:i] == name && cmd[i+1:] == botUsername
		}
		return cmd == name
	}
}

// commandArguments 沿用旧版语义，只跳过命令后的一个字符
func commandArguments(m *models.Message) string {
	e := m.Entities[0]
	if len(m.Text) == e.Length {
		return ""
	}
	return m.Text[e.Length+1:]
}
