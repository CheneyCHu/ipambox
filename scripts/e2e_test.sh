#!/usr/bin/env bash
# IPAMBox E2E 测试套件：独立数据目录 + 独立端口，完整走查所有功能。
# 用法：bash scripts/e2e_test.sh [端口]   （默认 18099）
set -uo pipefail

PORT="${1:-18099}"
B="http://localhost:$PORT/api/v1"
BIN="$(cd "$(dirname "$0")/.." && pwd)/backend/ipambox"
WORK=$(mktemp -d)
PASS=0; FAIL=0

ok()   { PASS=$((PASS+1)); echo "  ✅ $1"; }
bad()  { FAIL=$((FAIL+1)); echo "  ❌ $1"; }
check(){ # check <描述> <期望> <实际>
  if [[ "$2" == "$3" ]]; then ok "$1"; else bad "$1（期望 $2，实际 $3）"; fi
}
pyget(){ python3 -c "import sys,json;print(json.load(sys.stdin)$1)" 2>/dev/null; }

echo "==> 启动服务 (port $PORT, workdir $WORK)"
cd "$WORK"
IPAMBOX_PORT=$PORT "$BIN" > server.log 2>&1 &
SRV=$!
for i in $(seq 1 20); do curl -sf "$B/setup/status" >/dev/null 2>&1 && break; sleep 0.5; done

echo "==> 1. 认证链路"
check "初始未初始化" "false" "$(curl -s $B/setup/status | pyget "['initialized']" | tr 'A-Z' 'a-z')"
check "未初始化访问 412" "412" "$(curl -s -o /dev/null -w '%{http_code}' $B/subnets/)"
check "短密码被拒" "400" "$(curl -s -o /dev/null -w '%{http_code}' -X POST $B/setup/init -d '{"password":"123"}')"
TOKEN=$(curl -s -X POST $B/setup/init -d '{"password":"admin123"}' | pyget "['token']")
[[ -n "$TOKEN" ]] && ok "初始化获得 token" || bad "初始化失败"
check "重复初始化 409" "409" "$(curl -s -o /dev/null -w '%{http_code}' -X POST $B/setup/init -d '{"password":"xxxxxx"}')"
check "无 token 401" "401" "$(curl -s -o /dev/null -w '%{http_code}' $B/subnets/)"
check "错误密码 401" "401" "$(curl -s -o /dev/null -w '%{http_code}' -X POST $B/setup/login -d '{"password":"bad"}')"
TOKEN2=$(curl -s -X POST $B/setup/login -d '{"password":"admin123"}' | pyget "['token']")
[[ -n "$TOKEN2" ]] && ok "登录获得 token" || bad "登录失败"
AUTH="Authorization: Bearer $TOKEN"

echo "==> 2. 网卡枚举"
NICS=$(curl -s $B/interfaces -H "$AUTH" | python3 -c 'import sys,json;print(len(json.load(sys.stdin)))')
[[ "$NICS" -ge 0 ]] && ok "interfaces 返回 $NICS 块网卡" || bad "interfaces 异常"

echo "==> 3. 子网与扫描闭环"
curl -s -X POST $B/subnets/ -H "$AUTH" -d '{"cidr":"127.0.0.0/30","name":"回环测试网"}' >/dev/null
check "建子网成功" "1" "$(curl -s $B/subnets/ -H "$AUTH" | python3 -c 'import sys,json;print(len(json.load(sys.stdin)))')"
check "非法 CIDR 400" "400" "$(curl -s -o /dev/null -w '%{http_code}' -X POST $B/subnets/ -H "$AUTH" -d '{"cidr":"abc"}')"
check "IPv6 子网 400" "400" "$(curl -s -o /dev/null -w '%{http_code}' -X POST $B/subnets/ -H "$AUTH" -d '{"cidr":"fd00::/64"}')"
check "过大前缀 400" "400" "$(curl -s -o /dev/null -w '%{http_code}' -X POST $B/subnets/ -H "$AUTH" -d '{"cidr":"10.0.0.0/7"}')"
check "触发扫描 202" "202" "$(curl -s -o /dev/null -w '%{http_code}' -X POST $B/subnets/1/scan -H "$AUTH")"
# 扫描为异步：轮询最多 15 秒等待结果（ARP 兜底与反向 DNS 富化会耗时）
SCAN_ST=""
for i in $(seq 1 15); do
  SCAN_ST=$(curl -s $B/subnets/1/addresses -H "$AUTH" | python3 -c 'import sys,json;a=json.load(sys.stdin);print(a[0]["status"] if a else "none")' 2>/dev/null)
  [[ "$SCAN_ST" == "online" ]] && break; sleep 1
done
check "扫描发现 127.0.0.1" "online" "$SCAN_ST"
check "统计 online=1" "1" "$(curl -s $B/subnets/1/stats -H "$AUTH" | pyget "['online']")"

echo "==> 4. 标注"
curl -s -X PATCH $B/addresses/1 -H "$AUTH" -d '{"label":"本机回环","owner":"ops","dev_type":"服务器"}' >/dev/null
check "标注回读" "本机回环" "$(curl -s $B/subnets/1/addresses -H "$AUTH" | python3 -c 'import sys,json;print(json.load(sys.stdin)[0]["label"])')"

echo "==> 5. CSV 导出/导入"
EXP=$(curl -s $B/subnets/1/export -H "$AUTH")
echo "$EXP" | grep -q '127.0.0.1' && ok "导出含数据行" || bad "导出缺数据"
echo "$EXP" | head -1 | grep -q 'ip,mac,status' && ok "导出含表头" || bad "导出缺表头"
IMP=$(printf 'ip,mac,status,hostname,label,owner,dev_type,last_seen\n127.0.0.1,,online,,CSV导入的标注,张三,服务器,\n')
check "CSV 导入更新 1 条" "1" "$(curl -s -X POST $B/subnets/1/import -H "$AUTH" -H 'Content-Type: text/csv' -d "$IMP" | pyget "['updated']")"
check "导入后标注生效" "CSV导入的标注" "$(curl -s $B/subnets/1/addresses -H "$AUTH" | python3 -c 'import sys,json;print(json.load(sys.stdin)[0]["label"])')"

echo "==> 6. 设备台账与仪表盘"
check "devices 接口" "1" "$(curl -s $B/devices -H "$AUTH" | python3 -c 'import sys,json;print(len(json.load(sys.stdin)))')"
check "台账含子网名" "回环测试网" "$(curl -s $B/devices -H "$AUTH" | python3 -c 'import sys,json;print(json.load(sys.stdin)[0]["subnet_name"])')"
check "总览 subnets=1" "1" "$(curl -s $B/stats/overview -H "$AUTH" | pyget "['subnets']")"

echo "==> 7. 离线判定（删除 127.0.0.1 可达性模拟：改用空网段）"
curl -s -X POST $B/subnets/ -H "$AUTH" -d '{"cidr":"127.255.255.252/30","name":"无人网"}' >/dev/null
curl -s -X POST $B/subnets/2/scan -H "$AUTH" >/dev/null; sleep 4
# 无人网扫不到任何设备 → 不产生记录；回环网仍在 → 不影响 online
check "回环网在线数不变" "1" "$(curl -s $B/subnets/1/stats -H "$AUTH" | pyget "['online']")"

echo "==> 8. AI 网关"
check "AI 未配置提示" "503" "$(curl -s -o /dev/null -w '%{http_code}' -X POST $B/ai/chat -H "$AUTH" -d '{"message":"hi"}')"

echo "==> 9. SPA 服务"
check "首页 200" "200" "$(curl -s -o /dev/null -w '%{http_code}' localhost:$PORT/)"
check "SPA 回退 200" "200" "$(curl -s -o /dev/null -w '%{http_code}' localhost:$PORT/subnets/1)"
CSS=$(curl -s localhost:$PORT/ | grep -o 'assets/index-[^"]*\.css' | head -1)
check "样式资源含 Tailwind" "1" "$(curl -s localhost:$PORT/$CSS | grep -c 'rounded-lg')"

echo "==> 10. 网络设置接口"
check "网卡详情返回数组" "1" "$(curl -s $B/network/interfaces/ -H "$AUTH" | python3 -c 'import sys,json;a=json.load(sys.stdin);print(1 if isinstance(a,list) else 0)')"
check "网卡含 mac 字段" "1" "$(curl -s $B/network/interfaces/ -H "$AUTH" | python3 -c 'import sys,json;a=json.load(sys.stdin);print(1 if a and "mac" in a[0] else (1 if not a else 0))')"
check "虚拟接口 lo0 被拒" "400" "$(curl -s -o /dev/null -w '%{http_code}' -X POST $B/network/interfaces/lo0/config -H "$AUTH" -d '{"family":"ipv4","mode":"static","ip":"10.9.9.9","prefix":24}')"
check "非法 IP 被拒" "400" "$(curl -s -o /dev/null -w '%{http_code}' -X POST $B/network/interfaces/en0/config -H "$AUTH" -d '{"family":"ipv4","mode":"static","ip":"999.1.1.1","prefix":24}')"
check "子网带 iface 字段" "1" "$(curl -s $B/subnets/ -H "$AUTH" | python3 -c 'import sys,json;s=json.load(sys.stdin)[0];print(1 if "iface" in s else 0)')"

echo "==> 10b. 使用率统计（按容量）"
# 127.0.0.0/30 可用容量 2，当前 1 台在线 → 50%
check "子网使用率 50%" "50" "$(curl -s $B/subnets/1/stats -H "$AUTH" | pyget "['usage_pct']")"
check "总览非 100%" "1" "$(U=$(curl -s $B/stats/overview -H "$AUTH" | pyget "['usage_pct']"); [ "$U" != "100.0" ] && echo 1 || echo 0)"

echo "==> 10c. 子网更新 / 地址增删 / 设置"
curl -s -X PUT $B/subnets/1 -H "$AUTH" -d '{"name":"回环测试网-改","vlan":10,"iface":"lo0"}' >/dev/null
check "子网更新生效" "回环测试网-改" "$(curl -s $B/subnets/ -H "$AUTH" | python3 -c 'import sys,json;print(json.load(sys.stdin)[0]["name"])')"
check "手工登记地址 201" "201" "$(curl -s -o /dev/null -w '%{http_code}' -X POST $B/subnets/1/addresses -H "$AUTH" -d '{"ip":"127.0.0.2","label":"预留网关"}')"
check "重复登记 409" "409" "$(curl -s -o /dev/null -w '%{http_code}' -X POST $B/subnets/1/addresses -H "$AUTH" -d '{"ip":"127.0.0.2"}')"
check "越界 IP 400" "400" "$(curl -s -o /dev/null -w '%{http_code}' -X POST $B/subnets/1/addresses -H "$AUTH" -d '{"ip":"10.0.0.1"}')"
check "登记为保留状态" "reserved" "$(curl -s $B/subnets/1/addresses -H "$AUTH" | pyget "[1]['status']")"
NEWID=$(curl -s $B/subnets/1/addresses -H "$AUTH" | python3 -c 'import sys,json;a=json.load(sys.stdin);print([x["id"] for x in a if x["ip"]=="127.0.0.2"][0])')
check "删除地址记录" "ok" "$(curl -s -X DELETE $B/addresses/$NEWID -H "$AUTH" | pyget "['status']")"
check "读取设置默认值" "5" "$(curl -s $B/settings/ -H "$AUTH" | pyget "['scan_interval_min']")"
curl -s -X PUT $B/settings/ -H "$AUTH" -d '{"scan_interval_min":"15","auto_scan":"0"}' >/dev/null
check "设置保存生效" "15" "$(curl -s $B/settings/ -H "$AUTH" | pyget "['scan_interval_min']")"
check "非法间隔 400" "400" "$(curl -s -o /dev/null -w '%{http_code}' -X PUT $B/settings/ -H "$AUTH" -d '{"scan_interval_min":"0"}')"

echo "==> 10d. 外网连通（断网续存）"
check "uplink 返回 online 字段" "1" "$(curl -s $B/uplink/ -H "$AUTH" | python3 -c 'import sys,json;print(1 if "online" in json.load(sys.stdin)["status"] else 0)')"
check "uplink 含探测目标" "1" "$(curl -s $B/uplink/ -H "$AUTH" | python3 -c 'import sys,json;print(1 if json.load(sys.stdin)["status"]["probe"] else 0)')"
check "uplink 含 pending 字段" "1" "$(curl -s $B/uplink/ -H "$AUTH" | python3 -c 'import sys,json;print(1 if "pending" in json.load(sys.stdin) else 0)')"
check "手动探测返回状态" "1" "$(curl -s -X POST $B/uplink/check -H "$AUTH" | python3 -c 'import sys,json;print(1 if "online" in json.load(sys.stdin)["status"] else 0)')"
check "事件历史是数组" "1" "$(curl -s "$B/uplink/events?limit=10" -H "$AUTH" | python3 -c 'import sys,json;print(1 if isinstance(json.load(sys.stdin),list) else 0)')"
check "探测目标默认值" "1" "$(curl -s $B/settings/ -H "$AUTH" | python3 -c 'import sys,json;print(1 if "223.5.5.5:53" in json.load(sys.stdin)["uplink_probe"] else 0)')"
check "非法探测目标 400" "400" "$(curl -s -o /dev/null -w '%{http_code}' -X PUT $B/settings/ -H "$AUTH" -d '{"uplink_probe":"not-a-host"}')"
check "非法探测间隔 400" "400" "$(curl -s -o /dev/null -w '%{http_code}' -X PUT $B/settings/ -H "$AUTH" -d '{"uplink_check_sec":"1"}')"
check "探测配置保存生效" "192.0.2.1:53" "$(curl -s -X PUT $B/settings/ -H "$AUTH" -d '{"uplink_probe":"192.0.2.1:53","uplink_check_sec":"60"}' >/dev/null; curl -s $B/settings/ -H "$AUTH" | pyget "['uplink_probe']")"
# 探测一个必然不可达的目标 → 状态应变离线并产生事件与告警
curl -s -X PUT $B/settings/ -H "$AUTH" -d '{"uplink_probe":"192.0.2.1:9"}' >/dev/null
sleep 1
check "不可达探测判定离线" "False" "$(curl -s -X POST $B/uplink/check -H "$AUTH" | pyget "['status']['online']")"
check "离线事件已落库" "1" "$(curl -s "$B/uplink/events" -H "$AUTH" | python3 -c 'import sys,json;a=json.load(sys.stdin);print(1 if any(not e["online"] for e in a) else 0)')"
check "离线自治告警已产生" "1" "$(curl -s $B/alerts/ -H "$AUTH" | python3 -c 'import sys,json;a=json.load(sys.stdin);print(1 if any(x["type"]=="uplink" for x in a) else 0)')"
# 恢复默认可达目标 → 手动探测应变回在线并记录恢复事件
curl -s -X PUT $B/settings/ -H "$AUTH" -d '{"uplink_probe":"223.5.5.5:53,114.114.114.114:53","uplink_check_sec":"30"}' >/dev/null
check "恢复后判定在线" "True" "$(curl -s -X POST $B/uplink/check -H "$AUTH" | pyget "['status']['online']")"
check "恢复告警已产生" "1" "$(curl -s $B/alerts/ -H "$AUTH" | python3 -c 'import sys,json;a=json.load(sys.stdin);print(1 if any(x["type"]=="uplink" and "恢复" in x["message"] for x in a) else 0)')"
check "DHCP 接口已移除 404" "404" "$(curl -s -o /dev/null -w '%{http_code}' $B/dhcp/ -H "$AUTH")"
check "DHCP 设置键已移除" "1" "$(curl -s $B/settings/ -H "$AUTH" | python3 -c 'import sys,json;print(0 if "dhcp_enabled" in json.load(sys.stdin) else 1)')"

echo "==> 10f. OTA 升级"
check "版本接口" "1" "$(curl -s $B/version -H "$AUTH" | pyget "['version']" | grep -cE '^[0-9]+\.[0-9]+\.[0-9]+$')"
mkdir -p "$WORK/upd"
echo '{"version":"9.9.9","url":"http://127.0.0.1:18334/ipambox","sha256":"","notes":"测试版"}' > "$WORK/upd/latest.json"
echo '{"version":"1.0.0","url":"http://127.0.0.1:18334/ipambox"}' > "$WORK/upd/same.json"
cp "$BIN" "$WORK/upd/ipambox"
(cd "$WORK/upd" && python3 -m http.server 18334 >/dev/null 2>&1 &)
sleep 1
curl -s -X PUT $B/settings/ -H "$AUTH" -d '{"update_manifest_url":"http://127.0.0.1:18334/latest.json"}' >/dev/null
check "检查到有更新" "True" "$(curl -s $B/update/check -H "$AUTH" | pyget "['has_update']")"
check "最新版本号" "9.9.9" "$(curl -s $B/update/check -H "$AUTH" | pyget "['latest']")"
check "更新说明透传" "测试版" "$(curl -s $B/update/check -H "$AUTH" | pyget "['notes']")"
curl -s -X PUT $B/settings/ -H "$AUTH" -d '{"update_manifest_url":"http://127.0.0.1:18334/same.json"}' >/dev/null
check "同版本无更新" "False" "$(curl -s $B/update/check -H "$AUTH" | pyget "['has_update']")"
check "已最新时升级 400" "400" "$(curl -s -o /dev/null -w '%{http_code}' -X POST $B/update/apply -H "$AUTH")"
# 多平台清单：含本机平台可更新；仅异构平台报 502
PLAT="$(uname -s | tr 'A-Z' 'a-z')_$(uname -m | sed 's/x86_64/amd64/')"
python3 -c "
import json
m={'version':'9.9.9','notes':'多平台','platforms':{'$PLAT':{'url':'http://127.0.0.1:18334/ipambox','sha256':''}}}
open('$WORK/upd/plat.json','w').write(json.dumps(m))
m2={'version':'9.9.9','platforms':{'linux_s390x':{'url':'http://x','sha256':''}}}
open('$WORK/upd/noplat.json','w').write(json.dumps(m2))
"
curl -s -X PUT $B/settings/ -H "$AUTH" -d '{"update_manifest_url":"http://127.0.0.1:18334/plat.json"}' >/dev/null
check "多平台清单匹配本机" "True" "$(curl -s $B/update/check -H "$AUTH" | pyget "['has_update']")"
curl -s -X PUT $B/settings/ -H "$AUTH" -d '{"update_manifest_url":"http://127.0.0.1:18334/noplat.json"}' >/dev/null
check "无本机平台包 502" "502" "$(curl -s -o /dev/null -w '%{http_code}' $B/update/check -H "$AUTH")"
check "清单不可达 502" "502" "$(curl -s -X PUT $B/settings/ -H "$AUTH" -d '{"update_manifest_url":"http://127.0.0.1:9/x.json"}' >/dev/null; curl -s -o /dev/null -w '%{http_code}' $B/update/check -H "$AUTH")"
curl -s -X PUT $B/settings/ -H "$AUTH" -d '{"update_manifest_url":""}' >/dev/null
lsof -ti :18334 | xargs kill 2>/dev/null

echo "==> 10g. 安全：只读账号敏感信息屏蔽"
curl -s -X PUT $B/settings/ -H "$AUTH" -d '{"notify_webhook":"https://hooks.example.com/secret-abc","notify_secret":"SEC123"}' >/dev/null
curl -s -X POST $B/auth/viewer -H "$AUTH" -d '{"password":"view123"}' >/dev/null
VTOK=$(curl -s -X POST $B/setup/login -d '{"password":"view123"}' | pyget "['token']")
VAUTH="Authorization: Bearer $VTOK"
VROLE=$(curl -s -X POST $B/setup/login -d '{"password":"view123"}' | pyget "['role']")
check "只读登录成功" "viewer" "$VROLE"
VSET=$(curl -s $B/settings/ -H "$VAUTH")
check "viewer 看不到 webhook" "0" "$(echo $VSET | grep -c 'secret-abc')"
check "viewer 看不到加签密钥" "0" "$(echo $VSET | grep -c 'SEC123')"
ASET=$(curl -s $B/settings/ -H "$AUTH")
check "admin 看得到 webhook" "1" "$(echo $ASET | grep -c 'secret-abc')"
check "viewer 写操作 403" "403" "$(curl -s -o /dev/null -w '%{http_code}' -X PUT $B/settings/ -H "$VAUTH" -d '{"auto_scan":"0"}')"

echo "==> 10h. 安全：登录爆破锁定"
for i in 1 2 3 4 5; do curl -s -o /dev/null -X POST $B/setup/login -d '{"password":"wrong-pass"}'; done
check "第 6 次错误登录被锁定 429" "429" "$(curl -s -o /dev/null -w '%{http_code}' -X POST $B/setup/login -d '{"password":"wrong-pass"}')"
check "锁定期间正确密码也 429" "429" "$(curl -s -o /dev/null -w '%{http_code}' -X POST $B/setup/login -d '{"password":"admin123"}')"

echo "==> 11. 重置流程"
check "重置成功" "reset" "$(curl -s -X POST $B/setup/reset -H "$AUTH" | pyget "['status']")"
check "重置后未初始化" "false" "$(curl -s $B/setup/status | pyget "['initialized']" | tr 'A-Z' 'a-z')"
check "重置后 412" "412" "$(curl -s -o /dev/null -w '%{http_code}' $B/subnets/)"
TOKEN3=$(curl -s -X POST $B/setup/init -d '{"password":"round2"}' | pyget "['token']")
check "可重新初始化" "0" "$(curl -s $B/subnets/ -H "Authorization: Bearer $TOKEN3" | python3 -c 'import sys,json;print(len(json.load(sys.stdin)))')"

echo
echo "==================================="
echo "结果: $PASS 通过, $FAIL 失败"
kill $SRV 2>/dev/null
rm -rf "$WORK"
[[ $FAIL -eq 0 ]]
