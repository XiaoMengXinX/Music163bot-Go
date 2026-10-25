// music163bot v3 入口
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/sirupsen/logrus"

	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/bot"
	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/config"
	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/logging"
	"github.com/XiaoMengXinX/Music163bot-Go/v3/internal/netease"
)

var (
	versionName = "v3.0.0"
	commitSHA   = ""
	buildTime   = ""
)

func main() {
	build := bot.BuildInfo{
		RuntimeVer:  runtime.Version(),
		VersionName: versionName,
		CommitSHA:   commitSHA,
		BuildTime:   buildTime,
		BuildArch:   fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}

	if len(os.Args) > 1 && os.Args[1] == "migrate-config" {
		runMigrateConfig(os.Args[2:])
		return
	}

	cfgPath := flag.String("c", "config.toml", "配置文件路径")
	flag.Parse()

	autoMigrateINI(*cfgPath)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	for {
		cfg, err := config.Load(*cfgPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
			os.Exit(1)
		}
		log := logging.Setup(cfg.Log.Level, cfg.Log.Path)
		log.Printf("Music163bot-Go %s 启动中", build.VersionName)

		// MUSIC_U 缺失或失效时带着启动登录流程运行；网络原因无法判定则按现状启动
		loginState := checkLogin(cfg, log)

		reload, err := bot.Run(ctx, cfg, *cfgPath, log, build, loginState)
		if err != nil {
			log.Errorln(err)
			os.Exit(1)
		}
		if !reload {
			return
		}
		log.Println("/reload：重新加载配置中")
		// 注意：此处不能 stop()——reload 要带着同一个信号 ctx 进入下一轮循环
	}
}

// checkLogin 校验 MUSIC_U；返回启动时的 Cookie 状态
func checkLogin(cfg *config.Config, log *logrus.Logger) bot.LoginState {
	if cfg.Netease.MUSICU == "" {
		return bot.LoginEmpty
	}
	valid, err := netease.New(cfg.Netease.MUSICU, cfg.Netease.Level).LoginStatusValid()
	if err != nil {
		log.Warnf("无法验证 MUSIC_U 有效性: %v", err)
		return bot.LoginNone
	}
	if !valid {
		return bot.LoginInvalid
	}
	return bot.LoginNone
}

// autoMigrateINI 配置文件不存在而同目录有 v2 的 config.ini 时，自动迁移并提示
func autoMigrateINI(cfgPath string) {
	if _, err := os.Stat(cfgPath); err == nil {
		return
	} else if !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "读取配置文件失败: %v\n", err)
		os.Exit(1)
	}
	iniPath := filepath.Join(filepath.Dir(cfgPath), "config.ini")
	if _, err := os.Stat(iniPath); err != nil {
		fmt.Fprintf(os.Stderr, "配置文件 %s 不存在（可运行 `music163bot migrate-config -c config.ini -o %s` 从 v2 配置迁移）\n", cfgPath, cfgPath)
		os.Exit(1)
	}
	_, notes, err := config.MigrateINIFile(iniPath, cfgPath)
	fmt.Fprintf(os.Stderr, "检测到 v2 配置文件 %s，已自动迁移为 %s\n", iniPath, cfgPath)
	for _, n := range notes {
		fmt.Fprintf(os.Stderr, "迁移提示: %s\n", n)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "配置迁移失败: %v\n", err)
		os.Exit(1)
	}
}

func runMigrateConfig(args []string) {
	fs := flag.NewFlagSet("migrate-config", flag.ExitOnError)
	inPath := fs.String("c", "config.ini", "v2 配置文件 (INI)")
	outPath := fs.String("o", "config.toml", "输出 v3 配置文件 (TOML)")
	_ = fs.Parse(args)

	_, notes, err := config.MigrateINIFile(*inPath, *outPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "迁移失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("已将 %s 迁移为 %s：\n\n", *inPath, *outPath)
	for _, n := range notes {
		fmt.Printf("提示: %s\n", n)
	}
	data, err := os.ReadFile(*outPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取生成结果失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(data))
}
