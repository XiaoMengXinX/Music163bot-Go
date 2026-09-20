#!/bin/bash
# 统计面板离线自测（不连接 Telegram，PanelOnly 模式）
#
# 用法: bash smoke-test-panel.sh [二进制路径] [测试目录] [可选:真实 cache.db]
#   bash deploy/smoke-test-panel.sh /tmp/Music163bot-Go /tmp/panel-test ./cache.db
#
# 需要: curl（若装有 sqlite3 会额外做库侧一致性校验）
# 先以 PanelAuth=token 验证令牌门禁，再以 PanelAuth=none 验证数据接口
set -uo pipefail

BIN="${1:-./Music163bot-Go}"
DIR="${2:-/tmp/panel-test}"
CACHE="${3:-}"
PORT=18099

mkdir -p "$DIR" || exit 1
cd "$DIR" || exit 1
[ -x "$BIN" ] || { echo "找不到可执行文件: $BIN"; exit 1; }

write_config() { # write_config <auth> <port>
  cat > config.ini <<EOF
BOT_TOKEN = 0:smoke-test-token
Database = cache.db
StatsDB = stats.db
StatsTimezone = Asia/Shanghai
PanelEnable = true
PanelListen = 127.0.0.1:$2
PanelAuth = $1
PanelTokenTTL = 60
PanelOnly = true
LogLevel = info
EOF
}

PID=""
start_panel() { # start_panel <auth>
  stop_panel
  write_config "$1" "$PORT"
  "$BIN" >> panel.log 2>&1 &
  PID=$!
  sleep 3
  if ! kill -0 "$PID" 2>/dev/null; then
    echo "!! 面板进程已退出（auth=$1）"; tail -12 panel.log; exit 1
  fi
}
stop_panel() {
  if [ -n "$PID" ] && kill -0 "$PID" 2>/dev/null; then kill "$PID" 2>/dev/null; sleep 1; fi
}
trap 'stop_panel' EXIT

[ -n "$CACHE" ] && [ -f "$CACHE" ] && cp -f "$CACHE" cache.db
rm -f stats.db stats.db-wal stats.db-shm panel.log

BASE="http://127.0.0.1:$PORT"
ok=0; bad=0
check() { # check <描述> <上一步退出码>
  if [ "$2" -eq 0 ]; then echo "  ✓ $1"; ok=$((ok+1)); else echo "  ✗ $1"; bad=$((bad+1)); fi
}
jq() { python3 -c "import sys,json;d=json.load(sys.stdin);print(eval(sys.argv[1]))" "$1" 2>/dev/null || true; }
HAVE_SQLITE=0; command -v sqlite3 >/dev/null 2>&1 && HAVE_SQLITE=1

# ---------------- 阶段一：一次性令牌门禁 ----------------
echo "== 1. 页面与健康检查（PanelAuth=token）=="
start_panel token
curl -s --max-time 5 "$BASE/api/health" | grep -q '"ok":true'; check "面板存活" "$?"
curl -s --max-time 5 "$BASE/api/health" | grep -q '"auth":"token"'; check "默认认证方式为一次性令牌" "$?"
PAGE=$(curl -s --max-time 5 "$BASE/")
echo "$PAGE" | grep -q '<title>统计面板</title>'; check "首页标题为「统计面板」" "$?"
echo "$PAGE" | grep -q 'id="themeBtn"'; check "页面含明暗切换按钮" "$?"
echo "$PAGE" | grep -q 'data-theme="light"'; check "默认亮色主题" "$?"
echo "$PAGE" | grep -q 'html\[data-theme="dark"\]'; check "包含暗色主题样式" "$?"
echo "$PAGE" | grep -q 'id="tok"'; check "页面含一次性令牌入口" "$?"

echo "== 2. 未认证时的门禁 =="
for p in "/api/summary" "/api/chats" "/api/weeks" "/api/top" "/api/tracks" "/api/track?id=1" "/api/backfill"; do
  [ "$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 "$BASE$p")" = "401" ]; check "未认证访问 $p 返回 401" "$?"
done
curl -s --max-time 5 "$BASE/api/session" | grep -q '"authed":false'; check "/api/session 报告未登录" "$?"
curl -s -X POST -H 'Content-Type: application/json' -d '{"token":"deadbeef"}' "$BASE/api/session" | grep -q '令牌无效'; check "错误令牌被拒绝" "$?"
stop_panel

# ---------------- 阶段二：数据接口 ----------------
echo "== 3. 数据接口（PanelAuth=none，同一份统计库）=="
start_panel none
curl -s --max-time 5 "$BASE/api/health" | grep -q '"auth":"none"'; check "免密模式健康检查" "$?"
[ "$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 "$BASE/api/summary")" = "200" ]; check "免密模式可直接读取数据接口" "$?"
curl -s --max-time 5 "$BASE/api/session" | grep -q '"authed":true'; check "免密模式 /api/session 报告已登录" "$?"

BF=$(curl -s -X POST "$BASE/api/backfill")
echo "  回填: $BF"
echo "$BF" | grep -q '"songs":'; check "回填接口可用" "$?"
if [ "$HAVE_SQLITE" = "1" ]; then
  N1=$(sqlite3 stats.db 'select count(*) from play_records;' 2>/dev/null)
  curl -s -X POST "$BASE/api/backfill" >/dev/null
  N2=$(sqlite3 stats.db 'select count(*) from play_records;' 2>/dev/null)
  echo "  回填记录数: 第一次=$N1 第二次=$N2（应相同）"
  [ -n "$N1" ] && [ "$N1" = "$N2" ]; check "回填幂等" "$?"
fi

SUM=$(curl -s "$BASE/api/summary")
PT=$(echo "$SUM" | jq "d['summary']['playsTotal']")
GT=$(echo "$SUM" | jq "d['summary']['groups']")
echo "  概览: playsTotal=$PT groups=$GT"
echo "$SUM" | jq "len(d['summary']['dayPlays'])" | grep -q 7; check "本周每日分布为 7 天" "$?"
if [ "$HAVE_SQLITE" = "1" ]; then
  DBPT=$(sqlite3 stats.db 'select count(*) from play_records;' 2>/dev/null)
  [ "$PT" = "$DBPT" ]; check "playsTotal 与库一致" "$?"
fi

CH=$(curl -s "$BASE/api/chats?only=group")
echo "$CH" | grep -q '"rows":\['; check "群列表返回数据" "$?"
WK=$(curl -s "$BASE/api/weeks?limit=52")
W=$(echo "$WK" | jq "d['weeks'][0]['week']")
echo "  最近有数据的周: $W"
if [ -n "$W" ] && [ "$W" != "None" ]; then
  TOP=$(curl -s "$BASE/api/top?week=$W&scope=all&limit=5")
  echo "$TOP" | grep -q '"rank":1'; check "周榜返回排名" "$?"
  echo "$TOP" | grep -q '"start":"'; check "周榜返回周起止日期" "$?"
  MID=$(echo "$TOP" | jq "d['rows'][0]['musicId']")
  DT=$(curl -s "$BASE/api/track?id=$MID")
  echo "$DT" | grep -q '"weeks":\['; check "曲目详情含按周分布" "$?"
  CHATID=$(echo "$DT" | jq "d['chats'][0]['chatId']")
  [ "$CHATID" != "None" ] && [ "$CHATID" != "0" ]; check "曲目详情的会话 ID 正确映射" "$?"
fi
SC=$(curl -s "$BASE/api/tracks?q=%E5%A4%9C")
echo "$SC" | grep -q '"musicId"'; check "曲目搜索可用" "$?"

echo "== 4. PanelOnly 模式下的行为 =="
curl -s -X POST "$BASE/api/sync" | grep -q 'bot 尚未就绪'; check "未连接 bot 时 sync 返回明确错误" "$?"
curl -s "$BASE/api/sync/status" | grep -q '"total":0'; check "校验状态接口可用" "$?"

echo "== 5. 落盘检查 =="
[ -s stats.db ]; check "stats.db 已生成" "$?"
if [ "$HAVE_SQLITE" = "1" ]; then
  echo "  play_records=$(sqlite3 stats.db 'select count(*) from play_records;') chat_records=$(sqlite3 stats.db 'select count(*) from chat_records;')"
  echo "  played_at 类型=$(sqlite3 stats.db 'select typeof(played_at) from play_records limit 1;')"
fi
stop_panel

echo
echo "== 日志（最后 6 行）=="
tail -6 panel.log
echo
echo "结果: 通过 $ok 项, 失败 $bad 项"
[ "$bad" -eq 0 ]
