// Package bot 装配全部依赖并处理 Telegram 更新
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

// errLoginStopped 启动登录超时/取消导致停机的取消原因
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

// App 持有全部依赖
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
	musicSem      *semaphore.Weighted     // 下载并发
	statusSem     *semaphore.Weighted     // /status 限流
	loginMu       sync.Mutex              // 同时只允许一个扫码登录会话
	loginCancelMu sync.Mutex              // 保护 loginCancel 读写
	loginCancel   context.CancelCauseFunc // 当前登录会话的取消函数
	startupMu     sync.Mutex              // 保护 startupQR
	startupQR     *startupSession         // 非 nil 表示启动登录进行中
	cookieOK      atomic.Bool             // false 时暂停新歌下载
	warnMu        sync.Mutex              // 保护 warnCancel
	warnCancel    context.CancelFunc      // cookie 失效告警循环的取消函数
	reload        context.CancelCauseFunc
}

// Run 装配并阻塞运行，直到收到信号或 /reload；返回 true 表示需重新加载配置。
// loginState 非 LoginNone 时（Cookie 缺失或失效）bot 照常启动并引导扫码登录，
// 超时未登录则自动停止运行。
func Run(ctx context.Context, cfg *config.Config, cfgPath string, log *logrus.Logger, build BuildInfo, loginState LoginState) (reloadRequested bool, err error) {
	apiHTTP, err := cfg.Netease.HTTPClient()
	if err != nil {
		return false, err
	}
	defer apiHTTP.CloseIdleConnections()

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
		api:       netease.New(cfg.Netease.MUSICU, cfg.Netease.Level, apiHTTP),
		dl:        fetch.New(cacheDir, cfg.Download.Timeout.D(), cfg.Download.Retries, cfg.Download.ReverseProxy, cfg.Download.AutoRetry),
		recog:     recognize.New(apiHTTP),
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
		tgb.WithSkipGetMe(), // 由下方 GetMe 统一完成 token 校验并取得 bot 用户名
		tgb.WithAllowedUpdates(tgb.AllowedUpdates{"message", "callback_query", "inline_query"}), // 不显式声明会沿用服务端持久设置，收不到回调

		tgb.WithMiddlewares(a.recoverMiddleware),
		tgb.WithDefaultHandler(a.handleDefault),
		tgb.WithErrorsHandler(func(err error) { log.Errorf("telegram: %v", err) }),
	}
	if cfg.Bot.Debug {
		opts = append(opts, tgb.WithDebug())
	}
	tg, err := tgb.New(cfg.Bot.Token, opts...)
	if err != nil {
		return false, err
	}
	a.tg = tg

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

	tg.RegisterHandler(tgb.HandlerTypeCallbackQueryData, "music ", tgb.MatchTypePrefix, a.handleCallbackMusic)
	tg.RegisterHandler(tgb.HandlerTypeCallbackQueryData, "cancellogin", tgb.MatchTypeExact, a.handleCallbackCancelLogin, a.adminOnly)
	tg.RegisterHandler(tgb.HandlerTypeCallbackQueryData, "startlogin", tgb.MatchTypeExact, a.handleCallbackStartLogin, a.adminOnly)
	tg.RegisterHandlerMatchFunc(func(u *models.Update) bool { return u.InlineQuery != nil }, a.handleInline)
}

// command 注册命令 handler，匹配 /cmd 或 /cmd@本bot
func (a *App) command(tg *tgb.Bot, name string, h tgb.HandlerFunc, m ...tgb.Middleware) {
	tg.RegisterHandlerMatchFunc(matchCommand(name, a.me.Username), h, m...)
}

// matchCommand 首个实体必须是位于消息开头的 bot_command
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

// commandArguments 返回命令参数，跳过命令后紧跟的一个字符（tgbotapi 语义）
func commandArguments(m *models.Message) string {
	e := m.Entities[0]
	if len(m.Text) == e.Length {
		return ""
	}
	return m.Text[e.Length+1:]
}
