package bot

import (
	"github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
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

func initDB(config map[string]string) {
	database := "cache.db"
	if config["Database"] != "" {
		database = config["Database"]
	}
	db, err := gorm.Open(sqlite.Open(database), &gorm.Config{
		Logger:      logger.Default.LogMode(logger.Silent),
		PrepareStmt: true,
	})
	if err != nil {
		logrus.Fatal("Failed to connect database : ", err)
	}

	err = db.Table("song_infos").AutoMigrate(&SongInfo{})
	if err != nil {
		logrus.Errorln(err)
	}
	DB = db.Table("song_infos")
}
