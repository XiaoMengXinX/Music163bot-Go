// Package tagging 封面处理与 163key/ID3 标签写入
package tagging

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	_ "image/png"
	"io"
	"os"

	_ "golang.org/x/image/webp"

	"github.com/XiaoMengXinX/163KeyMarker"
	"github.com/XiaoMengXinX/Music163Api-Go/types"
	"github.com/nfnt/resize"
)

// ResizeCover 缩放封面到 320x320（黑底填充），输出 *.resize.jpg
func ResizeCover(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	buffer, err := io.ReadAll(file)
	closeErr := file.Close()
	if err != nil {
		return "", err
	}
	if closeErr != nil {
		return "", closeErr
	}

	img, _, err := image.Decode(bytes.NewReader(buffer))
	if err != nil {
		return "", fmt.Errorf("image decode error %s: %w", filePath, err)
	}

	width := img.Bounds().Dx()
	height := img.Bounds().Dy()

	var m image.Image
	if width/height >= 1 {
		m = resize.Resize(320, uint(height)*320/uint(width), img, resize.Lanczos3)
	} else {
		m = resize.Resize(uint(width*320/height), 320, img, resize.Lanczos3)
	}

	newImag := image.NewNRGBA(image.Rect(0, 0, 320, 320))
	if m.Bounds().Dx() > m.Bounds().Dy() {
		draw.Draw(newImag, image.Rectangle{
			Min: image.Point{Y: (320 - m.Bounds().Dy()) / 2},
			Max: image.Point{X: 320, Y: 320},
		}, m, m.Bounds().Min, draw.Src)
	} else {
		draw.Draw(newImag, image.Rectangle{
			Min: image.Point{X: (320 - m.Bounds().Dx()) / 2},
			Max: image.Point{X: 320, Y: 320},
		}, m, m.Bounds().Min, draw.Src)
	}

	outPath := filePath + ".resize.jpg"
	out, err := os.Create(outPath)
	if err != nil {
		return "", err
	}
	if err := jpeg.Encode(out, newImag, nil); err != nil {
		_ = out.Close()
		return "", err
	}
	if err := out.Close(); err != nil {
		return "", err
	}
	return outPath, nil
}

// Write 向音乐文件写入 163key 与封面标签；带封面失败时降级为无封面重试。
func Write(filePath, picPath string, songDetail types.SongDetailData, songURL types.SongURLData) error {
	mark := marker.CreateMarker(songDetail, songURL)

	musicFile, err := os.Open(filePath)
	if err != nil {
		return err
	}
	err = writeTags(musicFile, picPath, mark)
	_ = musicFile.Close()
	if err == nil || picPath == "" {
		return err
	}

	musicFile, err = os.Open(filePath)
	if err != nil {
		return err
	}
	err = writeTags(musicFile, "", mark)
	_ = musicFile.Close()
	return err
}

func writeTags(musicFile *os.File, picPath string, mark marker.MarkerData) error {
	if picPath == "" {
		return marker.AddMusicID3V2(musicFile, nil, mark)
	}
	picFile, err := os.Open(picPath)
	if err != nil {
		return err
	}
	defer picFile.Close()
	return marker.AddMusicID3V2(musicFile, picFile, mark)
}
