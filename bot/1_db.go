package bot

import (
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

// UserInfo 用户信息
type UserInfo struct {
	UserID        int64
	IsAprilFooled bool
}

const (
	// GlobalSetting 全局设置
	GlobalSetting = iota + 1
	// ChatSetting 对话设置
	ChatSetting
	// UserSetting 用户设置
	UserSetting
)

// Settings bot设置
type Settings struct {
	Type       int
	ChatID     int64
	SourceInfo bool
	ShareKey   bool
}

func initDB(config map[string]string) (err error) {
	database := "cache.db"
	if config["Database"] != "" {
		database = config["Database"]
	}
	db, err := gorm.Open(sqlite.Open(database), &gorm.Config{
		Logger:      logger.Default.LogMode(logger.Silent),
		PrepareStmt: true,
	})
	if err != nil {
		return err
	}

	err = db.Table("song_infos").AutoMigrate(&SongInfo{})
	if err != nil {
		return err
	}
	MusicDB = db.Table("song_infos")

	err = db.Table("bot_settings").AutoMigrate(&Settings{})
	if err != nil {
		return err
	}
	SettingDB = db.Table("bot_settings")

	err = db.Table("user_infos").AutoMigrate(&UserInfo{})
	if err != nil {
		return err
	}
	UserDB = db.Table("user_infos")

	return err
}
