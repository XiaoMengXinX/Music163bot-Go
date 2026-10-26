// Package fetch 下载音乐与文件
package fetch

import (
	"context"
	"strings"
	"time"

	downloader "github.com/XiaoMengXinX/SimpleDownloader"
)

// 替换为可直连的网易 CDN 域名
var hostReplacer = strings.NewReplacer("m8.", "m7.", "m801.", "m701.", "m804.", "m701.", "m704.", "m701.")

type Progress struct {
	Attempt  int    // 第几次尝试，从 1 开始
	ViaProxy bool   // 是否经由反代
	Written  int64  // 已下载字节
	Total    int64  // 文件总大小
	Speed    string // 最近 500ms 的速度
}

type Downloader struct {
	d         *downloader.Downloader
	retries   int
	proxy     string
	autoRetry bool
}

func New(savePath string, timeout time.Duration, retries int, proxy string, autoRetry bool) *Downloader {
	return &Downloader{
		d:         downloader.NewDownloader().SetSavePath(savePath).SetBreakPoint(true).SetTimeOut(timeout),
		retries:   retries,
		proxy:     proxy,
		autoRetry: autoRetry,
	}
}

// DownloadFile 下载单个文件（如封面）到 savePath，不重试
func (dl *Downloader) DownloadFile(url, fileName string) error {
	task, err := dl.d.NewDownloadTask(url)
	if err != nil {
		return err
	}
	return task.SetFileName(fileName).Download()
}

// 首次不使用反代，失败后按配置切换并复用任务续传
func (dl *Downloader) DownloadMusic(ctx context.Context, url, fileName string, progress func(Progress)) error {
	task, err := dl.d.NewDownloadTask(url)
	if err != nil {
		return err
	}
	task.ReplaceHostName(hostReplacer.Replace(task.GetHostName())).ForceHttps().ForceMultiThread().SetFileName(fileName)

	attempts := dl.retries + 1
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		viaProxy := attempt > 1 && dl.proxy != "" && dl.autoRetry
		if viaProxy && attempt == 2 {
			task.WithResolvedIpOnHost(dl.proxy)
		}
		lastErr = dl.watch(ctx, task, attempt, viaProxy, progress)
		if lastErr == nil {
			return nil
		}
		if ctx.Err() != nil {
			task.CleanTempFiles()
			return ctx.Err()
		}
	}
	task.CleanTempFiles()
	return lastErr
}

func (dl *Downloader) watch(ctx context.Context, task *downloader.DownloadTask, attempt int, viaProxy bool, progress func(Progress)) error {
	errCh := task.DownloadWithChannel()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case err := <-errCh:
			return err
		case <-ctx.Done():
			task.Cancel()
			<-errCh
			return ctx.Err()
		case <-ticker.C:
			written, total := task.GetWrittenBytes(), task.GetFileSize()
			if total != 0 && written != 0 {
				progress(Progress{
					Attempt:  attempt,
					ViaProxy: viaProxy,
					Written:  written,
					Total:    total,
					Speed:    task.CalculateSpeed(time.Millisecond * 500),
				})
			}
		}
	}
}
