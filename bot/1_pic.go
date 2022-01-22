package bot

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/nfnt/resize"
	"github.com/sirupsen/logrus"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io/ioutil"
	"os"
)

// 缩放图片到 320x320px (黑底填充)
func resizeImg(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}

	buffer, err := ioutil.ReadAll(bufio.NewReader(file))
	if err != nil {
		return "", err
	}

	var img image.Image

	img, err = jpeg.Decode(bytes.NewReader(buffer))
	if err != nil {
		img, err = png.Decode(bytes.NewReader(buffer))
		if err != nil {
			return "", fmt.Errorf("Image decode error  %s", filePath)
		}
	}
	err = file.Close()
	if err != nil {
		return "", err
	}

	width := img.Bounds().Dx()
	height := img.Bounds().Dy()
	widthNew := 320
	heightNew := 320

	var m image.Image
	if width/height >= widthNew/heightNew {
		m = resize.Resize(uint(widthNew), uint(height)*uint(widthNew)/uint(width), img, resize.Lanczos3)
	} else {
		m = resize.Resize(uint(width*heightNew/height), uint(heightNew), img, resize.Lanczos3)
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

	out, err := os.Create(filePath + ".resize.jpg")
	if err != nil {
		return "", fmt.Errorf("Create image file error  %s", err)
	}
	defer func(out *os.File) {
		err := out.Close()
		if err != nil {
			logrus.Errorln(err)
		}
	}(out)

	err = jpeg.Encode(out, newImag, nil)
	if err != nil {
		logrus.Fatal(err)
	}
	return filePath + ".resize.jpg", nil
}
