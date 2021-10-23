package bot

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"sync"
)

const (
	userAgent = `NeteaseMusic/6.5.0.1575377963(164);Dalvik/2.1.0 (Linux; U; Android 9; MIX 2 MIUI/V12.0.1.0.PDECNXM)`
)

// HttpDownloader 下载数据
type HttpDownloader struct {
	url           string
	filename      string
	contentLength int
	acceptRanges  bool // 是否支持断点续传
	numThreads    int  // 同时下载线程数
}

// New 新建下载任务
func New(url string, numThreads int) (*HttpDownloader, error) {
	var urlSplits = strings.Split(url, "/")
	var filename = urlSplits[len(urlSplits)-1]
	res, err := http.Head(url)
	if err != nil {
		return nil, err
	}
	httpDownload := new(HttpDownloader)
	httpDownload.url = url
	httpDownload.contentLength = int(res.ContentLength)
	httpDownload.numThreads = numThreads
	httpDownload.filename = filename
	if len(res.Header["Accept-Ranges"]) != 0 && res.Header["Accept-Ranges"][0] == "bytes" {
		httpDownload.acceptRanges = true
	} else {
		httpDownload.acceptRanges = false
	}

	return httpDownload, nil
}

// Download 下载综合调度
func (h *HttpDownloader) Download() (err error) {
	f, err := os.Create(cacheDir + "/" + h.filename)
	if err != nil {
		return err
	}
	defer func(f *os.File) {
		e := f.Close()
		if e != nil {
			err = fmt.Errorf("%v", e)
		}
	}(f)
	if h.acceptRanges == false {
		resp, err := http.Get(h.url)
		if err != nil {
			return err
		}
		err = save2file(cacheDir+"/"+h.filename, 0, resp)
		if err != nil {
			return err
		}
	} else {
		var wg sync.WaitGroup
		for _, ranges := range h.Split() {
			wg.Add(1)
			go func(start, end int) {
				defer wg.Done()
				err := h.download(start, end)
				if err != nil {
					logrus.Errorf("An error occurred while downloading %s: %v", h.url, err)
					return
				}
			}(ranges[0], ranges[1])
		}
		wg.Wait()
	}
	return nil
}

// Split 下载文件分段
func (h *HttpDownloader) Split() [][]int {
	var ranges [][]int
	blockSize := h.contentLength / h.numThreads
	for i := 0; i < h.numThreads; i++ {
		var start = i * blockSize
		var end = (i+1)*blockSize - 1
		if i == h.numThreads-1 {
			end = h.contentLength - 1
		}
		ranges = append(ranges, []int{start, end})
	}
	return ranges
}

// 多线程下载
func (h *HttpDownloader) download(start, end int) (err error) {
	req, err := http.NewRequest("GET", h.url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%v-%v", start, end))
	req.Header.Set("User-Agent", userAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func(Body io.ReadCloser) {
		e := Body.Close()
		if e != nil {
			err = fmt.Errorf("%v", err)
		}
	}(resp.Body)
	err = save2file(cacheDir+"/"+h.filename, int64(start), resp)
	if err != nil {
		return err
	}
	return nil
}

// 保存文件
func save2file(filename string, offset int64, resp *http.Response) error {
	f, err := os.OpenFile(filename, os.O_WRONLY, 0660)
	if err != nil {
		return err
	}
	_, err = f.Seek(offset, 0)
	if err != nil {
		return err
	}
	defer func(f *os.File) {
		e := f.Close()
		if e != nil {
			err = fmt.Errorf("%v", e)
		}
	}(f)
	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	_, err = f.Write(content)
	if err != nil {
		return err
	}
	return nil
}
