package fetch

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestVerifyMD5(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.mp3")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	sum := md5.Sum([]byte("hello"))
	want := hex.EncodeToString(sum[:])

	if err := VerifyMD5(path, want); err != nil {
		t.Errorf("md5 一致应通过: %v", err)
	}
	if err := VerifyMD5(path, "deadbeef"); err != ErrMD5Failed {
		t.Errorf("md5 不一致应返回 ErrMD5Failed, got %v", err)
	}
	if err := VerifyMD5(filepath.Join(dir, "missing"), want); err == nil {
		t.Error("文件不存在应报错")
	}
}

func TestDownloadFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("file-content"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	dl := New(dir, 10*time.Second, 0, "", false)
	if err := dl.DownloadFile(srv.URL+"/cover.jpg", "cover.jpg"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "cover.jpg"))
	if err != nil || string(data) != "file-content" {
		t.Errorf("下载内容错误: %q %v", data, err)
	}
}

func TestDownloadMusicRetryViaProxy(t *testing.T) {
	for _, retries := range []int{1, 2, 3} {
		t.Run(fmt.Sprintf("retries=%d", retries), func(t *testing.T) {
			origin := httptest.NewTLSServer(http.NotFoundHandler())
			defer origin.Close()
			originHost := strings.TrimPrefix(origin.URL, "https://")

			var attempts atomic.Int32
			data := []byte("music-data-0123456789")
			proxy := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Host != originHost {
					t.Errorf("反代请求应保留原 Host: got %q, want %q", r.Host, originHost)
				}
				if r.URL.RequestURI() != "/song.mp3?token=test" {
					t.Errorf("反代请求路径错误: %s", r.URL.RequestURI())
				}
				w.Header().Set("Accept-Ranges", "bytes")
				if r.Method == http.MethodHead {
					attempts.Add(1)
					w.Header().Set("Content-Length", strconv.Itoa(len(data)))
					return
				}
				if attempts.Load() < int32(retries) {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				var start, end int
				if _, err := fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &start, &end); err == nil {
					if end >= len(data) {
						end = len(data) - 1
					}
					w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(data)))
					w.WriteHeader(http.StatusPartialContent)
					_, _ = w.Write(data[start : end+1])
					return
				}
				_, _ = w.Write(data)
			}))
			defer proxy.Close()
			proxyHost := strings.TrimPrefix(proxy.URL, "https://")

			dir := t.TempDir()
			dl := New(dir, 10*time.Second, retries, proxyHost, true)
			err := dl.DownloadMusic(context.Background(), origin.URL+"/song.mp3?token=test", "song.mp3", func(Progress) {})
			if err != nil {
				t.Fatalf("经反代重试后应成功: %v", err)
			}
			if got := attempts.Load(); got != int32(retries) {
				t.Errorf("反代尝试次数错误: got %d, want %d", got, retries)
			}
			got, err := os.ReadFile(filepath.Join(dir, "song.mp3"))
			if err != nil || string(got) != string(data) {
				t.Errorf("下载内容错误: %q %v", got, err)
			}
		})
	}
}

func TestDownloadMusicAllFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	dir := t.TempDir()
	dl := New(dir, 10*time.Second, 1, "", false)
	err := dl.DownloadMusic(context.Background(), srv.URL+"/song.mp3", "song.mp3", func(Progress) {})
	if err == nil {
		t.Fatal("全部失败应返回错误")
	}
	// 临时文件应被清理
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("失败后临时文件应清理, 残留 %d 个", len(entries))
	}
}

func TestDownloadMusicContextCancel(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	defer srv.Close()
	defer close(release)

	dir := t.TempDir()
	dl := New(dir, 10*time.Second, 3, "", false)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	if err := dl.DownloadMusic(ctx, srv.URL+"/song.mp3", "song.mp3", func(Progress) {}); err == nil {
		t.Fatal("ctx 取消应返回错误")
	}
}
