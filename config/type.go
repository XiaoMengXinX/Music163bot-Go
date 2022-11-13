package config

type Config struct {
	General  GeneralConfig    `json:"general"`
	Bot      BotConfig        `json:"bot"`
	Sqlite   SqliteConfig     `json:"sqlite"`
	Download []DownloadConfig `json:"download"`
}

type GeneralConfig struct {
	Cookie   string `json:"MUSIC_U"`
	LogLevel string `json:"log_level"`
	LogPath  string `json:"log_path"`
}

type BotConfig struct {
	Token              string `json:"token"`
	API                string `json:"api_url"`
	IgnoreInvalidCerts bool   `json:"ignore_invalid_certs"`
	Proxy              string `json:"proxy"`
	Admin              []int  `json:"admin"`
	Debug              bool   `json:"debug"`
}

type SqliteConfig struct {
	Path     string `json:"path"`
	LogLevel string `json:"log_level"`
}

type DownloadConfig struct {
	Name         string `json:"name"`
	Timeout      int    `json:"timeout"`
	Proxy        string `json:"proxy"`
	Cdn          string `json:"cdn"`
	ReverseProxy string `json:"reverse_proxy"`
	Scheme       string `json:"scheme"`
}
