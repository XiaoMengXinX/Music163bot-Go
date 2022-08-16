package bot

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/XiaoMengXinX/Music163Api-Go/api"
	"github.com/XiaoMengXinX/Music163Api-Go/types"
	"github.com/sirupsen/logrus"
)

// 判断数组包含关系
func in(target string, strArray []string) bool {
	sort.Strings(strArray)
	index := sort.SearchStrings(strArray, target)
	if index < len(strArray) && strArray[index] == target {
		return true
	}
	return false
}

// 解析作曲家信息
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

// 判断文件夹是否存在/新建文件夹
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

// 校验 md5
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
		return false, fmt.Errorf(md5VerFailed)
	}
	return true, nil
}

// 解析 MusicID
func parseMusicID(text string) int {
	var replacer = strings.NewReplacer("\n", "", " ", "")
	messageText := replacer.Replace(text)
	musicid, _ := strconv.Atoi(linkTestMusic(messageText))
	return musicid
}

// 解析 ProgramID
func parseProgramID(text string) int {
	var replacer = strings.NewReplacer("\n", "", " ", "")
	messageText := replacer.Replace(text)
	programid, _ := strconv.Atoi(linkTestProgram(messageText))
	return programid
}

// 提取数字
func extractInt(text string) string {
	matchArr := regInt.FindStringSubmatch(text)
	if len(matchArr) == 0 {
		return ""
	}
	return matchArr[0]
}

// 解析分享链接
func linkTestMusic(text string) string {
	return extractInt(reg5.ReplaceAllString(reg4.ReplaceAllString(reg3.ReplaceAllString(reg2.ReplaceAllString(reg1.ReplaceAllString(text, ""), ""), ""), ""), ""))
}

func linkTestProgram(text string) string {
	return extractInt(reg5.ReplaceAllString(reg4.ReplaceAllString(reg3.ReplaceAllString(regP4.ReplaceAllString(regP3.ReplaceAllString(regP2.ReplaceAllString(regP1.ReplaceAllString(text, ""), ""), ""), ""), ""), ""), ""))
}

// 判断 error 是否为超时错误
func isTimeout(err error) bool {
	if strings.Contains(fmt.Sprintf("%v", err), "context deadline exceeded") {
		return true
	}
	return false
}

// 判断是否是愚人节
func isAprilFoolsDay() bool {
	_, m, d := time.Now().Date()
	return m == time.Month(4) && d == 1
}

// 获取电台节目的 MusicID
func getProgramRealID(programID int) int {
	programDetail, err := api.GetProgramDetail(data, programID)
	if err != nil {
		return 0
	}
	if programDetail.Program.MainSong.ID != 0 {
		return programDetail.Program.MainSong.ID
	}
	return 0
}
