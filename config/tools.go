package config

import (
	"os"

	"github.com/yosuke-furukawa/json5/encoding/json5"
)

func ReadConfig(path string) (Config, error) {
	var config Config
	file, err := os.Open(path)
	if err != nil {
		return config, err
	}
	dec := json5.NewDecoder(file)
	err = dec.Decode(&config)
	return config, err
}
