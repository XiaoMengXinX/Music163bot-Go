// Package render 界面文案，统一 HTML parse mode，动态内容经转义
package render

import (
	"fmt"
	"html"
	"strings"
)

const (
	InputKeyword     = "请输入搜索关键词"
	InputIDorKeyword = "请输入歌曲ID或歌曲关键词"
	InputIDorLink    = "请输入歌曲ID或歌曲分享链接"
	InputContent     = "请输入歌曲关键词/歌曲分享链接/歌曲ID"
	Searching        = "搜索中..."
	NoResults        = "未找到结果"
	NoCache          = "歌曲未缓存"
	TapToDownload    = "点击上方按钮缓存歌曲"
	TapMeToDown      = "点我缓存歌曲"
	HitCache         = "命中缓存, 正在发送中..."
	SendMeTo         = "Send me to..."
	GetLrcFailed     = "获取歌词失败, 歌曲可能不存在或为纯音乐"
	GetURLFailed     = "获取歌曲下载链接失败"
	FetchInfo        = "正在获取歌曲信息..."
	FetchInfoFailed  = "获取歌曲信息失败"
	WaitForDown      = "等待下载中..."
	Downloading      = "下载中..."
	Redownloading    = "下载失败，尝试重新下载中..."
	Uploading        = "下载完成, 发送中..."
	Md5VerFailed     = "MD5校验失败"
	RetryLater       = "请稍后重试"
	Reloading        = "重新加载中"
	CallbackText     = "Success"
	FetchingLyric    = "正在获取歌词中"
	DownloadTimeout  = "下载超时"

	ReplyVoiceFirst     = "请先发送一条包含音乐片段的语音，再回复该语音发送 /recognize。"
	GuestUsage          = "回复一条网易云音乐分享链接或包含音乐片段的语音，并在回复中 @我，即可发送或识别歌曲。"
	GuestNoCache        = "歌曲未缓存，请点击下方按钮在 Bot 中获取"
	Recognizing         = "正在识别音乐，请稍候..."
	RecognizeNoResult   = "暂未识别到歌曲\n请尝试录制更清晰、稍长一些的音乐片段，再试一次。"
	RecognizeFailed     = "音乐识别暂时失败，请稍后重试。"
	RecognizeReadFailed = "暂时无法读取这条语音\n请稍后重试，或重新发送语音。"
	RecognizeSuccess    = "已识别到歌曲"
	InlineHelpHint      = "输入 help 获取帮助"
	InlineHelpStep1     = "1.粘贴音乐分享URL或输入MusicID"
	InlineHelpStep2     = "2.输入 search+关键词 搜索歌曲"
	InlineInputKeyword  = "请输入关键词"
	BotName             = "Music163bot-Go v3"

	LoginBusy        = "已有扫码登录会话进行中"
	LoginKeyFailed   = "获取二维码失败，请稍后重试"
	LoginExpired     = "二维码已过期，请重新发送 /login"
	LoginCheckFailed = "查询扫码状态失败，请稍后重试"
	LoginCancelled   = "已取消登录"
	CancelLogin      = "取消登录"

	LoginStartButton      = "扫码登录"
	LoginTimeoutStopped   = "登录超时，bot 已停止"
	LoginCancelledStopped = "已取消登录，bot 已停止"
	LoginStartupDone      = "Cookie 已生效"
	LoginSessionOver      = "登录会话已结束，可发送 /login"
	LoginNoSession        = "当前没有进行中的登录会话，可发送 /login"
	LoginQRSent           = "已发送登录二维码"

	InputChatID          = "缓存群 ID 须为数字，也可以在目标群内直接发送 /setcachechat"
	CacheChatTest        = "已设为缓存群，Guest Mode 点歌时会先把未缓存的歌曲上传到这里"
	CacheChatUnreachable = "无法向该聊天发送消息，请确认 bot 已加入并可以发言"
	CacheChatDisabled    = "已关闭缓存群"

	CookieExpired          = "Cookie 过期，请联系 bot 管理员"
	CookieExpiredAdminWarn = "Cookie 已失效，新歌下载已暂停，请发送 /login 重新登录"
	CookieResumedAdmin     = "Cookie 已生效，新歌下载已恢复"
)

func LoginStartupPrompt(expired bool, remainSec int) string {
	head := "Cookie 未配置，需登录"
	if expired {
		head = "Cookie 已失效，需重新登录"
	}
	return fmt.Sprintf("%s\n点击下方按钮扫码登录，或扫描终端内二维码\n请在 %d 秒内操作", head, remainSec)
}

func LoginStartupTerminal(expired bool, remainSec int) string {
	head := "Cookie 未配置，需登录"
	if expired {
		head = "Cookie 已失效，需重新登录"
	}
	return fmt.Sprintf("%s\n请使用网易云音乐 App 扫描下方二维码\n请在 %d 秒内操作", head, remainSec)
}

func LoginQRCaption(remainSec int) string {
	return fmt.Sprintf("请使用网易云音乐 App 扫码\n剩余有效期 %d 秒", remainSec)
}

func LoginScanned(nickname string) string {
	if nickname != "" {
		return "已扫码，等待确认: " + Escape(nickname)
	}
	return "已扫码，等待确认"
}

func LoginOK(nickname string) string {
	if nickname != "" {
		return "扫码登录成功: " + Escape(nickname)
	}
	return "扫码登录成功"
}

func Escape(s string) string { return html.EscapeString(s) }

func MusicStatus(name, album, ext string, sizeMB float64, suffix string) string {
	return fmt.Sprintf("%s\n专辑: %s\n%s %.2fMB\n%s", Escape(name), Escape(album), Escape(ext), sizeMB, suffix)
}

func DownloadStatus(speed string, writtenMB, totalMB float64, percent int64) string {
	return fmt.Sprintf(" %s\n%.2fMB/%.2fMB %d%%", Escape(speed), writtenMB, totalMB, percent)
}

func DownloadFailed(errText string) string {
	if strings.Contains(errText, Md5VerFailed) || strings.Contains(errText, DownloadTimeout) {
		return Escape(errText)
	}
	return "下载/发送失败\n" + Escape(errText)
}

func SongInfo(name, artists, album string) string {
	return fmt.Sprintf("「%s」- %s\n专辑: %s", Escape(name), Escape(artists), Escape(album))
}

func RecognizeResult(name, artists, album string) string {
	return "🎵 识别成功\n" + SongInfo(name, artists, album)
}

func GuestMusicResult(name, artists, album string) string {
	return SongInfo(name, artists, album) + "\n" + GuestNoCache
}

func MusicCaption(name, artists, album, ext string, sizeMB, kbps float64, botName string) string {
	return fmt.Sprintf("%s\n#网易云音乐 #%s %.2fMB %.2fkbps\nvia @%s",
		SongInfo(name, artists, album), Escape(ext), sizeMB, kbps, Escape(botName))
}

func About(runtimeVer, versionName, commitSHA, buildTime, buildArch string) string {
	return fmt.Sprintf(`<b>Music163bot-Go v3</b>
Github: https://github.com/XiaoMengXinX/Music163bot-Go

[编译环境] %s
[编译版本] %s
[编译哈希] %s
[编译日期] %s
[运行环境] %s`,
		Escape(runtimeVer), Escape(versionName), Escape(commitSHA), Escape(buildTime), Escape(buildArch))
}

func StatusInfo(chatTitle, chatUsername string, chatID, userID, total, chatCount, userCount int64) string {
	var chatInfo string
	switch {
	case chatUsername != "" && chatTitle == "":
		chatInfo = fmt.Sprintf(`<a href="tg://user?id=%d">%s</a>`, chatID, Escape(chatUsername))
	case chatUsername != "":
		chatInfo = fmt.Sprintf(`<a href="https://t.me/%s">%s</a>`, Escape(chatUsername), Escape(chatTitle))
	default:
		chatInfo = Escape(chatTitle)
	}
	return fmt.Sprintf(`<b>[统计信息]</b>
数据库中总缓存歌曲数量: %d
当前对话 [%s] 缓存歌曲数量: %d
当前用户 [<a href="tg://user?id=%d">%d</a>] 缓存歌曲数量: %d`,
		total, chatInfo, chatCount, userID, userID, userCount)
}

func CacheChatSet(chatID int64) string {
	return fmt.Sprintf("缓存群已设置为 <code>%d</code>", chatID)
}

func RmcacheReport(name string) string {
	return fmt.Sprintf("清除 [%s] 缓存成功", Escape(name))
}

func SongLink(musicID int) string {
	return fmt.Sprintf("https://music.163.com/song/%d", musicID)
}
