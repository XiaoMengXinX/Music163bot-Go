package bot

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm/logger"
)

// Colors
const (
	Reset       = "\033[0m"
	Red         = "\033[31m"
	Green       = "\033[32m"
	Yellow      = "\033[33m"
	Blue        = "\033[34m"
	Magenta     = "\033[35m"
	Cyan        = "\033[36m"
	White       = "\033[37m"
	BlueBold    = "\033[34;1m"
	MagentaBold = "\033[35;1m"
	RedBold     = "\033[31;1m"
	YellowBold  = "\033[33;1m"
)

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
}

type LogInterface interface {
	Info(context.Context, string, ...interface{})
	Warn(context.Context, string, ...interface{})
	Error(context.Context, string, ...interface{})
	Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error)
	LogMode(level logger.LogLevel) logger.Interface
}

type dbLogger struct {
	*logrus.Logger
	SlowThreshold                       time.Duration
	traceStr, traceErrStr, traceWarnStr string
}

func NewLogger(level logger.LogLevel) logger.Interface {
	traceStr := Yellow + "[%.3fms] " + BlueBold + "[rows:%v]" + Reset + " %s"
	traceWarnStr := Green + "%s " + Yellow + "%s\n" + Reset + RedBold + "[%.3fms] " + Yellow + "[rows:%v]" + Magenta + " %s" + Reset
	traceErrStr := RedBold + "%s " + MagentaBold + "%s\n" + Reset + Yellow + "[%.3fms] " + BlueBold + "[rows:%v]" + Reset + " %s"
	l := &dbLogger{
		Logger:        logrus.New(),
		SlowThreshold: 200 * time.Millisecond,
		traceStr:      traceStr,
		traceWarnStr:  traceWarnStr,
		traceErrStr:   traceErrStr,
	}
	InitLogger(l.Logger)
	l.LogMode(level)
	return l
}

func (l *dbLogger) Info(ctx context.Context, msg string, data ...interface{}) {
	if int(l.GetLevel()) >= int(logger.Info) {
		l.Infof(msg, data...)
	}
}

func (l *dbLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	if int(l.GetLevel()) >= int(logger.Warn) {
		l.Warnf(msg, data...)
	}
}

func (l *dbLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	if int(l.GetLevel()) >= int(logger.Error) {
		l.Errorf(msg, data...)
	}
}

func (l *dbLogger) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	if int(l.Level) <= int(logger.Silent) {
		return
	}

	elapsed := time.Since(begin)
	switch {
	case err != nil && int(l.Level) >= int(logger.Error) && (!errors.Is(err, logger.ErrRecordNotFound)):
		sql, rows := fc()
		if rows == -1 {
			l.Printf(l.traceErrStr, err, float64(elapsed.Nanoseconds())/1e6, "-", sql)
		} else {
			l.Printf(l.traceErrStr, err, float64(elapsed.Nanoseconds())/1e6, rows, sql)
		}
	case elapsed > l.SlowThreshold && l.SlowThreshold != 0 && int(l.Level) >= int(logger.Warn):
		sql, rows := fc()
		slowLog := fmt.Sprintf("SLOW SQL >= %v", l.SlowThreshold)
		if rows == -1 {
			l.Printf(l.traceWarnStr, slowLog, float64(elapsed.Nanoseconds())/1e6, "-", sql)
		} else {
			l.Printf(l.traceWarnStr, slowLog, float64(elapsed.Nanoseconds())/1e6, rows, sql)
		}
	case int(l.Level) == int(logger.Info):
		sql, rows := fc()
		if rows == -1 {
			l.Printf(l.traceStr, float64(elapsed.Nanoseconds())/1e6, "-", sql)
		} else {
			l.Printf(l.traceStr, float64(elapsed.Nanoseconds())/1e6, rows, sql)
		}
	}
	return
}

func (l *dbLogger) LogMode(level logger.LogLevel) logger.Interface {
	switch level {
	case logger.Silent:
		l.SetLevel(logrus.FatalLevel)
	case logger.Error:
		l.SetLevel(logrus.ErrorLevel)
	case logger.Warn:
		l.SetLevel(logrus.WarnLevel)
	case logger.Info:
		l.SetLevel(logrus.InfoLevel)
	}
	return l
}
