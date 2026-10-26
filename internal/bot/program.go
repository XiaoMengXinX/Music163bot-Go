package bot

import (
	"github.com/XiaoMengXinX/Music163Api-Go/types"
)

// songArtist 须与 types.SongDetailData.Ar 的元素类型一致
type songArtist = struct {
	Id    int           `json:"id"`
	Name  string        `json:"name"`
	Tns   []interface{} `json:"tns"`
	Alias []interface{} `json:"alias"`
}

// songAlbum 须与 types.SongDetailData.Al 的类型一致
type songAlbum = struct {
	Id     int           `json:"id"`
	Name   string        `json:"name"`
	PicUrl string        `json:"picUrl"`
	Tns    []interface{} `json:"tns"`
	PicStr string        `json:"pic_str"`
	Pic    int           `json:"pic"`
}

// 节目在 song/detail 中可能缺少专辑、封面，需用节目详情补全
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

// 接口可能返回名字为空的占位歌手
func hasArtistNames(artists []songArtist) bool {
	for _, a := range artists {
		if a.Name != "" {
			return true
		}
	}
	return false
}

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
