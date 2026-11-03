package logging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSetupCloseStopsFileOutput(t *testing.T) {
	dir := t.TempDir()
	log, closeLog := Setup("info", dir)
	log.Info("before close")
	if err := closeLog(); err != nil {
		t.Fatal(err)
	}
	log.Info("after close")

	data, err := os.ReadFile(filepath.Join(dir, time.Now().Local().Format("2006-01-02")+".log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "before close") || strings.Contains(string(data), "after close") {
		t.Errorf("log file = %q", data)
	}
}
