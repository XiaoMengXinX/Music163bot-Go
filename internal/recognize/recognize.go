package recognize

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	ncm "github.com/XiaoMengXinX/Music163-Audio-Recognize-Api"
)

var ErrNoResult = errors.New("识别失败，可能是录音时间太短")

type Client struct {
	api *ncm.Client
}

func New(client *http.Client) *Client {
	httpClient := *client
	httpClient.Timeout = 60 * time.Second
	return &Client{api: &ncm.Client{HTTP: &httpClient}}
}

func (c *Client) Recognize(ctx context.Context, audio []byte) (int, error) {
	response, err := c.api.Recognize(ctx, audio)
	if err != nil {
		return 0, err
	}
	if response.Code != http.StatusOK {
		return 0, fmt.Errorf("识曲接口返回错误码 %d", response.Code)
	}
	var data struct {
		Result []struct {
			Song struct {
				ID int `json:"id"`
			} `json:"song"`
		} `json:"result"`
	}
	if len(response.Data) != 0 {
		if err := json.Unmarshal(response.Data, &data); err != nil {
			return 0, err
		}
	}
	if len(data.Result) == 0 || data.Result[0].Song.ID <= 0 {
		return 0, ErrNoResult
	}
	return data.Result[0].Song.ID, nil
}
