package bot

import (
	"github.com/XiaoMengXinX/Music163Api-Go/types"
)

// 节目主歌曲在 song/detail 接口里返回的元数据是稀疏的
//（专辑/封面常为空），用节目详情补全，展示/163key/封面全部受益。

// songArtist 与 types.SongDetailData.Ar 的匿名结构体一致
type songArtist = struct {
	Id    int           `json:"id"`
	Name  string        `json:"name"`
	Tns   []interface{} `json:"tns"`
	Alias []interface{} `json:"alias"`
}

// songAlbum 与 types.SongDetailData.Al 的匿名结构体一致
type songAlbum = struct {
	Id     int           `json:"id"`
	Name   string        `json:"name"`
	PicUrl string        `json:"picUrl"`
	Tns    []interface{} `json:"tns"`
	PicStr string        `json:"pic_str"`
	Pic    int           `json:"pic"`
}

// enrichFromProgram 用节目详情补全歌曲详情；详情为空时直接构造一条
func enrichFromProgram(detail *types.SongsDetailData, prog *types.ProgramDetailData) {
	p := prog.Program
	if len(detail.Songs) == 0 {
		detail.Songs = []types.SongDetailData{{
			Id:   p.MainSong.ID,
			Name: p.MainSong.Name,
			Dt:   p.MainSong.Duration,
			Al:   songAlbum{Name: p.Name, PicUrl: p.CoverURL},
		}}
	}
	song := &detail.Songs[0]
	if song.Name == "" {
		song.Name = p.MainSong.Name
	}
	if song.Name == "" {
		song.Name = p.Name
	}
	if song.Dt == 0 {
		song.Dt = p.MainSong.Duration
	}
	if song.Al.Name == "" {
		song.Al.Name = p.Name
	}
	if song.Al.PicUrl == "" {
		song.Al.PicUrl = p.CoverURL
	}
	if !hasArtistNames(song.Ar) {
		song.Ar = programArtists(prog)
	}
}

// hasArtistNames 歌手列表是否至少有一个非空名字（接口会返回名字为空的占位歌手）
func hasArtistNames(artists []songArtist) bool {
	for _, a := range artists {
		if a.Name != "" {
			return true
		}
	}
	return false
}

// programArtists 节目主歌曲的歌手列表，为空时回退到主播昵称
func programArtists(prog *types.ProgramDetailData) []songArtist {
	var artists []songArtist
	for _, a := range prog.Program.MainSong.Artists {
		artists = append(artists, songArtist{Id: a.ID, Name: a.Name})
	}
	if len(artists) == 0 && prog.Program.Dj.Nickname != "" {
		artists = append(artists, songArtist{Name: prog.Program.Dj.Nickname})
	}
	return artists
}
