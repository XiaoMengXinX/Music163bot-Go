package bot

import (
	"github.com/XiaoMengXinX/Music163Api-Go/utils"
	"gorm.io/gorm"
	"regexp"
)

// DB 全局数据库入口
var DB *gorm.DB
var config map[string]string
var data utils.RequestData
var botName string
var cacheDir = "./cache"
var maxRedownTimes int

var (
	reg1 = regexp.MustCompile(`(.*)song\?id=`)
	reg2 = regexp.MustCompile("(.*)song/")
	reg5 = regexp.MustCompile("/(.*)")
	reg4 = regexp.MustCompile("&(.*)")
	reg3 = regexp.MustCompile(`\?(.*)`)
)

var (
	aboutText = `*Music163bot-Go v2*
Github: https://github.com/XiaoMengXinX/Music163bot-Go

\[编译环境] %s
\[编译版本] %s
\[编译哈希] %s
\[编译日期] %s
\[运行环境] %s
\[运行版本] %s`
	musicInfo = `「%s」- %s
专辑: %s
#网易云音乐 #%s %.2fkpbs
via @%s`
	musicInfoMsg = `%s
专辑: %s
%s %sMB
`
	rmcacheReport = `清除 [%s] 缓存成功`
	inputKeyword  = "请输入搜索关键词"
	searching     = `搜索中...`
	noResults     = `未找到结果`
	noCache       = `歌曲未缓存`
	tapToDownload = `点击上方按钮缓存歌曲`
	tapMeToDown   = `点我缓存歌曲`
	hitCache      = `命中缓存，正在发送中...`
	sendMeTo      = `Send me to...`
	uploadFailed  = `下载/发送失败
%v`
	getUrlFailed     = `获取歌曲下载链接失败`
	fetchInfo        = `正在获取歌曲信息...`
	fetchInfoFailed  = `获取歌曲信息失败`
	waitForDown      = `等待下载中...`
	downloading      = `下载中...`
	uploading        = `下载完成，发送中...`
	md5VerFailed     = "MD5校验失败"
	redownlpading    = "尝试重新下载中 (%d/%d)"
	tryToRedown      = "请稍后重试"
	updateBinVersion = `请更新主程序文件版本！
详见: https://github.com/XiaoMengXinX/Music163bot-Go/releases`
)
