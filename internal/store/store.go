// Package store 管理 SQLite 歌曲缓存
package store

import (
	"context"
	"embed"
	"fmt"
	"io/fs"

	"github.com/glebarez/sqlite"
	"github.com/pressly/goose/v3"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// SongInfo 歌曲信息，字段与 v2 的 song_infos 表一致
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

func (SongInfo) TableName() string { return "song_infos" }

type Store struct {
	db *gorm.DB
}

func Open(path string, gormLogger logger.Interface) (*Store, error) {
	db, err := gorm.Open(sqlite.Open(path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)"), &gorm.Config{
		Logger:      gormLogger,
		PrepareStmt: true,
	})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	// goose 只扫描 FS 根目录
	migrations, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		return nil, err
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, sqlDB, migrations)
	if err != nil {
		return nil, err
	}
	if _, err := provider.Up(context.Background()); err != nil {
		return nil, fmt.Errorf("数据库迁移失败: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// GetByMusicID 按歌曲 ID 查询缓存（自动过滤软删除行）
func (s *Store) GetByMusicID(musicID int) (SongInfo, bool) {
	var songInfo SongInfo
	err := s.db.Where("music_id = ?", musicID).First(&songInfo).Error
	if err != nil {
		return songInfo, false
	}
	return songInfo, true
}

// 按 music_id 更新缓存，同时清除软删除标记
func (s *Store) Save(songInfo *SongInfo) error {
	return s.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "music_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"updated_at", "deleted_at", "song_name", "song_artists", "song_album",
			"file_ext", "music_size", "pic_size", "emb_pic_size", "bit_rate", "duration",
			"file_id", "thumb_file_id", "from_user_id", "from_user_name", "from_chat_id", "from_chat_name",
		}),
	}).Create(songInfo).Error
}

func (s *Store) DeleteByMusicID(musicID int) (name string, ok bool, err error) {
	songInfo, found := s.GetByMusicID(musicID)
	if !found {
		return "", false, nil
	}
	if err := s.db.Where("music_id = ?", musicID).Delete(&SongInfo{}).Error; err != nil {
		return "", false, err
	}
	return songInfo.SongName, true, nil
}

func (s *Store) Counts(fromUserID, fromChatID int64) (total, byUser, byChat int64) {
	s.db.Model(&SongInfo{}).Count(&total)
	s.db.Model(&SongInfo{}).Where("from_user_id = ?", fromUserID).Count(&byUser)
	s.db.Model(&SongInfo{}).Where("from_chat_id = ?", fromChatID).Count(&byChat)
	return total, byUser, byChat
}
