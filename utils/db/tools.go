package db

import "gorm.io/gorm"

func FindMusicCache(musicID int) (songInfo SongInfo, err error) {
	musicDB := MusicDB.Session(&gorm.Session{})
	err = musicDB.Where("music_id = ?", musicID).First(&songInfo).Error
	return
}

func DeleteMusicCache(musicID int) (err error) {
	musicDB := MusicDB.Session(&gorm.Session{})
	err = musicDB.Where("music_id = ?", musicID).Delete(&SongInfo{}).Error
	return
}

func AddMusicCache(songInfo SongInfo) (err error) {
	musicDB := MusicDB.Session(&gorm.Session{})
	err = musicDB.Create(&songInfo).Error
	return
}

func UpdateMusicCache(songInfo SongInfo) (err error) {
	musicDB := MusicDB.Session(&gorm.Session{})
	err = musicDB.Where("music_id = ?", songInfo.MusicID).Updates(&songInfo).Error
	return
}
