package db

import (
	"github.com/XiaoMengXinX/Music163bot-Go/v3/config"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

var MusicDB *gorm.DB

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

func InitDB(config config.SqliteConfig) (err error) {
	if config.Path == "" {
		config.Path = "cache.db"
	}
	db, err := gorm.Open(sqlite.Open(config.Path), &gorm.Config{
		Logger:      NewLogger(ParseLevel(config.LogLevel)),
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
	return err
}
