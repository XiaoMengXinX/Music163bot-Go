package bot

import (
	"regexp"
	"strings"

	"github.com/XiaoMengXinX/Music163Api-Go/utils"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"gorm.io/gorm"
)

// MusicDB 音乐缓存数据库入口
var MusicDB *gorm.DB

// config 配置文件数据
var config map[string]string

// data 网易云 cookie
var data utils.RequestData

var bot *tgbotapi.BotAPI
var botAdmin []int
var botAdminStr []string
var botName string
var cacheDir = "./cache"
var botAPI = "https://api.telegram.org"

// maxRetryTimes 最大重试次数, downloaderTimeout 下载超时时间
var maxRetryTimes, downloaderTimeout int

var (
	reg1   = regexp.MustCompile(`(.*)song\?id=`)
	reg2   = regexp.MustCompile("(.*)song/")
	regP1  = regexp.MustCompile(`(.*)program\?id=`)
	regP2  = regexp.MustCompile("(.*)program/")
	regP3  = regexp.MustCompile(`(.*)dj\?id=`)
	regP4  = regexp.MustCompile("(.*)dj/")
	reg5   = regexp.MustCompile("/(.*)")
	reg4   = regexp.MustCompile("&(.*)")
	reg3   = regexp.MustCompile(`\?(.*)`)
	regInt = regexp.MustCompile(`\d+`)
	regUrl = regexp.MustCompile("(http|https)://[\\w\\-_]+(\\.[\\w\\-_]+)+([\\w\\-.,@?^=%&:/~+#]*[\\w\\-@?^=%&/~+#])?")
)

var mdV2Replacer = strings.NewReplacer(
	"_", "\\_", "*", "\\*", "[", "\\[", "]", "\\]", "(",
	"\\(", ")", "\\)", "~", "\\~", "`", "\\`", ">", "\\>",
	"#", "\\#", "+", "\\+", "-", "\\-", "=", "\\=", "|",
	"\\|", "{", "\\{", "}", "\\}", ".", "\\.", "!", "\\!",
)

var (
	aboutText = `*Music163bot-Go v2*
Github: https://github.com/XiaoMengXinX/Music163bot-Go

\[编译环境] %s
\[编译版本] %s
\[编译哈希] %s
\[编译日期] %s
\[运行环境] %s`
	musicInfo = `「%s」- %s
专辑: %s
#网易云音乐 #%s %.2fMB %.2fkpbs
via @%s`
	musicInfoMsg = `%s
专辑: %s
%s %.2fMB
`
	uploadFailed = `下载/发送失败
%v`
	statusInfo = `*\[统计信息\]*
数据库中总缓存歌曲数量: %d
当前对话 \[%s\] 缓存歌曲数量: %d
当前用户 \[[%d](tg://user?id=%d)\] 缓存歌曲数量: %d
`
	rmcacheReport    = `清除 [%s] 缓存成功`
	inputKeyword     = "请输入搜索关键词"
	inputIDorKeyword = "请输入歌曲ID或歌曲关键词"
	inputContent     = "请输入歌曲关键词/歌曲分享链接/歌曲ID"
	searching        = `搜索中...`
	noResults        = `未找到结果`
	noCache          = `歌曲未缓存`
	tapToDownload    = `点击上方按钮缓存歌曲`
	tapMeToDown      = `点我缓存歌曲`
	hitCache         = `命中缓存, 正在发送中...`
	sendMeTo         = `Send me to...`
	getLrcFailed     = `获取歌词失败, 歌曲可能不存在或为纯音乐`
	getUrlFailed     = `获取歌曲下载链接失败`
	fetchInfo        = `正在获取歌曲信息...`
	fetchInfoFailed  = `获取歌曲信息失败`
	waitForDown      = `等待下载中...`
	downloading      = `下载中...`
	downloadStatus   = " %s\n%.2fMB/%.2fMB %d%%"
	redownloading    = `下载失败，尝试重新下载中...`
	uploading        = `下载完成, 发送中...`
	md5VerFailed     = "MD5校验失败"
	reTrying         = "尝试重新下载中 (%d/%d)"
	retryLater       = "请稍后重试"

	reloading    = "重新加载中"
	callbackText = "Success"

	fetchingLyric   = "正在获取歌词中"
	downloadTimeout = `下载超时`
)
