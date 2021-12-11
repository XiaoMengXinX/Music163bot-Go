package bot

import (
	"encoding/json"
	"fmt"
	"github.com/sirupsen/logrus"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"strconv"
)

type metadata struct {
	Version     string `json:"version"`
	VersionCode int    `json:"version_code"`
	Files       []struct {
		File string `json:"file"`
		Md5  string `json:"md5"`
	} `json:"files"`
}

type versions struct {
	Version     string `json:"version"`
	VersionCode int    `json:"version_code"`
	CommitSha   string `json:"commit_sha"`
}

func getUpdate() (meta metadata, err error) {
	versionData, err := getVersions()
	meta, err = checkUpdate(versionData)
	if err != nil {
		return meta, err
	}
	if config["CheckMD5"] != "false" {
		logrus.Println("正在校验文件MD5")
		err := checkUpdateMD5(meta)
		if err != nil {
			return meta, err
		}
		logrus.Println("MD5校验成功")
	}
	return meta, err
}

func getLocalVersion() (meta metadata, err error) {
	if fileExists(fmt.Sprintf("%s/version.json", config["SrcPath"])) {
		content, err := ioutil.ReadFile(fmt.Sprintf("%s/version.json", config["SrcPath"]))
		if err != nil {
			return meta, err
		}
		err = json.Unmarshal(content, &meta)
		return meta, err
	}
	return meta, err
}

func checkUpdate(versionData []versions) (meta metadata, err error) {
	dirExists(config["SrcPath"])
	var versionName string
	var versionCode int
	currentVersion, _ := getLocalVersion()
	if currentVersion.VersionCode != 0 {
		versionCode = currentVersion.VersionCode
		versionName = currentVersion.Version
		meta = currentVersion
	} else {
		versionCode, _ = strconv.Atoi(config["BinVersionCode"])
		versionName = config["BinVersionName"]
	}

	latest := func() versions {
		for _, v := range versionData {
			if v.VersionCode > versionCode {
				return v
			}
		}
		return versions{}
	}()
	if latest.VersionCode == 0 {
		logrus.Printf("%s(%d) 已是最新版本", versionName, versionCode)
		return meta, err
	}

	logrus.Printf("检测到版本更新: %s(%d), 正在获取更新", latest.Version, latest.VersionCode)
	dataFile, err := getFile(fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/metadata.json", config["repoPath"], latest.CommitSha))
	if err != nil {
		return meta, err
	}
	err = ioutil.WriteFile(fmt.Sprintf("%s/version.json", config["SrcPath"]), dataFile, 0644)
	if err != nil {
		return meta, err
	}

	_ = json.Unmarshal(dataFile, &meta)
	for _, v := range meta.Files {
		srcFile, err := getFile(fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s", config["repoPath"], latest.CommitSha, v.File))
		if err != nil {
			return meta, err
		}
		err = ioutil.WriteFile(fmt.Sprintf("%s/%s", config["SrcPath"], path.Base(v.File)), srcFile, 0644)
		if err != nil {
			return meta, err
		}
	}
	logrus.Println("更新下载完成")
	return meta, err
}

func getVersions() (versionData []versions, err error) {
	updateData, err := getFile(fmt.Sprintf("https://raw.githubusercontent.com/%s/versions.json", config["rawRepoPath"]))
	if err != nil {
		return versionData, err
	}
	err = json.Unmarshal(updateData, &versionData)
	if err != nil {
		return versionData, err
	}
	return versionData, err
}

func getFile(url string) (body []byte, err error) {
	method := "GET"
	client := &http.Client{}
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return body, err
	}
	res, err := client.Do(req)
	if err != nil {
		return body, err
	}
	defer func(Body io.ReadCloser) {
		e := Body.Close()
		if e != nil {
			err = e
		}
	}(res.Body)

	body, err = ioutil.ReadAll(res.Body)
	if err != nil {
		return body, err
	}
	return body, err
}

func checkUpdateMD5(data metadata) (err error) {
	for _, f := range data.Files {
		_, err := verifyMD5(fmt.Sprintf("%s/%s", config["SrcPath"], path.Base(f.File)), f.Md5)
		if err != nil {
			return fmt.Errorf("文件: %s/%s %s ", config["SrcPath"], path.Base(f.File), err)
		}
	}
	return err
}

func fileExists(path string) bool {
	_, err := os.Stat(path) //os.Stat获取文件信息
	if err != nil {
		if os.IsExist(err) {
			return true
		}
		return false
	}
	return true
}
