package bot

import (
	"github.com/XiaoMengXinX/Music163bot-Go/v2/util"
	"github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

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

	err = db.AutoMigrate(&util.SongInfo{})
	if err != nil {
		logrus.Errorln(err)
	}
	DB = db
}
