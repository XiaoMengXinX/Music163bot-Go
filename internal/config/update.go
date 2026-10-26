package config

import (
	"fmt"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var (
	// music_u 键名须全表唯一
	musicULine    = regexp.MustCompile(`^(\s*music_u\s*=\s*)('([^']*)'|"([^"]*)")(.*)$`)
	sectionLine   = regexp.MustCompile(`^\s*\[\s*([^\[\]\s]+)\s*\]\s*(#.*)?$`)
	cacheChatLine = regexp.MustCompile(`^(\s*cache_chat\s*=\s*)[+-]?\d+(.*)$`)
)

// UpdateMUSICU 修改配置中的 music_u，其余行（含注释）原样保留
func UpdateMUSICU(path, musicU string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		m := musicULine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		lines[i] = m[1] + strconv.Quote(musicU) + m[5]
		return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
	}
	return fmt.Errorf("配置文件 %s 中未找到 music_u 配置项", path)
}

// UpdateCacheChat 修改 [bot] 中的 cache_chat，缺少时插入到段首
func UpdateCacheChat(path string, chatID int64) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	value := strconv.FormatInt(chatID, 10)
	section, botLine := "", -1
	for i, line := range lines {
		if m := sectionLine.FindStringSubmatch(line); m != nil {
			section = m[1]
			if section == "bot" {
				botLine = i
			}
			continue
		}
		if section != "bot" {
			continue
		}
		if m := cacheChatLine.FindStringSubmatch(line); m != nil {
			lines[i] = m[1] + value + m[2]
			return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
		}
	}
	if botLine < 0 {
		return fmt.Errorf("配置文件 %s 中未找到 [bot] 配置段", path)
	}
	lines = slices.Insert(lines, botLine+1, "cache_chat = "+value)
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
}
