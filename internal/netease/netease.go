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

// Client 是网易云 API 客户端
type Client struct {
	mu    sync.RWMutex
	data  utils.RequestData
	http  *http.Client
	level string // 请求音质等级
}

// New 创建客户端；musicU 为空时以无 cookie 方式请求，level 为音质等级
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

// SongDetailAndURL 批量请求歌曲详情与下载链接
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

// LyricAndDetail 批量请求歌词与歌曲详情
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

// SearchSong 按关键词搜索歌曲
func (c *Client) SearchSong(keyword string, limit int) (types.SearchSongData, error) {
	return api.SearchSong(c.requestData(), api.SearchSongConfig{
		Keyword: keyword,
		Limit:   limit,
	})
}

// ProgramDetail 查询电台节目详情
func (c *Client) ProgramDetail(programID int) (types.ProgramDetailData, error) {
	return api.GetProgramDetail(c.requestData(), programID)
}

// ProgramRealID 查询电台节目对应的歌曲 ID，查询失败或节目无主歌曲时返回 0
func (c *Client) ProgramRealID(programID int) int {
	programDetail, err := c.ProgramDetail(programID)
	if err != nil {
		return 0
	}
	return programDetail.Program.MainSong.ID
}
