module github.com/XiaoMengXinX/Music163bot-Go

go 1.16

require (
	github.com/EDDYCJY/gsema v0.0.0-20190120044130-7f6a61b75219
	github.com/XiaoMengXinX/NeteaseCloudApi-Go v0.0.0
	github.com/XiaoMengXinX/NeteaseCloudApi-Go/tools/SongDownloader/utils v0.0.0
	github.com/go-telegram-bot-api/telegram-bot-api/v5 v5.0.0-rc1.0.20210627191509-66dc9e824616
	github.com/nfnt/resize v0.0.0-20180221191011-83c6a9932646
	github.com/robfig/cron v1.2.0
	github.com/sirupsen/logrus v1.8.1
	gorm.io/driver/sqlite v1.1.4
	gorm.io/gorm v1.21.11
)

replace github.com/XiaoMengXinX/NeteaseCloudApi-Go => ./NeteaseCloudApi-Go

replace github.com/XiaoMengXinX/NeteaseCloudApi-Go/tools/SongDownloader => ./NeteaseCloudApi-Go/tools/SongDownloader
