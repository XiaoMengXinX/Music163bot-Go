package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
)

var dir = "./bot"

type metadata struct {
	Version     string     `json:"version"`
	VersionCode int        `json:"version_code"`
	Unsupported bool       `json:"unsupported"`
	Files       []fileData `json:"files"`
}

type fileData struct {
	File string `json:"file"`
	Md5  string `json:"md5"`
}

func main() {
	var data metadata
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		log.Println(err)
	}
	for _, f := range files {
		file, err := ioutil.ReadFile(fmt.Sprintf("%s/%s", dir, f.Name()))
		if err != nil {
			log.Println(err)
		}
		md5data := md5.Sum(file)
		data.Files = append(data.Files, fileData{
			File: fmt.Sprintf("bot/%s", f.Name()),
			Md5:  hex.EncodeToString(md5data[:]),
		})
	}
	metaJson, _ := json.MarshalIndent(data, "", "  ")
	err = ioutil.WriteFile("./metadata.json", metaJson, 0644)
	if err != nil {
		log.Println(err)
	}
}
