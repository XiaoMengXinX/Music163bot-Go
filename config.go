package main

import (
	"fmt"
	"github.com/EDDYCJY/gsema"
	songdownloader "github.com/XiaoMengXinX/NeteaseCloudApi-Go/tools/SongDownloader/utils"
	"github.com/XiaoMengXinX/NeteaseCloudApi-Go/utils"
	"github.com/robfig/cron"
	log "github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"io"
	"net/http"
	"os"
	"path"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const (
	picPath       = "./pic"
	musicPath     = "./music"
	fileNameStyle = 1
)

var DB *gorm.DB
var minuteLimitation int64 = 5
var options, cookies map[string]interface{}
var botName string
var processTreads = gsema.NewSemaphore(10)
var downloadTreads = gsema.NewSemaphore(2)
var readConfig = func() map[string]string {
	config, err := utils.ReadConfig("./config.ini")
	if err != nil {
		log.Errorln("读取配置文件失败，请检查配置文件")
		log.Fatal(err)
	}
	return config
}
var config = readConfig()

/*
   网易云分享链接的两种格式：
   https://music.163.com/song?id=1436919586&userid=2333
   https://y.music.163.com/m/song/28941713/?userid=376740360
*/

var (
	reg1 = regexp.MustCompile(`(.*)song\?id=`)
	reg2 = regexp.MustCompile("(.*)song/")
	reg3 = regexp.MustCompile("/(.*)")
	reg4 = regexp.MustCompile("&(.*)")
)

var (
	RUNTIME_VERSION = fmt.Sprintf(runtime.Version())                     // 编译环境
	VERSION         = ""                                                 // 程序版本
	COMMIT_SHA      = ""                                                 // 编译哈希
	BUILD_TIME      = ""                                                 // 编译日期
	BUILD_OS        = ""                                                 // 编译系统
	BUILD_ARCH      = fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH) // 运行环境

)

type LogFormatter struct{}
type SongInfo struct {
	gorm.Model
	MusicID      string
	SongName     string
	SongArtists  string
	SongAlbum    string
	FileExt      string
	FileSize     string
	BitRate      int
	Duration     int
	FileID       string
	ThumbFileID  string
	FromUserID   int64
	FromUserName string
	FromChatID   int64
	FromChatName string
}
type DownloadingMusic struct {
	MusicID string
}
type RequestTimes struct {
	TimeMinute string
	ChatID     int64
}

func (s *LogFormatter) Format(entry *log.Entry) ([]byte, error) {
	timestamp := time.Now().Local().Format("2006/01/02 15:04:05")
	var msg string
	if config["BotDebug"] == "true" {
		msg = fmt.Sprintf("%s [%s] %s (%s:%d) [%s]\n", timestamp, strings.ToUpper(entry.Level.String()), entry.Message, path.Base(entry.Caller.File), entry.Caller.Line, entry.Caller.Function)
	} else {
		msg = fmt.Sprintf("%s [%s] %s (%s:%d)\n", timestamp, strings.ToUpper(entry.Level.String()), entry.Message, path.Base(entry.Caller.File), entry.Caller.Line)
	}
	return []byte(msg), nil
}

func init() {
	songdownloader.CheckPathExists("./log")
	timeStamp := time.Now().Local().Format("2006-01-02")
	logFile := fmt.Sprintf("./log/%v.log", timeStamp)
	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Error(err)
	}
	output := io.MultiWriter(os.Stdout, file)
	log.SetOutput(output)
	log.SetFormatter(&log.TextFormatter{
		DisableColors:          false,
		FullTimestamp:          true,
		DisableLevelTruncation: true,
		PadLevelText:           true,
	})
	log.SetFormatter(new(LogFormatter))
	switch config["LogLevel"] {
	case "DEBUG":
		log.SetLevel(log.DebugLevel)
	case "WARN":
		log.SetLevel(log.WarnLevel)
	case "FATAL":
		log.SetLevel(log.FatalLevel)
	case "TRACE":
		log.SetLevel(log.TraceLevel)
	default:
		log.SetLevel(log.InfoLevel)
	}
	log.SetReportCaller(true)
}

func init() {
	database := "cache.db"
	if config["Database"] != "" {
		database = config["Database"]
	}
	db, err := gorm.Open(sqlite.Open(database), &gorm.Config{
		Logger:      logger.Default.LogMode(logger.Silent),
		PrepareStmt: true,
	})
	if err != nil {
		log.Fatal("failed to connect database")
	}

	err = db.AutoMigrate(&SongInfo{}, &RequestTimes{}, &DownloadingMusic{})
	if err != nil {
		log.Errorln(err)
	}
	DB = db
}

func init() {
	db := DB.Session(&gorm.Session{AllowGlobalUpdate: true})
	err := db.Delete(&RequestTimes{}).Error
	if err != nil {
		log.Errorln(err)
	}
	err = db.Delete(&DownloadingMusic{}).Error
	if err != nil {
		log.Errorln(err)
	}
	startCron()
}

func getFileSize(url string) (size string, err error) {
	header, err := http.Head(url)
	if err != nil {
		return "null", err
	}
	return fmt.Sprintf("%.2f", float64(header.ContentLength)/1024/1024), nil
}

func startCron() { // 定时清理每分钟请求请求记录
	c := cron.New()
	err := c.AddFunc("0 */1 * * * ?", func() {
		defer func() {
			err := recover()
			if err != nil {
				log.Errorln(err)
			}
		}()
		db := DB.Session(&gorm.Session{AllowGlobalUpdate: true})
		now := time.Now()
		t := now.Add(time.Minute * -1)
		dbResult := db.Delete(&RequestTimes{}, fmt.Sprintf("%d%d", t.Hour(), t.Minute()))
		if dbResult.Error != nil {
			log.Errorln(dbResult.Error)
		}
	})
	if err != nil {
		log.Fatalf("Cron 任务添加失败 : %s", err)
	}
	c.Start()
}
