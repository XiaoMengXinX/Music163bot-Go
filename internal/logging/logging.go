// Package logging logrus 初始化与 gorm 日志适配，业务日志与 SQL 日志同源
package logging

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm/logger"
)

// Formatter 日志格式：2006/01/02 15:04:05 [LEVEL] message (file:line)
type Formatter struct{}

func (f *Formatter) Format(entry *logrus.Entry) ([]byte, error) {
	ts := time.Now().Local().Format("2006/01/02 15:04:05")
	return []byte(fmt.Sprintf("%s [%s] %s (%s:%d)\n",
		ts, strings.ToUpper(entry.Level.String()), entry.Message,
		filepath.Base(entry.Caller.File), entry.Caller.Line)), nil
}

// dailyFileWriter 按日期切分日志文件：log/<YYYY-MM-DD>.log
type dailyFileWriter struct {
	mu  sync.Mutex
	dir string
	day string
	f   *os.File
}

func (w *dailyFileWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	day := time.Now().Local().Format("2006-01-02")
	if w.f == nil || day != w.day {
		if err := os.MkdirAll(w.dir, 0o755); err != nil {
			return 0, err
		}
		f, err := os.OpenFile(filepath.Join(w.dir, day+".log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return 0, err
		}
		if w.f != nil {
			_ = w.f.Close()
		}
		w.f, w.day = f, day
	}
	return w.f.Write(p)
}

// Setup 输出到 stdout 与按日切分的日志文件，文件打开失败时仅 stdout
func Setup(level, dir string) *logrus.Logger {
	l := logrus.New()
	l.SetFormatter(&Formatter{})
	l.SetReportCaller(true)
	lvl, err := logrus.ParseLevel(level)
	if err != nil {
		lvl = logrus.InfoLevel
	}
	l.SetLevel(lvl)
	if dir != "" {
		l.SetOutput(io.MultiWriter(os.Stdout, &dailyFileWriter{dir: dir}))
	} else {
		l.SetOutput(os.Stdout)
	}
	return l
}

// GormLogger 与业务日志同源的 gorm logger；debug 记全部 SQL，info 起记慢查询
func GormLogger(l *logrus.Logger, level string) logger.Interface {
	var lvl logger.LogLevel
	switch level {
	case "trace", "debug":
		lvl = logger.Info
	case "info":
		lvl = logger.Warn
	default:
		lvl = logger.Error
	}
	return &gormLogger{log: l, level: lvl, slowThreshold: 200 * time.Millisecond}
}

type gormLogger struct {
	log           *logrus.Logger
	level         logger.LogLevel
	slowThreshold time.Duration
}

func (g *gormLogger) LogMode(level logger.LogLevel) logger.Interface {
	c := *g
	c.level = level
	return &c
}

func (g *gormLogger) Info(ctx context.Context, msg string, args ...any) {
	if g.level >= logger.Info {
		g.log.Infof(msg, args...)
	}
}

func (g *gormLogger) Warn(ctx context.Context, msg string, args ...any) {
	if g.level >= logger.Warn {
		g.log.Warnf(msg, args...)
	}
}

func (g *gormLogger) Error(ctx context.Context, msg string, args ...any) {
	if g.level >= logger.Error {
		g.log.Errorf(msg, args...)
	}
}

func (g *gormLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if g.level <= logger.Silent {
		return
	}
	elapsed := time.Since(begin)
	switch {
	case err != nil && g.level >= logger.Error && !errors.Is(err, logger.ErrRecordNotFound):
		sql, rows := fc()
		g.log.Errorf("SQL 错误: %v [%.3fms] [rows:%v] %s", err, float64(elapsed.Nanoseconds())/1e6, rowsFormat(rows), sql)
	case elapsed > g.slowThreshold && g.slowThreshold != 0 && g.level >= logger.Warn:
		sql, rows := fc()
		g.log.Warnf("慢 SQL >= %v [%.3fms] [rows:%v] %s", g.slowThreshold, float64(elapsed.Nanoseconds())/1e6, rowsFormat(rows), sql)
	case g.level >= logger.Info:
		sql, rows := fc()
		g.log.Infof("SQL [%.3fms] [rows:%v] %s", float64(elapsed.Nanoseconds())/1e6, rowsFormat(rows), sql)
	}
}

func rowsFormat(rows int64) any {
	if rows == -1 {
		return "-"
	}
	return rows
}
