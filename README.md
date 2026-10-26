<h1 align="center">Music163bot</h1>

<h4 align="center">一个用来下载/分享/搜索网易云歌曲的telegram bot</h4>

<p align="center">演示bot：<a href="https://t.me/Music163bot">https://t.me/Music163bot</a></p>

<p align="center">
	<a href="https://github.com/XiaoMengXinX/Music163bot-Go/releases">
		<img src="https://img.shields.io/github/v/release/XiaoMengXinX/Music163bot-Go?include_prereleases&style=flat-square">
  	</a>
	<a href="https://tgbotmau.quoi.dev/?bot=Music163bot" target="_blank">
		<img alt="@Music163bot MAU" title="@Music163bot MAU" src="https://tgbotmau.quoi.dev/api/bot/Music163bot/mau/badge?style=flat">
	</a>
</p>

## ✨ 特性

- 分享链接嗅探（含 `163cn.tv` / `163cn.link` 短链）
- inlinebot
- 歌曲搜索
- 为歌曲文件添加163key
- 歌曲快速分享
- 下载无损flac音频（需设置网易云VIP账号的 MUSIC_U）
- 网易云扫码登录（管理员 `/login`，免手动抓 MUSIC_U）
- 语音识别搜歌（`/recognize`，依赖 ffmpeg）
- 优雅退出与 `/reload` 热重载配置

## ⚙️ 构建

构建前请确保拥有 `Go 1.22` 或更高版本

```
git clone -b v3 https://github.com/XiaoMengXinX/Music163bot-Go
cd Music163bot-Go
bash build.sh

# 也可以加入环境变量以交叉编译，如
GOOS=windows GOARCH=amd64 bash build.sh
```

## 🛠️ 部署

**修改配置文件**

复制 `config_example.toml` 为 `config.toml` 并按注释填写：

```toml
[bot]
token = "YOUR_BOT_TOKEN"   # 必填
admin = [123456]           # 管理员 ID

[netease]
music_u = "YOUR_MUSIC_U"   # 必填，网易云 cookie 中的 MUSIC_U（下载无损需 VIP 账号）
level = "hires"            # 优先音质，账号无权限时自动降级

[download]
timeout = "60s"
retries = 3
concurrency = 4
reverse_proxy = ""
auto_retry = true
```

完整配置项见 `config_example.toml`。所有配置都可用 `MUSIC163BOT_` 前缀的环境变量覆盖
（如 `MUSIC163BOT_BOT_TOKEN`、`MUSIC163BOT_DOWNLOAD_TIMEOUT`），便于容器部署。

优先音质 `level` 可选 `standard` / `higher` / `lossless` / `hires`（默认）/
`jyeffect`（高清环绕声）/ `sky`（沉浸环绕声）/ `jymaster`（超清母带），
高音质需要对应 VIP 等级，无权限时服务端自动降级。

指令菜单在每次启动时自动注册（无需在 BotFather 手动设置）：
普通用户看到 `/music` `/search` 等公共指令，管理员额外看到 `/login` `/rmcache` `/reload`。

inline 模式需要在 BotFather 手动开启一次（API 无法代办）：发 `/setinline` 选择 bot
并设置占位文案；建议同时 `/setinlinefeedback` 设为 0。

**启动**

```
$ ./Music163bot-Go -c config.toml
2026/01/01 12:00:00 [INFO] Music163bot v3.0.0 启动中 (main.go:47)
MUSIC_U 已失效，请重新扫码登录

请使用网易云音乐 App 扫描二维码登录（Ctrl-C 退出）
...（终端二维码）...
扫码登录成功，已写入 config.toml
2026/01/01 12:00:20 [INFO] Music163bot 验证成功 (app.go:100)
```

`MUSIC_U` 缺失或已失效时，bot 会照常启动，终端内会打印登录
二维码，同时管理员会收到带「扫码登录」按钮的提示，点击按钮扫码登录；
扫码成功自动写入配置并恢复下载，超时未登录则自动停止运行。
运行中 `MUSIC_U` 失效时，新歌下载会暂停（缓存过的歌曲仍可发送），管理员重新登录后自动恢复。

管理员发送 `/reload` 可在进程内重载配置，无需重启。

**Docker 部署**

配置好 `config.toml` 后启动：

```sh
mkdir -p data
docker compose up -d
```

`data/` 存放歌曲缓存数据库 `cache.db`，以及 SQLite 运行时生成的
`cache.db-wal`、`cache.db-shm` 文件。首次启动会自动创建数据库。

已有数据库时，先停止旧 bot，再将原来的 `cache.db` 及同名 `-wal`、`-shm` 文件（如有）一起移入 `data/`，然后启动容器。

镜像内已包含 ffmpeg，以 root 用户运行。配置、数据库、缓存和日志均挂载到宿主机。

**自建 Bot API 本地模式**

Bot API 以 `--local` 运行时，在 `config.toml` 中开启本地文件读取：

```toml
[bot]
api_url = "http://127.0.0.1:8081"
local_mode = true
```

也可设置环境变量 `MUSIC163BOT_BOT_LOCAL_MODE=true`。此选项只控制 bot 的文件读取方式，
Bot API 服务需要单独启用 `--local`。

bot 必须能读取 `getFile` 返回的绝对路径。同机直接运行时确保有读取权限；
如果 bot 在容器中，需要将 API 的实际文件存储目录以相同绝对路径、只读方式挂载到 bot 容器。
例如 API 的存储目录为 `/srv/telegram-bot-api`，在 bot 服务的 `volumes` 中添加：

```yaml
- /srv/telegram-bot-api:/srv/telegram-bot-api:ro
```

容器内的 `api_url` 应填写能访问到的 API 地址，`127.0.0.1` 指向容器自身。
跨服务器使用本地模式需要共享存储。语音文件会复制到 bot 的临时目录后处理，原文件不会被修改或删除。
使用官方 API 时保持 `local_mode = false`。

## 📦 从 v2 升级

命令和交互与 v2 一致，直接用原来的目录启动即可：

1. **配置**：把 v2 的 `config.ini` 放在程序同目录，首次启动会自动转换成
   `config.toml` 并打印提示；也可以先手动执行
   `./Music163bot-Go migrate-config -c config.ini -o config.toml`。
   v2 里没有实现的配置项（`AutoUpdate`、`CheckMD5`、`SrcPath`、`BotEntry`）在转换时丢弃并提示。
2. **数据库**：v2 的 `cache.db` 首次启动时自动迁移，重复的缓存行会去重并建唯一索引，
   已缓存的歌曲全部保留。

## 🤖 命令

- `/music <ID>` 或 `/netease <关键词>` —— 按 ID 或关键词点歌
- `/search <关键词>` —— 搜索歌曲，点数字按钮选择
- `/program <ID>` —— 电台节目
- `/lyric <关键词/ID>`（或回复一条消息）—— 发送歌词文件
- `/recognize`（回复一条语音）—— 语音识别搜歌
- `/status` —— 缓存统计
- `/about` —— 关于本 bot
- `/start <ID>` —— 深链点歌（inline 未缓存引导）
- 管理员：`/rmcache <ID/链接>` 清除缓存、`/reload` 重载配置、`/login` 网易云扫码登录
- 直接发送网易云分享链接即可嗅探点歌；inline 输入 `help` 查看用法

**网易云扫码登录**：MUSIC_U 过期时，管理员发送 `/login`，bot 回一张二维码，
用网易云音乐 App 扫码确认后，bot 会立即用新账号凭据继续工作（无需重启），
并把新的 `music_u` 自动写回 `config.toml`。
