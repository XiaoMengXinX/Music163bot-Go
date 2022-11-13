package main

import (
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/XiaoMengXinX/Music163bot-Go/v3/bot"
	"github.com/XiaoMengXinX/Music163bot-Go/v3/config"
	"github.com/XiaoMengXinX/Music163bot-Go/v3/utils/db"
	"github.com/XiaoMengXinX/Music163bot-Go/v3/utils/logger"
	ps "github.com/XiaoMengXinX/Music163bot-Go/v3/utils/process"
	"github.com/sirupsen/logrus"
)

var log = logger.Logger

func init() {
	logrus.RegisterExitHandler(ps.KillDaemonProcess)
	logger.InitLogger(log)
}

var configPath *string

func init() {
	f := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	configPath = f.String("c", "config.ini", "The path of your config file")
	_ = f.Parse(os.Args[1:])
}

func init() {
	switch ps.GetProcessStatus() {
	case ps.StatusStart, ps.StartsUpdate:
	default:
		ps.SetDaemonPidEnv()
		ps.SetProcessStatus(ps.StatusStart)
		pid, err := ps.ForkProcess(ps.StatusStart)
		if err != nil {
			_ = ps.KillProcess(pid)
			log.Fatalln(err)
		}
		log.Println("守护进程已启动")
		for {
			pid, err = startDaemon(pid)
			if err != nil {
				_ = ps.KillProcess(pid)
				log.Fatalln(err)
			}
		}
	}
}

func startDaemon(pid int) (int, error) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGUSR1, syscall.SIGUSR2)
	for {
		time.Sleep(1 * time.Second)
		select {
		case <-signals:
			_ = ps.KillProcess(pid)
			os.Exit(0)
		default:
			if !ps.CheckProcessAlive(pid) {
				log.Println("检测到主进程退出，重启中...")
				return ps.ForkProcess(ps.StatusStart)
			}
		}
	}
}

func main() {
	defer ps.KillDaemonProcess()
	configData, err := config.ReadConfig(*configPath)
	if err != nil {
		log.Fatalln(err)
	}
	if configData.General.LogPath != "" {
		err = logger.SetLoggerOutputPath(log, configData.General.LogPath)
		if err != nil {
			log.Fatalln(err)
		}
	}
	err = db.InitDB(configData.Sqlite)
	if err != nil {
		log.Fatalln(err)
	}
	bot.StartHandle(configData)
}
