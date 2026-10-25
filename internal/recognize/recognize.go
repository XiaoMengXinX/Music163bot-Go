// Package recognize 语音识别：ffmpeg 转码 + 远程识别 API
package recognize

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os/exec"
	"time"
)

// DefaultAPI 默认识别服务
const DefaultAPI = "https://music-recognize.vercel.app/api/recognize"

// ErrNoResult 表示识别服务未返回结果（如录音时间太短）
var ErrNoResult = errors.New("识别失败，可能是录音时间太短")

// Client 识别客户端
type Client struct {
	apiURL string
	http   *http.Client
}

// New 创建客户端，apiURL 为空时用默认服务
func New(apiURL string) *Client {
	if apiURL == "" {
		apiURL = DefaultAPI
	}
	return &Client{apiURL: apiURL, http: &http.Client{Timeout: 60 * time.Second}}
}

// ToMp3 用 ffmpeg 把 ogg 语音转成 mp3
func ToMp3(srcPath, dstPath string) error {
	out, err := exec.Command("ffmpeg", "-y", "-i", srcPath, dstPath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg 转码失败: %w: %s", err, bytes.TrimSpace(out))
	}
	return nil
}

type resultData struct {
	Data struct {
		Result []struct {
			Song struct {
				Name string `json:"name"`
				ID   int    `json:"id"`
			} `json:"song"`
		} `json:"result"`
	} `json:"data"`
}

// Recognize 上传音频数据，返回识别到的第一个歌曲 ID
func (c *Client) Recognize(audio []byte) (int, error) {
	bodyBuf := &bytes.Buffer{}
	bodyWriter := multipart.NewWriter(bodyBuf)
	fileWriter, err := bodyWriter.CreateFormFile("file", "")
	if err != nil {
		return 0, err
	}
	if _, err := fileWriter.Write(audio); err != nil {
		return 0, err
	}
	contentType := bodyWriter.FormDataContentType()
	_ = bodyWriter.Close()

	resp, err := c.http.Post(c.apiURL, contentType, bodyBuf)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var result resultData
	if err := json.Unmarshal(respBody, &result); err != nil {
		return 0, err
	}
	if len(result.Data.Result) == 0 {
		return 0, ErrNoResult
	}
	return result.Data.Result[0].Song.ID, nil
}
