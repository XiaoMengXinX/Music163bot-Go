package fetch

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"io"
	"os"
)

var ErrMD5Failed = errors.New("MD5校验失败")

func VerifyMD5(filePath, expect string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()
	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if hex.EncodeToString(h.Sum(nil)) != expect {
		return ErrMD5Failed
	}
	return nil
}
