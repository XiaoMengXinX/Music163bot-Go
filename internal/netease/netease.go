// Package netease 网易云 API 封装（Music163Api-Go）
package netease

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/XiaoMengXinX/Music163Api-Go/api"
	"github.com/XiaoMengXinX/Music163Api-Go/types"
	"github.com/XiaoMengXinX/Music163Api-Go/utils"
)

type Client struct {
	mu    sync.RWMutex
	data  utils.RequestData
	http  *http.Client
	level string // 请求音质等级
}

// musicU 为空时匿名请求
func New(musicU, level string, client *http.Client) *Client {
	c := &Client{level: level, http: client}
	if musicU != "" {
		c.data = utils.RequestData{
			Cookies: []*http.Cookie{
				{Name: "MUSIC_U", Value: musicU},
			},
		}
	}
	return c
}

func (c *Client) SongDetailAndURL(musicID int) (types.SongsDetailData, types.SongsURLData, error) {
	var detail types.SongsDetailData
	var urlData types.SongsURLData
	b := api.NewBatch(
		api.BatchAPI{
			Key:  api.SongDetailAPI,
			Json: api.CreateSongDetailReqJson([]int{musicID}),
		},
		api.BatchAPI{
			Key:  api.SongUrlAPI,
			Json: api.CreateSongURLJson(api.SongURLConfig{Ids: []int{musicID}, Level: c.level}),
		},
	)
	if err := b.Do(c.requestData()).Error; err != nil {
		return detail, urlData, err
	}
	_, result := b.Parse()
	if err := json.Unmarshal([]byte(result[api.SongDetailAPI]), &detail); err != nil {
		return detail, urlData, err
	}
	if err := json.Unmarshal([]byte(result[api.SongUrlAPI]), &urlData); err != nil {
		return detail, urlData, err
	}
	return detail, urlData, nil
}

func (c *Client) LyricAndDetail(musicID int) (types.SongLyricData, types.SongsDetailData, error) {
	var lyric types.SongLyricData
	var detail types.SongsDetailData
	b := api.NewBatch(
		api.BatchAPI{
			Key:  api.SongLyricAPI,
			Json: api.CreateSongLyricReqJson(musicID),
		},
		api.BatchAPI{
			Key:  api.SongDetailAPI,
			Json: api.CreateSongDetailReqJson([]int{musicID}),
		},
	)
	if err := b.Do(c.requestData()).Error; err != nil {
		return lyric, detail, err
	}
	_, result := b.Parse()
	if err := json.Unmarshal([]byte(result[api.SongLyricAPI]), &lyric); err != nil {
		return lyric, detail, err
	}
	if err := json.Unmarshal([]byte(result[api.SongDetailAPI]), &detail); err != nil {
		return lyric, detail, err
	}
	return lyric, detail, nil
}

func (c *Client) SearchSong(keyword string, limit int) (types.SearchSongData, error) {
	return api.SearchSong(c.requestData(), api.SearchSongConfig{
		Keyword: keyword,
		Limit:   limit,
	})
}

func (c *Client) ProgramDetail(programID int) (types.ProgramDetailData, error) {
	return api.GetProgramDetail(c.requestData(), programID)
}

// 查询失败或节目无主歌曲时返回 0
func (c *Client) ProgramRealID(programID int) int {
	programDetail, err := c.ProgramDetail(programID)
	if err != nil {
		return 0
	}
	return programDetail.Program.MainSong.ID
}
