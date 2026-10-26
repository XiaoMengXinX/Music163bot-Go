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
	"unicode/utf8"
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

func JoinNames(names []string) string {
	return strings.Join(names, "/")
}

var filenameReplacer = strings.NewReplacer(
	"/", " ", "?", " ", "*", " ", ":", " ", "|", " ", "\\", " ", "<", " ", ">", " ", "\"", " ",
)

// 常见文件系统限制文件名不超过 255 字节
const maxFilenameBytes = 255

// SanitizeFilename 替换非法字符，超长时按字节截断并保留扩展名
func SanitizeFilename(name string) string {
	name = filenameReplacer.Replace(name)
	if len(name) <= maxFilenameBytes {
		return name
	}
	ext := path.Ext(name)
	n := maxFilenameBytes - len(ext)
	for n > 0 && !utf8.RuneStart(name[n]) {
		n--
	}
	return name[:n] + ext
}

func URLBase(rawURL string) string {
	if i := strings.IndexByte(rawURL, '?'); i >= 0 {
		rawURL = rawURL[:i]
	}
	if i := strings.IndexByte(rawURL, '#'); i >= 0 {
		rawURL = rawURL[:i]
	}
	return path.Base(rawURL)
}

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

func ParseProgramID(text string) int {
	messageText := strings.NewReplacer("\n", "", " ", "").Replace(text)
	// 短链跳转后的 id 参数不一定在首位
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

func LinkTestMusic(text string) string {
	return extractInt(reg5.ReplaceAllString(reg4.ReplaceAllString(reg3.ReplaceAllString(reg2.ReplaceAllString(reg1.ReplaceAllString(text, ""), ""), ""), ""), ""))
}

func linkTestProgram(text string) string {
	return extractInt(reg5.ReplaceAllString(reg4.ReplaceAllString(reg3.ReplaceAllString(regP4.ReplaceAllString(regP3.ReplaceAllString(regP2.ReplaceAllString(regP1.ReplaceAllString(text, ""), ""), ""), ""), ""), ""), ""))
}

var shortLinkClient = &http.Client{
	Timeout: 10 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

func ContainsMusicLink(text string) bool {
	return strings.Contains(text, "music.163.com") ||
		strings.Contains(text, "163cn.tv") ||
		strings.Contains(text, "163cn.link")
}

// 仅含域名的消息不作为歌曲链接，避免误取域名中的数字
func ContainsSongLink(text string) bool {
	return strings.Contains(text, "song") ||
		strings.Contains(text, "program") ||
		strings.Contains(text, "dj")
}

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
