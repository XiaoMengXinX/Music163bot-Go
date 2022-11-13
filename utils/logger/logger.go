package logger

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/XiaoMengXinX/Music163bot-Go/v3/utils"
	"github.com/natefinch/lumberjack"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm/logger"
)

var Logger = logrus.StandardLogger()
var logWriter io.Writer

type LogFormatter struct{}

func (s *LogFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	timestamp := time.Now().Local().Format("2006/01/02 15:04:05")
	var msg string
	msg = fmt.Sprintf("%s [%s] %s (%s:%d)\n", timestamp, strings.ToUpper(entry.Level.String()), entry.Message, path.Base(entry.Caller.File), entry.Caller.Line)
	return []byte(msg), nil
}

func InitLogger(l *logrus.Logger) {
	l.SetFormatter(&logrus.TextFormatter{
		DisableColors:          false,
		FullTimestamp:          true,
		DisableLevelTruncation: true,
		PadLevelText:           true,
	})
	l.SetFormatter(new(LogFormatter))
	l.SetReportCaller(true)
	l.SetOutput(os.Stdout)
	if logWriter != nil {
		l.SetOutput(logWriter)
	}
}

func SetLoggerOutputPath(l *logrus.Logger, path string) (err error) {
	err = utils.CheckDir(path)
	if err != nil {
		return err
	}
	output := &lumberjack.Logger{
		Filename:   fmt.Sprintf("%s/Music163bot.log", path),
		MaxSize:    10,
		MaxBackups: 300,
		MaxAge:     300,
		Compress:   false,
	}
	logWriter = io.MultiWriter(os.Stdout, output)
	l.SetOutput(logWriter)
	return
}

type LogInterface interface {
	Info(context.Context, string, ...interface{})
	Warn(context.Context, string, ...interface{})
	Error(context.Context, string, ...interface{})
	Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error)
	LogMode(level logger.LogLevel) logger.Interface
}
