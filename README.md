<h1 align="center">Music163bot</h1>

<h4 align="center">一个用来下载/分享/搜索网易云歌曲的telegram bot</h4>

<p align="center">演示bot：<a href="https://t.me/Music163bot">https://t.me/Music163bot</a></p>

<p align="center">
	<a href="https://goreportcard.com/report/github.com/XiaoMengXinX/Music163bot-Go">
      <img src="https://goreportcard.com/badge/github.com/XiaoMengXinX/Music163bot-Go?style=flat-square">
	</a>
	<a href="https://github.com/XiaoMengXinX/Music163bot-Go/releases">
    <img src="https://img.shields.io/github/v/release/XiaoMengXinX/Music163bot-Go?include_prereleases&style=flat-square">
  </a>
</p>

## ✨ 特性

- 分享链接嗅探
- 内联（inline）bot
- 歌曲搜索
- 为歌曲文件添加163key
- 歌曲快速分享
- 下载无损flac音频 （需设置网易云VIP账号的MUSIC_U)

## ⚙️ 构建

构建前请确保拥有 `Go 1.16.5`

**克隆代码 （使用 submoudle ）**

```
git clone --recurse-submodules https://github.com/XiaoMengXinX/Music163bot
```

**使用脚本自动编译 ( 支持 windows 的 bash 环境，例如 git bash )**

```
cd Music163bot
bash build.sh 

# 也可以加入参数以交叉编译，如
bash build.sh linux arm64
```

## 🛠️ 部署

**修改配置文件**

打开项目根目录下的 `config_full.ini`

```
# 以下为必填项
BOT_TOKEN = YOUR_BOT_TOKEN
# 你的 Bot Token

MUSIC_U = 009BA2108A5602AE0FF1EFB4FDE0931C227DCBA6A4CB323A1CAC3522AD13C5D7F0F587C9D0738103B46E0425B2BC0F50386B1106358EC75FC89E6E178D0$
# 你的网易云 cookie （用于下载无损歌曲）

# 以下为可选项
BotAPI = https://api.telegram.org
# 可自定义接入本地 api

BotDebug = false
# 可开启 bot 的 debug 模式 （请勿用于生产环境）

BotApiDebug = false
# 可开启 tgbotapi 的 debug 模式

Database = cache.db
# 自定义 sqlite3 数据库文件 （默认为 cache.db）

LogLevel = INFO
# 设置日志等级 [TRACE|FATAL|WARN|INFO|DEBUG] 默认为 INFO

```

**※ 修改配置后，将 `config_full.ini` 重命名为 `config.ini`**

**启动 Music163-bot**

```
$ ./Music163bot-Go
2021/07/10 10:00:00 [INFO] xxxxBot 验证成功 (bot.go:45)
```

