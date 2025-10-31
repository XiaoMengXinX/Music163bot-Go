package bot

import (
	"fmt"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// SongInfo 歌曲信息
type SongInfo struct {
	gorm.Model
	MusicID      int
	SongName     string
	SongArtists  string
	SongAlbum    string
	FileExt      string
	MusicSize    int
	PicSize      int
	EmbPicSize   int
	BitRate      int
	Duration     int
	FileID       string
	ThumbFileID  string
	FromUserID   int64
	FromUserName string
	FromChatID   int64
	FromChatName string
}

// UserStats 用户统计信息
type UserStats struct {
	gorm.Model
	UserID        int64 `gorm:"uniqueIndex"`
	UserName      string
	TotalDownload int    // 累计下载数
	TodayDownload int    // 今日下载数
	LastResetDate string // 最后重置日期 (格式: YYYY-MM-DD)
}

func initDB(config map[string]string) (err error) {
	database := "cache.db"
	if config["Database"] != "" {
		database = config["Database"]
	}
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("%s?_pragma=busy_timeout(5000)", database)), &gorm.Config{
		Logger:      NewLogger(logger.Silent),
		PrepareStmt: true,
	})
	if err != nil {
		return err
	}
	err = db.Table("song_infos").AutoMigrate(&SongInfo{})
	if err != nil {
		return err
	}
	err = db.Table("user_stats").AutoMigrate(&UserStats{})
	if err != nil {
		return err
	}
	MusicDB = db.Table("song_infos")
	UserStatsDB = db.Table("user_stats")
	return err
}
