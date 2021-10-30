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
- inlinebot
- 歌曲搜索
- 为歌曲文件添加163key
- 歌曲快速分享
- 下载无损flac音频 （需设置网易云VIP账号的MUSIC_U)
- 动态更新（使用 [traefik/yaegi](https://github.com/traefik/yaegi) 作为动态扩展框架）
- 扩展插件（需设置 `EnableExt = true `或加入 `-enable-ext` 参数，插件示例参考 [demo.go](https://github.com/XiaoMengXinX/Music163bot-Go/blob/v2/extension/demo.go)）

## ⚙️ 构建

构建前请确保拥有 `Go 1.17`

**克隆代码 （使用 submoudle ）**

```
git clone https://github.com/XiaoMengXinX/Music163bot-Go
```

**使用脚本自动编译 ( 支持 windows 的 bash 环境，例如 git bash )**

```
cd Music163bot-Go
bash build.sh 

# 也可以加入环境变量以交叉编译，如
GOOS=windows GOARCH=amd64 bash build.sh
```

## 🛠️ 部署

**修改配置文件**

打开项目根目录下的 `config_example.ini`

```
# 以下为必填项
# 你的 Bot Token
BOT_TOKEN = YOUR_BOT_TOKEN

# 你的网易云 cookie （用于下载无损歌曲）
MUSIC_U = YOUR_MUSIC_U


# 以下为可选项
# 自定义 telegram bot API 地址
BotAPI = https://api.telegram.org

# 设置 bot 管理员 ID, 用 “," 分隔
BotAdmin = 1234,3456

# 是否开启 bot 的 debug 功能
BotDebug = false

# 自定义 sqlite3 数据库文件 （默认为 cache.db）
Database = cache.db

# 设置日志等级 [panic|fatal|error|warn|info|debug|trace] (默认为 info)
LogLevel = info

# 是否开启自动更新 (默认开启）, 若设置为 false 相当于 -no-update 参数
AutoUpdate = true

# 是否校验更新文件 md5 (默认开启）, 若设置为 false 相当于 -no-md5-check 参数
CheckMD5 = true

# 自定义源码路径
SrcPath = ./src

# 自定义 bot 函数入口 (默认为 bot.Start)
BotEntry = bot.Start

# 是否开启插件功能 (默认关闭)
EnableExt = false

# 自定义插件目录
ExtPath = ./ext

# 自定义插件函数入口 (默认为 ext.CustomScript)
ExtEntry = ext.CustomScript
```

**※ 修改配置后，将 `config_example.ini` 重命名为 `config.ini`**

**启动 Music163-bot**

```
$ ./Music163bot-Go
2021/10/30 13:05:40 [INFO] Music163bot-Go v2.0.0(20000) (main.go:122)
2021/10/30 13:05:40 [INFO] 正在检查更新中 (main.go:155)
2021/10/30 13:05:40 [INFO] v2.0.0(20000) 已是最新版本 (main.go:361)
2021/10/30 13:05:40 [INFO] 正在校验文件MD5 (main.go:164)
2021/10/30 13:05:40 [INFO] MD5校验成功 (main.go:169)
2021/10/30 13:05:40 [INFO] 加载版本 v2.0.0(20000) 中 (main.go:195)
2021/10/30 13:05:41 [INFO] Music163bot 验证成功 (value.go:543)
```

## 🤖 命令

- `/musicid` 或 `/netease` + `音乐ID`  —— 从 MusicID 获取歌曲
- `/search` + `关键词` —— 搜索歌曲
- `/about` —— 关于本 bot
