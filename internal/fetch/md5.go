package fetch

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"io"
	"os"
)

// ErrMD5Failed MD5 校验失败
var ErrMD5Failed = errors.New("MD5校验失败")

// VerifyMD5 计算文件 md5 并与期望值比对，不匹配返回 ErrMD5Failed
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
