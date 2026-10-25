package netease

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/XiaoMengXinX/Music163Api-Go/api"
	"github.com/XiaoMengXinX/Music163Api-Go/utils"
)

// QRLoginStatus 扫码登录状态，对应 CheckQrLogin 返回码
type QRLoginStatus int

const (
	QRExpired QRLoginStatus = 800 // 二维码已过期
	QRWaiting QRLoginStatus = 801 // 等待扫码
	QRScanned QRLoginStatus = 802 // 已扫码，等待确认
	QRSuccess QRLoginStatus = 803 // 登录成功
)

// SetMUSICU 替换 MUSIC_U cookie，立即生效
func (c *Client) SetMUSICU(musicU string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data = utils.RequestData{
		Cookies: []*http.Cookie{{Name: "MUSIC_U", Value: musicU}},
	}
}

// requestData 返回当前请求数据的快照（切片不可变，读写安全）
func (c *Client) requestData() utils.RequestData {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.data
}

// CreateQRLoginKey 申请扫码登录用的 unikey（匿名请求）
func (c *Client) CreateQRLoginKey() (string, error) {
	result, err := api.GetQrUnikey(utils.RequestData{})
	if err != nil {
		return "", err
	}
	if result.Unikey == "" {
		return "", fmt.Errorf("获取扫码登录 key 失败: %s", result.RawJson)
	}
	return result.Unikey, nil
}

// QRLoginContent 拼接 unikey 对应的二维码内容
func QRLoginContent(unikey string) string {
	return "https://music.163.com/login?codekey=" + unikey
}

// CheckQRLogin 查询扫码状态（匿名请求），成功时同时返回新的 MUSIC_U
func (c *Client) CheckQRLogin(key string) (status QRLoginStatus, nickname, musicU string, err error) {
	result, header, err := api.CheckQrLogin(utils.RequestData{}, key)
	if err != nil {
		return 0, "", "", err
	}
	switch result.Code {
	case int(QRExpired):
		return QRExpired, "", "", nil
	case int(QRWaiting):
		return QRWaiting, "", "", nil
	case int(QRScanned):
		return QRScanned, result.Nickname, "", nil
	case int(QRSuccess):
		musicU := extractMUSICU(header)
		if musicU == "" {
			return 0, "", "", fmt.Errorf("登录成功但未从响应中解析到 MUSIC_U")
		}
		return QRSuccess, result.Nickname, musicU, nil
	}
	return 0, "", "", fmt.Errorf("扫码登录返回未知状态 %d: %s", result.Code, result.Message)
}

// extractMUSICU 从 Set-Cookie 响应头中提取 MUSIC_U
func extractMUSICU(header http.Header) string {
	for _, cookie := range header.Values("Set-Cookie") {
		first := strings.SplitN(cookie, ";", 2)[0]
		if name, value, ok := strings.Cut(strings.TrimSpace(first), "="); ok && name == "MUSIC_U" {
			return value
		}
	}
	return ""
}

// LoginStatusValid 检查 MUSIC_U 是否处于登录状态；网络错误时返回 err
func (c *Client) LoginStatusValid() (bool, error) {
	result, err := api.GetLoginStatus(c.requestData())
	if err != nil {
		return false, err
	}
	return result.Code == 200 && result.Profile.UserId != 0, nil
}
