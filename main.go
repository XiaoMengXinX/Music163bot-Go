package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"runtime"
	"strings"
	"time"

	"github.com/XiaoMengXinX/Music163bot-Go/v2/bot"
	"github.com/sirupsen/logrus"
)

var config map[string]string

var (
	_ConfigPath *string
	_NoUpdate   *bool
	_NoMD5Check *bool
	_SrcPath    *string
	_BotEntry   *string
)

var (
	runtimeVer   = fmt.Sprintf(runtime.Version())                     // 编译环境
	_VersionName = ""                                                 // 程序版本
	commitSHA    = ""                                                 // 编译哈希
	buildTime    = ""                                                 // 编译日期
	buildArch    = fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH) // 运行环境
	repoPath     = ""                                                 // 项目地址
	rawRepoPath  = ""
)

// LogFormatter 自定义 log 格式
type LogFormatter struct{}

// Format 自定义 log 格式
func (s *LogFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	timestamp := time.Now().Local().Format("2006/01/02 15:04:05")
	var msg string
	msg = fmt.Sprintf("%s [%s] %s (%s:%d)\n", timestamp, strings.ToUpper(entry.Level.String()), entry.Message, path.Base(entry.Caller.File), entry.Caller.Line)
	return []byte(msg), nil
}

func init() {
	logrus.SetFormatter(&logrus.TextFormatter{
		DisableColors:          false,
		FullTimestamp:          true,
		DisableLevelTruncation: true,
		PadLevelText:           true,
	})
	logrus.SetFormatter(new(LogFormatter))
	logrus.SetReportCaller(true)
	dirExists("./log")
	timeStamp := time.Now().Local().Format("2006-01-02")
	logFile := fmt.Sprintf("./log/%v.log", timeStamp)
	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		logrus.Errorln(err)
	}
	output := io.MultiWriter(os.Stdout, file)
	logrus.SetOutput(output)
	if config["LogLevel"] != "" {
		level, err := logrus.ParseLevel(config["LogLevel"])
		if err != nil {
			logrus.Errorln(err)
		} else {
			logrus.SetLevel(level)
		}
	}

	f := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	_ConfigPath = f.String("c", "config.ini", "配置文件")
	_NoUpdate = f.Bool("no-update", false, "关闭更新检测")
	_NoMD5Check = f.Bool("no-md5-check", false, "关闭 md5 效验")
	_SrcPath = f.String("path", "./src", "自定义更新下载/加载路径")
	_BotEntry = f.String("bot-entry", "bot.Start", "自定义动态加载入口")
	_ = f.Parse(os.Args[1:])

	logrus.Printf("Music163bot-Go %s", _VersionName)

	conf, err := readConfig(*_ConfigPath)
	if err != nil {
		logrus.Errorln("读取配置文件失败，请检查配置文件")
		logrus.Fatal(err)
	}
	config = conf
	initConfig(config)
}

func main() {
	var actionCode = -1 // actionCode: 0 exit, 1 error exit, 2 reload src, 3 update src

	for true {
		logrus.Printf("加载内置版本 %s 中", _VersionName)
		actionCode = bot.Start(config)
		switch actionCode {
		case 0:
			os.Exit(0)
		case 1:
			logrus.Fatal("Unexpected error")
		case 2:
			time.Sleep(2 * time.Second)
			conf, err := readConfig(*_ConfigPath)
			if err != nil {
				logrus.Errorln(err)
				logrus.Fatal("读取配置文件失败，请检查配置文件")
			} else {
				config = conf
				initConfig(config)
			}
			continue
		case 3:
			time.Sleep(2 * time.Second)
			continue
		}
	}
}

func readConfig(path string) (config map[string]string, err error) {
	config = make(map[string]string)
	f, err := os.Open(path)
	if err != nil {
		return config, err
	}
	defer func(f *os.File) {
		e := f.Close()
		if e != nil {
			err = e
		}
	}(f)
	r := bufio.NewReader(f)
	for {
		b, _, err := r.ReadLine()
		if err != nil {
			if err == io.EOF {
				break
			}
			return config, err
		}
		s := strings.TrimSpace(string(b))
		index := strings.Index(s, "=")
		if index < 0 {
			continue
		}
		key := strings.TrimSpace(s[:index])
		if len(key) == 0 {
			continue
		}
		value := strings.TrimSpace(s[index+1:])
		if len(value) == 0 {
			continue
		}
		config[key] = value
	}
	return config, err
}

func dirExists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		err := os.Mkdir(path, os.ModePerm)
		if err != nil {
			logrus.Errorf("mkdir %v failed: %v\n", path, err)
		}
		return false
	}
	logrus.Errorf("Error: %v\n", err)
	return false
}

func initConfig(config map[string]string) {
	config["BinVersionName"] = _VersionName
	config["runtimeVer"] = runtimeVer
	config["buildTime"] = buildTime
	config["commitSHA"] = commitSHA
	config["buildArch"] = buildArch
	config["repoPath"] = repoPath
	config["rawRepoPath"] = rawRepoPath
	if *_NoUpdate {
		config["AutoUpdate"] = "false"
	} else if config["AutoUpdate"] == "false" {
		*_NoUpdate = true
	}
	if *_NoMD5Check {
		config["CheckMD5"] = "false"
	} else if config["CheckMD5"] == "false" {
		*_NoMD5Check = true
	}
	if config["SrcPath"] != "" {
		*_SrcPath = config["SrcPath"]
	} else {
		config["SrcPath"] = *_SrcPath
	}
	if config["BotEntry"] != "" {
		*_BotEntry = config["BotEntry"]
	}
}
