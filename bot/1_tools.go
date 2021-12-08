package bot

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"github.com/XiaoMengXinX/Music163Api-Go/types"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	"os"
	"sort"
)

func init() {
	dirExists(cacheDir)
}

func in(target string, strArray []string) bool {
	sort.Strings(strArray)
	index := sort.SearchStrings(strArray, target)
	if index < len(strArray) && strArray[index] == target {
		return true
	}
	return false
}

func parseArtist(songDetail types.SongDetailData) string {
	var artists string
	for i, ar := range songDetail.Ar {
		if i == 0 {
			artists = ar.Name
		} else {
			artists = fmt.Sprintf("%s/%s", artists, ar.Name)
		}
	}
	return artists
}

func dirExists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		err := os.Mkdir(path, os.ModePerm)
		if err != nil {
			logrus.Errorf("mkdir %v failed: %v\n", path, err)
		}
		return false
	}
	logrus.Errorf("Error: %v\n", err)
	return false
}

func verifyMD5(filePath string, md5str string) (bool, error) {
	file, err := ioutil.ReadFile(filePath)
	if err != nil {
		return false, err
	}
	md5data := md5.Sum(file)
	var md5buffer []byte
	for _, j := range md5data[:] {
		md5buffer = append(md5buffer, j)
	}
	if hex.EncodeToString(md5buffer) != md5str {
		return false, fmt.Errorf("MD5校验失败")
	}
	return true, nil
}

func linkTest(text string) string {
	return reg5.ReplaceAllString(reg4.ReplaceAllString(reg3.ReplaceAllString(reg2.ReplaceAllString(reg1.ReplaceAllString(text, ""), ""), ""), ""), "")
}
