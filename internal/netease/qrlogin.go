package netease

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/XiaoMengXinX/Music163Api-Go/api"
	"github.com/XiaoMengXinX/Music163Api-Go/utils"
)

type QRLoginStatus int

const (
	QRExpired QRLoginStatus = 800 // 二维码已过期
	QRWaiting QRLoginStatus = 801 // 等待扫码
	QRScanned QRLoginStatus = 802 // 已扫码，等待确认
	QRSuccess QRLoginStatus = 803 // 登录成功
)

func (c *Client) SetMUSICU(musicU string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data = utils.RequestData{
		Cookies: []*http.Cookie{{Name: "MUSIC_U", Value: musicU}},
	}
}

// Cookie 切片只替换，不原地修改
func (c *Client) requestData() utils.RequestData {
	c.mu.RLock()
	defer c.mu.RUnlock()
	data := c.data
	data.HTTPClient = c.http
	return data
}

// 匿名申请扫码登录 key
func (c *Client) CreateQRLoginKey() (string, error) {
	result, err := api.GetQrUnikey(utils.RequestData{HTTPClient: c.http})
	if err != nil {
		return "", err
	}
	if result.Unikey == "" {
		return "", fmt.Errorf("获取扫码登录 key 失败: %s", result.RawJson)
	}
	return result.Unikey, nil
}

func QRLoginContent(unikey string) string {
	return "https://music.163.com/login?codekey=" + unikey
}

// 匿名查询扫码状态，成功时返回新的 MUSIC_U
func (c *Client) CheckQRLogin(key string) (status QRLoginStatus, nickname, musicU string, err error) {
	result, header, err := api.CheckQrLogin(utils.RequestData{HTTPClient: c.http}, key)
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

func extractMUSICU(header http.Header) string {
	for _, cookie := range header.Values("Set-Cookie") {
		first := strings.SplitN(cookie, ";", 2)[0]
		if name, value, ok := strings.Cut(strings.TrimSpace(first), "="); ok && name == "MUSIC_U" {
			return value
		}
	}
	return ""
}

func (c *Client) LoginStatusValid() (bool, error) {
	result, err := api.GetLoginStatus(c.requestData())
	if err != nil {
		return false, err
	}
	return result.Code == 200 && result.Profile.UserId != 0, nil
}
