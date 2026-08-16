<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api, token, type Overview } from '../api'

const baseURL = ref('')
const model = ref('')
const apiKey = ref('')
const msg = ref('')
const testing = ref(false)

// ---- 扫描计划 ----
const scanInterval = ref(5)
const autoScan = ref(true)
const scanMsg = ref('')
const scanSaving = ref(false)

// ---- 连通探测（断网续存） ----
const uplinkProbe = ref('223.5.5.5:53,114.114.114.114:53')
const uplinkCheckSec = ref(30)

// ---- 系统升级（OTA） ----
const appVersion = ref('')
const updateMsg = ref('')
const updateInfo = ref<{ latest: string; has_update: boolean; notes: string } | null>(null)
const checkingUpdate = ref(false)
const applying = ref(false)
const confirmUpgrade = ref(false)

async function checkUpdate() {
  updateMsg.value = ''; updateInfo.value = null; confirmUpgrade.value = false; checkingUpdate.value = true
  try {
    const r = await api.updateCheck()
    updateInfo.value = r
    if (!r.has_update) updateMsg.value = '✅ 当前已是最新版本'
  } catch (e: any) { updateMsg.value = '❌ 检查失败：' + e.message } finally { checkingUpdate.value = false }
}

async function applyUpdate() {
  confirmUpgrade.value = false
  applying.value = true; updateMsg.value = ''
  try {
    const r = await api.updateApply()
    updateMsg.value = `✅ 已升级到 ${r.version}，服务正在重启，约 5 秒后请重新登录…`
    setTimeout(() => { token.clear(); location.href = '/login' }, 6000)
  } catch (e: any) { updateMsg.value = '❌ 升级失败：' + e.message } finally { applying.value = false }
}

// ---- 台账字典（设备类型 / 负责人） ----
const devTypes = ref<string[]>([])
const owners = ref<string[]>([])
const newType = ref('')
const newOwner = ref('')
const dictMsg = ref('')
// 重命名状态：{kind, index, value}；null 表示未在编辑
const renaming = ref<{ kind: 'dev_types' | 'owners'; index: number; value: string } | null>(null)

function parseList(s: string | undefined): string[] {
  try { const a = JSON.parse(s || '[]'); return Array.isArray(a) ? a.filter(x => typeof x === 'string') : [] } catch { return [] }
}

function flashDict(t: string) { dictMsg.value = t; setTimeout(() => { if (dictMsg.value === t) dictMsg.value = '' }, 3000) }

function listOf(kind: 'dev_types' | 'owners') { return kind === 'dev_types' ? devTypes : owners }

// 添加 / 删除：即时保存
async function addItem(kind: 'dev_types' | 'owners', val: string) {
  const v = (val || '').trim()
  if (!v) return
  const list = listOf(kind)
  if (list.value.includes(v)) { flashDict('⚠ 已存在同名项'); return }
  list.value.push(v)
  if (kind === 'dev_types') newType.value = ''; else newOwner.value = ''
  await persistDict(kind, '已添加')
}

async function removeItem(kind: 'dev_types' | 'owners', i: number) {
  const list = listOf(kind)
  list.value.splice(i, 1)
  await persistDict(kind, '已删除（台账中已有记录保留原值）')
}

async function persistDict(kind: 'dev_types' | 'owners', action: string) {
  try {
    await api.saveSettings({ [kind]: JSON.stringify(listOf(kind).value) })
    flashDict(`✅ ${action}，即时生效`)
  } catch (e: any) { flashDict('❌ ' + e.message) }
}

// 重命名：调用级联接口，台账引用一并更新
function startRename(kind: 'dev_types' | 'owners', i: number) {
  renaming.value = { kind, index: i, value: listOf(kind).value[i] }
}

async function commitRename() {
  const r = renaming.value
  if (!r) return
  renaming.value = null
  const list = listOf(r.kind)
  const from = list.value[r.index]
  const to = r.value.trim()
  if (!to || to === from) return
  try {
    const { updated } = await api.renameDictItem({ kind: r.kind, from, to })
    list.value[r.index] = to
    flashDict(`✅ 已重命名「${from}」→「${to}」` + (updated > 0 ? `，同步更新 ${updated} 条台账记录` : ''))
  } catch (e: any) { flashDict('❌ ' + e.message) }
}

// ---- 告警通知 ----
const notifyEnabled = ref(false)
const notifyChannel = ref('webhook')
const notifyWebhook = ref('')
const notifySecret = ref('')
const notifyEvents = ref<string[]>(['conflict', 'offline'])
const notifyMsg = ref('')
const notifySaving = ref(false)
const notifyTesting = ref(false)

async function saveNotify() {
  notifyMsg.value = ''; notifySaving.value = true
  try {
    await api.saveSettings({
      notify_enabled: notifyEnabled.value ? '1' : '0',
      notify_channel: notifyChannel.value,
      notify_webhook: notifyWebhook.value.trim(),
      notify_secret: notifySecret.value.trim(),
      notify_events: notifyEvents.value.join(','),
    })
    notifyMsg.value = '✅ 已保存'
  } catch (e: any) { notifyMsg.value = '❌ ' + e.message } finally { notifySaving.value = false }
}

async function testNotify() {
  notifyMsg.value = ''; notifyTesting.value = true
  try {
    await saveNotifySilent()
    await api.notifyTest()
    notifyMsg.value = '✅ 测试消息已发送，请到对应群/接口确认'
  } catch (e: any) { notifyMsg.value = '❌ ' + e.message } finally { notifyTesting.value = false }
}

// 测试前先静默保存当前表单，确保测的是正在编辑的配置
async function saveNotifySilent() {
  await api.saveSettings({
    notify_enabled: notifyEnabled.value ? '1' : '0',
    notify_channel: notifyChannel.value,
    notify_webhook: notifyWebhook.value.trim(),
    notify_secret: notifySecret.value.trim(),
    notify_events: notifyEvents.value.join(','),
  })
}

function toggleEvent(ev: string) {
  const i = notifyEvents.value.indexOf(ev)
  if (i >= 0) notifyEvents.value.splice(i, 1); else notifyEvents.value.push(ev)
}

// ---- 只读账号 ----
const viewerEnabled = ref(false)
const viewerPwd = ref('')
const viewerMsg = ref('')
const viewerBusy = ref(false)

async function saveViewer(disable = false) {
  viewerMsg.value = ''; viewerBusy.value = true
  try {
    await api.setViewer(disable ? '' : viewerPwd.value)
    viewerEnabled.value = !disable
    viewerPwd.value = ''
    viewerMsg.value = disable ? '✅ 只读账号已停用' : '✅ 只读账号已生效，用该密码登录即为只读模式'
  } catch (e: any) { viewerMsg.value = '❌ ' + e.message } finally { viewerBusy.value = false }
}

// ---- 备份与恢复 ----
const backupMsg = ref('')
const exporting = ref(false)
const importBusy = ref(false)
const pendingImport = ref<File | null>(null)

async function doExport() {
  backupMsg.value = ''; exporting.value = true
  try {
    const res = await fetch('/api/v1/backup/export', { headers: { Authorization: `Bearer ${token.get()}` } })
    if (!res.ok) throw new Error(await res.text())
    const blob = await res.blob()
    const cd = res.headers.get('Content-Disposition') || ''
    const name = cd.match(/filename="?([^"]+)"?/)?.[1] || 'ipambox-backup.db'
    const a = document.createElement('a')
    a.href = URL.createObjectURL(blob)
    a.download = name
    a.click()
    URL.revokeObjectURL(a.href)
    backupMsg.value = `✅ 已导出 ${name}`
  } catch (e: any) { backupMsg.value = '❌ 导出失败：' + e.message } finally { exporting.value = false }
}

function pickImport(ev: Event) {
  const f = (ev.target as HTMLInputElement).files?.[0]
  if (f) pendingImport.value = f
  ;(ev.target as HTMLInputElement).value = ''
}

async function doImport() {
  if (!pendingImport.value) return
  importBusy.value = true; backupMsg.value = ''
  try {
    await api.backupImport(pendingImport.value)
    backupMsg.value = '✅ 恢复成功，3 秒后刷新页面…'
    setTimeout(() => location.reload(), 3000)
  } catch (e: any) {
    backupMsg.value = '❌ 恢复失败：' + e.message
  } finally { importBusy.value = false; pendingImport.value = null }
}

// ---- 重新初始化 ----
const overview = ref<Overview | null>(null)
const confirmReset = ref(false)
const resetting = ref(false)
const resetMsg = ref('')

onMounted(async () => {
  overview.value = await api.overview().catch(() => null)
  const v = await api.version().catch(() => null)
  if (v) appVersion.value = v.version
  const s = await api.getSettings().catch(() => null)
  if (s) {
    scanInterval.value = Number(s.scan_interval_min) || 5
    autoScan.value = s.auto_scan !== '0'
    uplinkProbe.value = s.uplink_probe || '223.5.5.5:53,114.114.114.114:53'
    uplinkCheckSec.value = Number(s.uplink_check_sec) || 30
    devTypes.value = parseList(s.dev_types)
    owners.value = parseList(s.owners)
    notifyEnabled.value = s.notify_enabled === '1'
    notifyChannel.value = s.notify_channel || 'webhook'
    notifyWebhook.value = s.notify_webhook || ''
    notifySecret.value = s.notify_secret || ''
    notifyEvents.value = (s.notify_events || 'conflict,offline').split(',').filter(Boolean)
  }
  const vs = await api.viewerStatus().catch(() => null)
  if (vs) viewerEnabled.value = vs.enabled
})

async function saveScan() {
  scanMsg.value = ''; scanSaving.value = true
  try {
    await api.saveSettings({
      scan_interval_min: String(scanInterval.value),
      auto_scan: autoScan.value ? '1' : '0',
      uplink_probe: uplinkProbe.value,
      uplink_check_sec: String(uplinkCheckSec.value),
    })
    scanMsg.value = '✅ 已保存，下一轮定时任务生效'
  } catch (e: any) { scanMsg.value = '❌ ' + e.message } finally { scanSaving.value = false }
}

async function doReset() {
  resetting.value = true
  resetMsg.value = ''
  try {
    await api.setupReset()
    token.clear()
    location.href = '/wizard'
  } catch (e: any) {
    resetMsg.value = '❌ 清除失败：' + e.message
    resetting.value = false
  }
}

async function save() {
  msg.value = ''
  try {
    await api.aiSaveConfig({ base_url: baseURL.value, model: model.value, api_key: apiKey.value })
    msg.value = '✅ 已保存'
  } catch (e: any) { msg.value = '❌ ' + e.message }
}

async function test() {
  msg.value = ''; testing.value = true
  try {
    const { reply } = await api.aiTest()
    msg.value = '✅ 连通成功，模型回复：' + reply
  } catch (e: any) { msg.value = '❌ ' + e.message } finally { testing.value = false }
}
</script>

<template>
  <div class="max-w-2xl animate-fade-in">
    <header class="mb-5">
      <h1 class="text-2xl font-bold text-slate-900 tracking-tight">设置</h1>
      <p class="text-sm text-slate-400 mt-1">扫描、通知与 AI 助手配置</p>
    </header>

    <div class="grid grid-cols-2 gap-4 mb-5">
      <section class="bg-white rounded-2xl shadow-card border border-slate-100 p-5">
        <span class="w-9 h-9 rounded-xl bg-brand-50 text-brand-600 flex items-center justify-center mb-3">
          <svg viewBox="0 0 24 24" class="w-5 h-5" fill="currentColor"><path d="M12 2a10 10 0 100 20 10 10 0 000-20zm1 10.6l4.2 2.4-.8 1.3L11 13.5V7h2v5.6z"/></svg>
        </span>
        <h2 class="font-semibold text-slate-800 mb-3">扫描计划</h2>
        <label class="flex items-center justify-between text-sm text-slate-600 mb-3 cursor-pointer">
          定时自动扫描
          <button @click="autoScan = !autoScan" type="button"
                  :class="['w-10 h-6 rounded-full transition relative', autoScan ? 'bg-brand-600' : 'bg-slate-200']">
            <i :class="['absolute top-1 w-4 h-4 rounded-full bg-white shadow transition-all', autoScan ? 'left-5' : 'left-1']"></i>
          </button>
        </label>
        <label class="block text-sm text-slate-600 mb-3">扫描间隔（分钟）
          <input v-model.number="scanInterval" type="number" min="1" max="1440" :disabled="!autoScan"
                 class="border border-slate-200 rounded-xl w-full px-3 py-2 mt-1.5 font-normal disabled:opacity-40" /></label>
        <div class="border-t border-slate-100 pt-3 mb-3">
          <p class="text-xs font-medium text-slate-500 mb-2">外网连通探测（断网续存）</p>
          <label class="block text-sm text-slate-600 mb-2">探测目标（host:port，逗号分隔）
            <input v-model="uplinkProbe" placeholder="223.5.5.5:53,114.114.114.114:53"
                   class="border border-slate-200 rounded-xl w-full px-3 py-2 mt-1 font-mono font-normal text-xs" /></label>
          <label class="block text-sm text-slate-600">探测间隔（秒）
            <input v-model.number="uplinkCheckSec" type="number" min="5" max="3600"
                   class="border border-slate-200 rounded-xl w-full px-3 py-2 mt-1.5 font-normal" /></label>
          <p class="text-[11px] text-slate-400 mt-1.5">离线期间扫描与台账照常记录，通知暂存队列、恢复后自动补发。</p>
        </div>
        <button @click="saveScan" :disabled="scanSaving"
                class="bg-brand-600 text-white rounded-xl px-4 py-2 text-sm font-medium hover:bg-brand-700 active:scale-95 disabled:opacity-50 transition">
          {{ scanSaving ? '保存中…' : '保存' }}
        </button>
        <p v-if="scanMsg" class="text-xs mt-2 text-slate-500">{{ scanMsg }}</p>
      </section>
      <section class="bg-white rounded-2xl shadow-card border border-slate-100 p-5">
        <span class="w-9 h-9 rounded-xl bg-purple-50 text-reserved flex items-center justify-center mb-3">
          <svg viewBox="0 0 24 24" class="w-5 h-5" fill="currentColor"><path d="M12 2a7 7 0 00-7 7v4l-2 3v1h18v-1l-2-3V9a7 7 0 00-7-7zm-2 16a2 2 0 104 0h-4z"/></svg>
        </span>
        <h2 class="font-semibold text-slate-800 mb-3">通知渠道</h2>
        <label class="flex items-center justify-between text-sm text-slate-600 mb-3 cursor-pointer">
          启用告警推送
          <button @click="notifyEnabled = !notifyEnabled" type="button"
                  :class="['w-10 h-6 rounded-full transition relative', notifyEnabled ? 'bg-brand-600' : 'bg-slate-200']">
            <i :class="['absolute top-1 w-4 h-4 rounded-full bg-white shadow transition-all', notifyEnabled ? 'left-5' : 'left-1']"></i>
          </button>
        </label>
        <div :class="[!notifyEnabled && 'opacity-40 pointer-events-none']">
          <label class="block text-sm text-slate-600 mb-2">渠道
            <select v-model="notifyChannel" class="border border-slate-200 rounded-xl w-full px-3 py-2 mt-1 font-normal bg-white">
              <option value="webhook">通用 Webhook（JSON）</option>
              <option value="dingtalk">钉钉群机器人</option>
              <option value="wecom">企业微信群机器人</option>
            </select></label>
          <label class="block text-sm text-slate-600 mb-2">Webhook 地址
            <input v-model="notifyWebhook" placeholder="https://..."
                   class="border border-slate-200 rounded-xl w-full px-3 py-2 mt-1 font-mono font-normal text-xs" /></label>
          <label v-if="notifyChannel === 'dingtalk'" class="block text-sm text-slate-600 mb-2">加签密钥（可选）
            <input v-model="notifySecret" placeholder="SEC 开头"
                   class="border border-slate-200 rounded-xl w-full px-3 py-2 mt-1 font-mono font-normal text-xs" /></label>
          <div class="flex items-center gap-3 text-sm text-slate-600 mb-3">
            <span class="text-slate-400 text-xs">推送事件：</span>
            <label class="flex items-center gap-1.5 cursor-pointer">
              <input type="checkbox" :checked="notifyEvents.includes('conflict')" @change="toggleEvent('conflict')" class="accent-brand-600" /> 地址冲突
            </label>
            <label class="flex items-center gap-1.5 cursor-pointer">
              <input type="checkbox" :checked="notifyEvents.includes('offline')" @change="toggleEvent('offline')" class="accent-brand-600" /> 设备离线
            </label>
          </div>
        </div>
        <div class="flex gap-2">
          <button @click="saveNotify" :disabled="notifySaving"
                  class="bg-brand-600 text-white rounded-xl px-4 py-2 text-sm font-medium hover:bg-brand-700 active:scale-95 disabled:opacity-50 transition">
            {{ notifySaving ? '保存中…' : '保存' }}
          </button>
          <button @click="testNotify" :disabled="notifyTesting || !notifyEnabled"
                  class="border border-slate-200 rounded-xl px-4 py-2 text-sm text-slate-600 hover:border-brand-400 hover:text-brand-600 disabled:opacity-50 active:scale-95 transition flex items-center gap-2">
            <svg v-if="notifyTesting" class="w-4 h-4 animate-spin" viewBox="0 0 24 24" fill="none"><circle cx="12" cy="12" r="9" stroke="currentColor" stroke-opacity=".25" stroke-width="3"/><path d="M21 12a9 9 0 00-9-9" stroke="currentColor" stroke-width="3" stroke-linecap="round"/></svg>
            {{ notifyTesting ? '发送中…' : '发送测试' }}
          </button>
        </div>
        <p v-if="notifyMsg" class="text-xs mt-2 text-slate-500 break-all">{{ notifyMsg }}</p>
      </section>
    </div>

    <!-- 台账字典 -->
    <div class="grid grid-cols-2 gap-4 mb-5">
      <section class="bg-white rounded-2xl shadow-card border border-slate-100 p-5">
        <span class="w-9 h-9 rounded-xl bg-sky-50 text-sky-600 flex items-center justify-center mb-3">
          <svg viewBox="0 0 24 24" class="w-5 h-5" fill="currentColor"><path d="M4 5h16a1 1 0 011 1v9a1 1 0 01-1 1h-7v2h3v2H8v-2h3v-2H4a1 1 0 01-1-1V6a1 1 0 011-1zm1 2v7h14V7H5z"/></svg>
        </span>
        <h2 class="font-semibold text-slate-800 mb-3">设备类型</h2>
        <div class="flex flex-wrap gap-1.5 mb-3">
          <template v-for="(t, i) in devTypes" :key="t">
            <input v-if="renaming && renaming.kind === 'dev_types' && renaming.index === i"
                   v-model="renaming.value" @keyup.enter="commitRename" @keyup.esc="renaming = null" @blur="commitRename"
                   class="border border-brand-300 rounded-full px-2.5 py-1 text-xs w-24 outline-none" />
            <span v-else @click="startRename('dev_types', i)" title="点击重命名"
                  class="bg-slate-100 hover:bg-brand-50 text-slate-600 text-xs rounded-full pl-2.5 pr-1.5 py-1 flex items-center gap-1 cursor-pointer transition">
              {{ t }}
              <button @click.stop="removeItem('dev_types', i)" class="text-slate-300 hover:text-conflict font-bold leading-none">×</button>
            </span>
          </template>
          <span v-if="!devTypes.length" class="text-xs text-slate-300">暂无，请在下方添加</span>
        </div>
        <div class="flex gap-2">
          <input v-model="newType" @keyup.enter="addItem('dev_types', newType)" placeholder="新类型，回车添加"
                 class="border border-slate-200 rounded-xl flex-1 px-3 py-1.5 text-sm" />
          <button @click="addItem('dev_types', newType)" class="border border-slate-200 rounded-xl px-3 text-sm text-slate-500 hover:border-brand-400 hover:text-brand-600 transition">添加</button>
        </div>
        <p class="text-xs text-slate-400 mt-2">点击标签可重命名（台账引用会同步更新）；增删即时生效。</p>
      </section>

      <section class="bg-white rounded-2xl shadow-card border border-slate-100 p-5">
        <span class="w-9 h-9 rounded-xl bg-amber-50 text-amber-600 flex items-center justify-center mb-3">
          <svg viewBox="0 0 24 24" class="w-5 h-5" fill="currentColor"><path d="M12 12a4 4 0 100-8 4 4 0 000 8zm0 2c-4 0-7 2-7 4v2h14v-2c0-2-3-4-7-4z"/></svg>
        </span>
        <h2 class="font-semibold text-slate-800 mb-3">负责人</h2>
        <div class="flex flex-wrap gap-1.5 mb-3">
          <template v-for="(o, i) in owners" :key="o">
            <input v-if="renaming && renaming.kind === 'owners' && renaming.index === i"
                   v-model="renaming.value" @keyup.enter="commitRename" @keyup.esc="renaming = null" @blur="commitRename"
                   class="border border-brand-300 rounded-full px-2.5 py-1 text-xs w-24 outline-none" />
            <span v-else @click="startRename('owners', i)" title="点击重命名"
                  class="bg-slate-100 hover:bg-brand-50 text-slate-600 text-xs rounded-full pl-2.5 pr-1.5 py-1 flex items-center gap-1 cursor-pointer transition">
              {{ o }}
              <button @click.stop="removeItem('owners', i)" class="text-slate-300 hover:text-conflict font-bold leading-none">×</button>
            </span>
          </template>
          <span v-if="!owners.length" class="text-xs text-slate-300">暂无，请在下方添加</span>
        </div>
        <div class="flex gap-2">
          <input v-model="newOwner" @keyup.enter="addItem('owners', newOwner)" placeholder="新负责人，回车添加"
                 class="border border-slate-200 rounded-xl flex-1 px-3 py-1.5 text-sm" />
          <button @click="addItem('owners', newOwner)" class="border border-slate-200 rounded-xl px-3 text-sm text-slate-500 hover:border-brand-400 hover:text-brand-600 transition">添加</button>
        </div>
        <p class="text-xs text-slate-400 mt-2">点击标签可重命名（台账引用会同步更新）；增删即时生效。</p>
      </section>
    </div>
    <p v-if="dictMsg" class="text-xs text-slate-500 mb-5 -mt-3">{{ dictMsg }}</p>

    <!-- 只读账号 -->
    <section class="bg-white rounded-2xl shadow-card border border-slate-100 p-6 mb-5">
      <div class="flex items-center gap-3 mb-1">
        <span class="w-9 h-9 rounded-xl bg-amber-50 text-amber-600 flex items-center justify-center">
          <svg viewBox="0 0 24 24" class="w-5 h-5" fill="currentColor"><path d="M12 4.5C7 4.5 2.7 7.6 1 12c1.7 4.4 6 7.5 11 7.5S21.3 16.4 23 12c-1.7-4.4-6-7.5-11-7.5zm0 12.5a5 5 0 110-10 5 5 0 010 10zm0-8a3 3 0 100 6 3 3 0 000-6z"/></svg>
        </span>
        <h2 class="font-semibold text-slate-800">只读账号</h2>
        <span v-if="viewerEnabled" class="text-xs bg-emerald-50 text-online rounded-full px-2 py-0.5 font-medium">已启用</span>
      </div>
      <p class="text-sm text-slate-400 mb-4">
        设置一个独立密码，分享给只需要查看的人（如领导、客户）。用该密码登录后只能浏览仪表盘 / IP 地图 / 台账 / 告警 / 报表，所有修改操作均被禁止。
      </p>
      <div class="flex flex-wrap gap-3 items-center">
        <input v-model="viewerPwd" type="password" placeholder="只读密码（至少 6 位）"
               class="border border-slate-200 rounded-xl px-3 py-2 text-sm w-56" />
        <button @click="saveViewer(false)" :disabled="viewerBusy || viewerPwd.length < 6"
                class="bg-brand-600 text-white rounded-xl px-4 py-2 text-sm font-medium hover:bg-brand-700 active:scale-95 disabled:opacity-50 transition">
          {{ viewerEnabled ? '重置密码' : '启用' }}
        </button>
        <button v-if="viewerEnabled" @click="saveViewer(true)" :disabled="viewerBusy"
                class="border border-red-200 text-conflict rounded-xl px-4 py-2 text-sm font-medium hover:bg-red-50 active:scale-95 disabled:opacity-50 transition">
          停用
        </button>
      </div>
      <p v-if="viewerMsg" class="text-xs mt-2 text-slate-500">{{ viewerMsg }}</p>
    </section>

    <!-- 备份与恢复 -->
    <section class="bg-white rounded-2xl shadow-card border border-slate-100 p-6 mb-5">
      <div class="flex items-center gap-3 mb-1">
        <span class="w-9 h-9 rounded-xl bg-emerald-50 text-online flex items-center justify-center">
          <svg viewBox="0 0 24 24" class="w-5 h-5" fill="currentColor"><path d="M12 3a5 5 0 015 5v1a4 4 0 010 8H7a5 5 0 01-.9-9.9A5 5 0 0112 3zm-1 6v5.6l-2.3-2.3-1.4 1.4 4.7 4.7 4.7-4.7-1.4-1.4-2.3 2.3V9h-2z"/></svg>
        </span>
        <h2 class="font-semibold text-slate-800">备份与恢复</h2>
      </div>
      <p class="text-sm text-slate-400 mb-4">备份包含全部子网、台账、告警与系统配置（含管理员密码）。低成本硬件 SD 卡易损坏，建议定期导出。</p>
      <div class="flex flex-wrap gap-3 items-center">
        <button @click="doExport" :disabled="exporting"
                class="bg-brand-600 text-white rounded-xl px-5 py-2 text-sm font-medium hover:bg-brand-700 active:scale-95 disabled:opacity-50 transition shadow-sm shadow-brand-600/30">
          {{ exporting ? '导出中…' : '导出备份' }}
        </button>
        <label class="border border-slate-200 rounded-xl px-5 py-2 text-sm text-slate-600 hover:border-brand-400 hover:text-brand-600 active:scale-95 cursor-pointer transition">
          导入备份…
          <input type="file" accept=".db" class="hidden" @change="pickImport" />
        </label>
      </div>

      <!-- 导入确认 -->
      <div v-if="pendingImport" class="mt-4 bg-amber-50 border border-amber-200 rounded-xl p-4 animate-fade-in">
        <p class="text-sm text-amber-700 font-medium mb-1">⚠ 确认用「{{ pendingImport.name }}」覆盖当前全部数据？</p>
        <p class="text-xs text-amber-600 mb-3">当前所有子网、台账、告警与配置将被备份文件替换（含管理员密码），此操作不可撤销。</p>
        <div class="flex gap-3">
          <button @click="doImport" :disabled="importBusy"
                  class="bg-amber-600 text-white rounded-xl px-4 py-1.5 text-sm font-medium hover:opacity-90 active:scale-95 disabled:opacity-50 transition">
            {{ importBusy ? '恢复中…' : '确认恢复' }}
          </button>
          <button @click="pendingImport = null" class="border border-slate-200 bg-white rounded-xl px-4 py-1.5 text-sm text-slate-500 hover:bg-slate-50 transition">取消</button>
        </div>
      </div>
      <p v-if="backupMsg" class="text-sm mt-3 text-slate-500 break-all">{{ backupMsg }}</p>
    </section>

    <section class="bg-white rounded-2xl shadow-card border border-slate-100 p-6 mb-5">
      <div class="flex items-center gap-3 mb-1">
        <span class="w-9 h-9 rounded-xl bg-gradient-to-br from-brand-500 to-reserved text-white flex items-center justify-center">✨</span>
        <h2 class="font-semibold text-slate-800">AI 助手</h2>
      </div>
      <p class="text-sm text-slate-400 mb-5">
        填写 OpenAI 兼容接口（如 DeepSeek <span class="font-mono text-slate-500">https://api.deepseek.com/v1</span>、
        本地 Ollama <span class="font-mono text-slate-500">http://127.0.0.1:11434/v1</span>）。
      </p>
      <label class="block text-sm font-medium text-slate-600 mb-3">Base URL
        <input v-model="baseURL" class="border border-slate-200 rounded-xl w-full px-3 py-2 mt-1.5 font-mono font-normal" placeholder="https://api.deepseek.com/v1" /></label>
      <label class="block text-sm font-medium text-slate-600 mb-3">模型名
        <input v-model="model" class="border border-slate-200 rounded-xl w-full px-3 py-2 mt-1.5 font-mono font-normal" placeholder="deepseek-chat / qwen2.5:3b" /></label>
      <label class="block text-sm font-medium text-slate-600 mb-5">API Key
        <input v-model="apiKey" type="password" class="border border-slate-200 rounded-xl w-full px-3 py-2 mt-1.5 font-mono font-normal" placeholder="本地 Ollama 可留空" /></label>
      <div class="flex gap-3">
        <button @click="save" class="bg-brand-600 text-white rounded-xl px-5 py-2 text-sm font-medium hover:bg-brand-700 active:scale-95 transition shadow-sm shadow-brand-600/30">保存</button>
        <button @click="test" :disabled="testing"
                class="border border-slate-200 rounded-xl px-5 py-2 text-sm text-slate-600 hover:border-brand-400 hover:text-brand-600 disabled:opacity-50 active:scale-95 transition flex items-center gap-2">
          <svg v-if="testing" class="w-4 h-4 animate-spin" viewBox="0 0 24 24" fill="none"><circle cx="12" cy="12" r="9" stroke="currentColor" stroke-opacity=".25" stroke-width="3"/><path d="M21 12a9 9 0 00-9-9" stroke="currentColor" stroke-width="3" stroke-linecap="round"/></svg>
          {{ testing ? '测试中…' : '测试连通' }}
        </button>
      </div>
      <p v-if="msg" class="text-sm mt-4 bg-slate-50 rounded-xl px-3 py-2 break-all">{{ msg }}</p>
    </section>

    <!-- 系统升级（OTA） -->
    <section class="bg-white rounded-2xl shadow-card border border-slate-100 p-6 mb-5">
      <div class="flex items-center gap-3 mb-3">
        <span class="w-9 h-9 rounded-xl bg-sky-50 text-sky-600 flex items-center justify-center">
          <svg viewBox="0 0 24 24" class="w-5 h-5" fill="currentColor"><path d="M12 3l4 4h-3v6h-2V7H8l4-4zM5 19h14a1 1 0 011 1v1H4v-1a1 1 0 011-1z" transform="rotate(180 12 12)"/><path d="M12 21a9 9 0 110-18 9 9 0 010 18zm0-2a7 7 0 100-14 7 7 0 000 14z" opacity="0"/></svg>
        </span>
        <div>
          <h2 class="font-semibold text-slate-800">系统升级</h2>
          <p class="text-xs text-slate-400">当前版本 <span class="font-mono">{{ appVersion || '…' }}</span> · 升级不中断数据，完成后自动重启</p>
        </div>
        <button @click="checkUpdate" :disabled="checkingUpdate"
                class="ml-auto border border-slate-200 rounded-xl px-4 py-2 text-sm text-slate-600 hover:border-brand-400 hover:text-brand-600 active:scale-95 disabled:opacity-50 transition">
          {{ checkingUpdate ? '检查中…' : '检查更新' }}
        </button>
      </div>
      <div v-if="updateInfo?.has_update" class="bg-brand-50 border border-brand-100 rounded-xl px-4 py-3 flex items-center gap-3">
        <div class="flex-1 min-w-0">
          <p class="text-sm font-medium text-slate-800">发现新版本 <span class="font-mono text-brand-600">{{ updateInfo.latest }}</span></p>
          <p v-if="updateInfo.notes" class="text-xs text-slate-500 mt-0.5 whitespace-pre-line">{{ updateInfo.notes }}</p>
        </div>
        <button v-if="!confirmUpgrade" @click="confirmUpgrade = true" :disabled="applying"
                class="bg-brand-600 text-white rounded-xl px-4 py-2 text-sm font-medium hover:bg-brand-700 active:scale-95 disabled:opacity-50 transition shrink-0">
          立即升级
        </button>
        <div v-else class="flex gap-2 shrink-0">
          <button @click="applyUpdate" :disabled="applying"
                  class="bg-conflict text-white rounded-xl px-4 py-2 text-sm font-medium hover:bg-red-600 active:scale-95 disabled:opacity-50 transition">
            {{ applying ? '升级中…' : '确认升级' }}
          </button>
          <button @click="confirmUpgrade = false" class="border border-slate-200 rounded-xl px-4 py-2 text-sm text-slate-600">取消</button>
        </div>
      </div>
      <p v-if="updateMsg" class="text-xs mt-3 text-slate-500 break-all">{{ updateMsg }}</p>
    </section>

    <!-- 重新初始化（危险区） -->
    <section class="bg-white rounded-2xl shadow-card p-6 border border-red-200">
      <h2 class="font-semibold text-conflict mb-1">重新初始化</h2>
      <p class="text-sm text-slate-400 mb-4">
        重新运行初始化向导（可重新选择本机网卡自动组网）。AI 助手配置会保留。
      </p>

      <button v-if="!confirmReset" @click="confirmReset = true"
              class="border border-red-300 text-conflict rounded-xl px-4 py-2 text-sm font-medium hover:bg-red-50 active:scale-95 transition">
        重新运行初始化向导…
      </button>

      <div v-else class="bg-red-50 rounded-xl p-4 animate-fade-in">
        <template v-if="overview && (overview.subnets > 0 || overview.stats.total > 0)">
          <p class="text-sm text-conflict font-medium mb-2">⚠ 当前已有数据：</p>
          <ul class="text-sm text-slate-600 mb-3 list-disc list-inside space-y-0.5">
            <li>{{ overview.subnets }} 个受管子网</li>
            <li>{{ overview.stats.total }} 条 IP 观测/标注记录</li>
            <li>{{ overview.unread_alerts }} 条未读告警</li>
          </ul>
          <p class="text-sm text-conflict mb-4">进入初始化前必须清除以上数据，此操作不可恢复。确定要继续吗？</p>
        </template>
        <p v-else class="text-sm text-slate-600 mb-4">当前没有业务数据，可以安全地重新初始化。</p>
        <div class="flex gap-3">
          <button @click="doReset" :disabled="resetting"
                  class="bg-conflict text-white rounded-xl px-4 py-2 text-sm font-medium hover:opacity-90 active:scale-95 disabled:opacity-50 transition">
            {{ resetting ? '正在清除…' : '确认清除并进入初始化' }}
          </button>
          <button @click="confirmReset = false" class="border border-slate-200 bg-white rounded-xl px-4 py-2 text-sm text-slate-500 hover:bg-slate-50 transition">取消</button>
        </div>
        <p v-if="resetMsg" class="text-sm mt-3">{{ resetMsg }}</p>
      </div>
    </section>
  </div>
</template>
