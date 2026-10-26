package config

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// music_u 键名须全表唯一
var musicULine = regexp.MustCompile(`^(\s*music_u\s*=\s*)('([^']*)'|"([^"]*)")(.*)$`)

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
