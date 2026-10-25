// Package textx 链接/ID 解析、短链跟随、文件名清洗
package textx

import (
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	reg1   = regexp.MustCompile(`(.*)song\?id=`)
	reg2   = regexp.MustCompile(`(.*)song/`)
	regP1  = regexp.MustCompile(`(.*)program\?id=`)
	regP2  = regexp.MustCompile(`(.*)program/`)
	regP3  = regexp.MustCompile(`(.*)dj\?id=`)
	regP4  = regexp.MustCompile(`(.*)dj/`)
	reg3   = regexp.MustCompile(`\?(.*)`)
	reg4   = regexp.MustCompile("&(.*)")
	reg5   = regexp.MustCompile("/(.*)")
	regInt = regexp.MustCompile(`\d+`)
	regURL = regexp.MustCompile("(http|https)://[\\w\\-_]+(\\.[\\w\\-_]+)+([\\w\\-.,@?^=%&:/~+#]*[\\w\\-@?^=%&/~+#])?")
)

// JoinNames 用 "/" 拼接歌手名
func JoinNames(names []string) string {
	return strings.Join(names, "/")
}

// SanitizeFilename 替换文件名中的非法字符为空格
var filenameReplacer = strings.NewReplacer(
	"/", " ", "?", " ", "*", " ", ":", " ", "|", " ", "\\", " ", "<", " ", ">", " ", "\"", " ",
)

func SanitizeFilename(name string) string {
	return filenameReplacer.Replace(name)
}

// URLBase 返回 URL 去掉查询参数后的最后一段路径，用作下载文件名
func URLBase(rawURL string) string {
	if i := strings.IndexByte(rawURL, '?'); i >= 0 {
		rawURL = rawURL[:i]
	}
	if i := strings.IndexByte(rawURL, '#'); i >= 0 {
		rawURL = rawURL[:i]
	}
	return path.Base(rawURL)
}

// ParseMusicID 从文本解析网易云歌曲 ID
func ParseMusicID(text string) int {
	messageText := strings.NewReplacer("\n", "", " ", "").Replace(text)
	if musicURL := regURL.FindStringSubmatch(messageText); len(musicURL) != 0 {
		if strings.Contains(musicURL[0], "song") {
			ur, _ := url.Parse(musicURL[0])
			if musicID, _ := strconv.Atoi(ur.Query().Get("id")); musicID != 0 {
				return musicID
			}
		}
	}
	musicID, _ := strconv.Atoi(LinkTestMusic(messageText))
	return musicID
}

// ParseProgramID 从文本解析电台节目 ID
func ParseProgramID(text string) int {
	messageText := strings.NewReplacer("\n", "", " ", "").Replace(text)
	// 短链落点的参数顺序不固定（如 program?app_version=...&id=...），按 Query 取
	if programURL := regURL.FindStringSubmatch(messageText); len(programURL) != 0 {
		if strings.Contains(programURL[0], "program") {
			ur, _ := url.Parse(programURL[0])
			if id, _ := strconv.Atoi(ur.Query().Get("id")); id != 0 {
				return id
			}
		}
	}
	programID, _ := strconv.Atoi(linkTestProgram(messageText))
	return programID
}

func extractInt(text string) string {
	matchArr := regInt.FindStringSubmatch(text)
	if len(matchArr) == 0 {
		return ""
	}
	return matchArr[0]
}

// LinkTestMusic 清理文本中的链接前缀后提取数字（inline 查询等直接调用）
func LinkTestMusic(text string) string {
	return extractInt(reg5.ReplaceAllString(reg4.ReplaceAllString(reg3.ReplaceAllString(reg2.ReplaceAllString(reg1.ReplaceAllString(text, ""), ""), ""), ""), ""))
}

func linkTestProgram(text string) string {
	return extractInt(reg5.ReplaceAllString(reg4.ReplaceAllString(reg3.ReplaceAllString(regP4.ReplaceAllString(regP3.ReplaceAllString(regP2.ReplaceAllString(regP1.ReplaceAllString(text, ""), ""), ""), ""), ""), ""), ""))
}

// shortLinkClient 只跟随一跳重定向，带超时
var shortLinkClient = &http.Client{
	Timeout: 10 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// ContainsMusicLink 判断消息文本是否包含网易云分享相关域名
func ContainsMusicLink(text string) bool {
	return strings.Contains(text, "music.163.com") ||
		strings.Contains(text, "163cn.tv") ||
		strings.Contains(text, "163cn.link")
}

// ContainsSongLink 判断文本是否包含歌曲/电台节目分享链接（song/program/dj 关键字）。
// 仅提及域名（如 music.163.com）或其它页面链接（歌单/MV 等）不算，
// 避免嗅探把域名里的数字误当成歌曲 ID。
func ContainsSongLink(text string) bool {
	return strings.Contains(text, "song") ||
		strings.Contains(text, "program") ||
		strings.Contains(text, "dj")
}

// ContainsShortLink 判断是否包含 163cn 短链（.tv 与 .link）
func ContainsShortLink(text string) bool {
	return strings.Contains(text, "163cn.tv") || strings.Contains(text, "163cn.link")
}

// ResolveShortLink 解析 163cn 短链返回重定向地址，无短链或失败时原样返回（校验 host）
func ResolveShortLink(text string) string {
	messageText := strings.NewReplacer("\n", "", " ", "").Replace(text)
	musicURL := regURL.FindStringSubmatch(messageText)
	if len(musicURL) == 0 {
		return text
	}
	u, err := url.Parse(musicURL[0])
	if err != nil || (u.Host != "163cn.tv" && u.Host != "163cn.link") {
		return text
	}
	resp, err := shortLinkClient.Get(musicURL[0])
	if err != nil {
		return text
	}
	defer resp.Body.Close()
	return resp.Header.Get("Location")
}
