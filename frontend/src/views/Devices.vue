<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api, isViewer, type DeviceRow, type Subnet } from '../api'
import { t } from '../i18n'
import DictSelect from '../components/DictSelect.vue'

const devices = ref<DeviceRow[]>([])
const subnets = ref<Subnet[]>([])
const filter = ref('')
const statusFilter = ref('')
const loadError = ref('')
const notice = ref('')

// ---- 台账字典（设置页维护） ----
const dictTypes = ref<string[]>([])
const dictOwners = ref<string[]>([])
function parseList(s: string | undefined): string[] {
  try { const a = JSON.parse(s || '[]'); return Array.isArray(a) ? a.filter(x => typeof x === 'string') : [] } catch { return [] }
}

// ---- 编辑弹窗 ----
const editing = ref<DeviceRow | null>(null)
const eLabel = ref('')
const eOwner = ref('')
const eType = ref('')
const eErr = ref('')
const saving = ref(false)

// ---- 删除确认 ----
const deleting = ref<DeviceRow | null>(null)
const deleteBusy = ref(false)

// ---- 添加设备 ----
const showAdd = ref(false)
const aSubnet = ref(0)
const aIP = ref('')
const aLabel = ref('')
const aOwner = ref('')
const aType = ref('')
const aErr = ref('')
const addBusy = ref(false)

const statusKeys: Record<string, string> = {
  online: '在线', offline: '离线', free: '闲置', conflict: '冲突', reserved: '保留', rogue: '未授权',
}
const statusText = (s: string) => t(statusKeys[s] || s)
const badgeCls: Record<string, string> = {
  online: 'bg-emerald-50 text-online',
  offline: 'bg-slate-100 text-slate-500',
  conflict: 'bg-red-50 text-conflict',
  reserved: 'bg-purple-50 text-reserved',
  rogue: 'bg-orange-50 text-rogue',
  free: 'bg-slate-50 text-slate-400',
}
const dotCls: Record<string, string> = {
  online: 'bg-online', offline: 'bg-offline', conflict: 'bg-conflict',
  reserved: 'bg-reserved', rogue: 'bg-rogue', free: 'bg-slate-300',
}

const visible = computed(() => devices.value.filter(d => {
  if (statusFilter.value === 'unregistered') {
    if (!(d.status === 'online' && !d.label)) return false
  } else if (statusFilter.value && d.status !== statusFilter.value) {
    return false
  }
  const q = filter.value.trim()
  if (!q) return true
  return [d.ip, d.mac, d.hostname, d.label, d.owner].some(v => v?.includes(q))
}))

const unregisteredCount = computed(() => devices.value.filter(d => d.status === 'online' && !d.label).length)

function fmtTime(v?: string): string {
  if (!v) return ''
  const d = new Date(v)
  if (isNaN(d.getTime())) return v
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}

async function load() {
  loadError.value = ''
  try {
    devices.value = await api.devices()
  } catch (e: any) { loadError.value = t('加载失败：') + e.message }
}

function flash(m: string) {
  notice.value = m
  setTimeout(() => (notice.value = ''), 3000)
}

onMounted(async () => {
  load()
  subnets.value = await api.subnets().catch(() => [])
  const s = await api.getSettings().catch(() => null)
  if (s) {
    dictTypes.value = parseList(s.dev_types)
    dictOwners.value = parseList(s.owners)
  }
})

// ---- 编辑 ----
function openEdit(d: DeviceRow) {
  editing.value = d
  eLabel.value = d.label ?? ''
  eOwner.value = d.owner ?? ''
  eType.value = d.dev_type ?? ''
  eErr.value = ''
}

async function saveEdit() {
  if (!editing.value) return
  saving.value = true; eErr.value = ''
  try {
    await api.annotate(editing.value.id, { label: eLabel.value, owner: eOwner.value, dev_type: eType.value })
    editing.value = null
    flash(t('✅ 已保存'))
    await load()
  } catch (e: any) { eErr.value = e.message } finally { saving.value = false }
}

// ---- 删除 ----
async function doDelete() {
  if (!deleting.value) return
  deleteBusy.value = true
  try {
    await api.deleteAddress(deleting.value.id)
    flash(t('✅ 已删除 {ip} 的台账记录', { ip: deleting.value.ip }))
    deleting.value = null
    await load()
  } catch (e: any) { flash(t('❌ 删除失败：') + e.message) } finally { deleteBusy.value = false }
}

// ---- 添加 ----
function openAdd() {
  aSubnet.value = subnets.value[0]?.id ?? 0
  aIP.value = ''; aLabel.value = ''; aOwner.value = ''; aType.value = ''; aErr.value = ''
  showAdd.value = true
}

async function doAdd() {
  aErr.value = ''
  if (!aSubnet.value) { aErr.value = t('请先在「子网管理」中添加子网'); return }
  if (!/^\d+\.\d+\.\d+\.\d+$/.test(aIP.value)) { aErr.value = t('请填写合法的 IPv4 地址'); return }
  addBusy.value = true
  try {
    await api.createAddress(aSubnet.value, {
      ip: aIP.value, status: 'reserved', label: aLabel.value, owner: aOwner.value, dev_type: aType.value,
    })
    showAdd.value = false
    flash(t('✅ 已登记 {ip}（保留）', { ip: aIP.value }))
    await load()
  } catch (e: any) { aErr.value = e.message } finally { addBusy.value = false }
}
</script>

<template>
  <div class="animate-fade-in">
    <header class="mb-5 flex flex-wrap items-end justify-between gap-3">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 tracking-tight">{{ t('设备台账') }}</h1>
        <p class="text-sm text-slate-400 mt-1">{{ t('全网已观测设备的清单与归属，可编辑、登记保留地址') }}</p>
      </div>
      <div class="flex items-center gap-3">
        <span class="text-sm text-slate-400">
          {{ t('共') }} <span class="font-semibold text-slate-600 tabular-nums">{{ visible.length }}</span> {{ t('条') }}
          <span v-if="unregisteredCount" class="text-rogue font-medium">{{ t('（{n} 台未登记）', { n: unregisteredCount }) }}</span>
        </span>
        <button v-if="!isViewer()" @click="openAdd"
                class="bg-brand-600 text-white rounded-xl px-4 py-2 text-sm font-medium hover:bg-brand-700 active:scale-95 transition shadow-sm shadow-brand-600/30">
          + {{ t('登记地址') }}
        </button>
      </div>
    </header>

    <div class="bg-white rounded-2xl shadow-card border border-slate-100 p-3 mb-4 flex flex-wrap items-center gap-3">
      <div class="relative flex-1 max-w-sm">
        <svg viewBox="0 0 24 24" class="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 text-slate-400" fill="currentColor"><path d="M10 2a8 8 0 105.3 14l4.4 4.3 1.4-1.4-4.3-4.4A8 8 0 0010 2zm0 2a6 6 0 110 12 6 6 0 010-12z"/></svg>
        <input v-model="filter" :placeholder="t('搜索 IP / MAC / 主机名 / 标注 / 负责人…')"
               class="border border-slate-200 rounded-xl pl-9 pr-3 py-2 w-full text-sm" />
      </div>
      <select v-model="statusFilter" class="border border-slate-200 rounded-xl px-3 py-2 text-sm bg-white cursor-pointer">
        <option value="">{{ t('全部状态') }}</option>
        <option value="unregistered">⚠ {{ t('未登记设备') }}</option>
        <option value="online">{{ t('在线') }}</option>
        <option value="offline">{{ t('离线') }}</option>
        <option value="conflict">{{ t('冲突') }}</option>
        <option value="reserved">{{ t('保留') }}</option>
      </select>
    </div>

    <div v-if="loadError" class="bg-red-50 border border-red-200 text-conflict rounded-xl px-4 py-2.5 mb-4 text-sm">{{ loadError }}</div>
    <div v-if="notice" class="bg-emerald-50 border border-emerald-200 text-online rounded-xl px-4 py-2.5 mb-4 text-sm animate-fade-in">{{ notice }}</div>

    <div class="bg-white rounded-2xl shadow-card border border-slate-100 overflow-x-auto">
      <table class="w-full text-sm">
        <thead>
          <tr class="text-left text-xs uppercase tracking-wider text-slate-400 border-b border-slate-100 bg-slate-50/60">
            <th class="px-5 py-3 font-medium">IP</th><th class="px-4 py-3 font-medium">{{ t('状态') }}</th><th class="px-4 py-3 font-medium">MAC</th>
            <th class="px-4 py-3 font-medium">{{ t('主机名') }}</th><th class="px-4 py-3 font-medium">{{ t('标注') }}</th><th class="px-4 py-3 font-medium">{{ t('负责人') }}</th>
            <th class="px-4 py-3 font-medium">{{ t('类型') }}</th><th class="px-4 py-3 font-medium">{{ t('所属子网') }}</th><th class="px-4 py-3 font-medium">{{ t('最后在线') }}</th>
            <th class="px-4 py-3 font-medium text-right">{{ t('操作') }}</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-slate-50">
          <tr v-for="(d, i) in visible" :key="d.id" :class="['hover:bg-brand-50/40 transition-colors', i % 2 ? 'bg-slate-50/40' : '']">
            <td class="px-5 py-3 font-mono text-slate-800">{{ d.ip }}</td>
            <td class="px-4 py-3">
              <span :class="['inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-medium', badgeCls[d.status] || badgeCls.free]">
                <i :class="['w-1.5 h-1.5 rounded-full', dotCls[d.status] || dotCls.free]"></i>
                {{ statusText(d.status) }}
              </span>
            </td>
            <td class="px-4 py-3 font-mono text-xs text-slate-500">
              {{ d.mac || '—' }}
              <div v-if="d.vendor" class="font-sans text-xs text-slate-400 mt-0.5">{{ d.vendor }}</div>
            </td>
            <td class="px-4 py-3 text-slate-600">{{ d.hostname || '—' }}</td>
            <td class="px-4 py-3">
              <span v-if="d.label" class="text-slate-700">{{ d.label }}</span>
              <span v-else-if="d.status === 'online'" class="bg-orange-50 text-rogue text-xs rounded-full px-2 py-0.5 font-medium">{{ t('未登记') }}</span>
              <span v-else class="text-slate-300">—</span>
            </td>
            <td class="px-4 py-3 text-slate-600">{{ d.owner || '—' }}</td>
            <td class="px-4 py-3 text-slate-600">{{ d.dev_type || '—' }}</td>
            <td class="px-4 py-3 text-slate-500">{{ d.subnet_name || d.subnet_cidr }}</td>
            <td class="px-4 py-3 text-slate-400 text-xs">{{ fmtTime(d.last_seen) || '—' }}</td>
            <td class="px-4 py-3 text-right whitespace-nowrap">
              <button v-if="!isViewer()" @click="openEdit(d)" class="text-brand-600 hover:text-brand-700 text-xs font-medium mr-3">{{ t('编辑') }}</button>
              <button v-if="!isViewer()" @click="deleting = d" class="text-conflict hover:opacity-80 text-xs font-medium">{{ t('删除') }}</button>
            </td>
          </tr>
          <tr v-if="!visible.length">
            <td colspan="10" class="px-4 py-16 text-center text-slate-400">{{ t('暂无设备记录') }}</td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- 编辑弹窗 -->
    <Teleport to="body">
      <div v-if="editing" class="fixed inset-0 bg-slate-900/30 backdrop-blur-[2px] z-40 flex items-center justify-center p-6"
           @click.self="editing = null">
        <div class="bg-white rounded-2xl shadow-pop w-full max-w-md p-6 animate-fade-in">
          <h3 class="font-semibold text-slate-800 mb-1">{{ t('编辑') }} <span class="font-mono">{{ editing.ip }}</span></h3>
          <p class="text-xs text-slate-400 mb-4">{{ editing.subnet_name || editing.subnet_cidr }} · {{ editing.mac || t('无 MAC') }}</p>
          <label class="block text-sm font-medium text-slate-600 mb-3">{{ t('标注') }}
            <input v-model="eLabel" :placeholder="t('如：张三的笔记本')" class="border border-slate-200 rounded-xl w-full px-3 py-2 mt-1.5 font-normal" /></label>
          <label class="block text-sm font-medium text-slate-600 mb-3">{{ t('负责人') }}
            <DictSelect v-model="eOwner" :options="dictOwners" /></label>
          <label class="block text-sm font-medium text-slate-600 mb-4">{{ t('类型') }}
            <DictSelect v-model="eType" :options="dictTypes" /></label>
          <p v-if="eErr" class="text-conflict text-sm mb-3">{{ eErr }}</p>
          <div class="flex gap-3">
            <button @click="saveEdit" :disabled="saving"
                    class="bg-brand-600 text-white rounded-xl px-5 py-2 flex-1 text-sm font-medium hover:bg-brand-700 active:scale-[0.98] disabled:opacity-50 transition">
              {{ saving ? t('保存中…') : t('保存') }}
            </button>
            <button @click="editing = null" class="border border-slate-200 rounded-xl px-5 py-2 text-sm text-slate-500 hover:bg-slate-50 transition">{{ t('取消') }}</button>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- 删除确认 -->
    <Teleport to="body">
      <div v-if="deleting" class="fixed inset-0 bg-slate-900/30 backdrop-blur-[2px] z-40 flex items-center justify-center p-6"
           @click.self="deleting = null">
        <div class="bg-white rounded-2xl shadow-pop w-full max-w-sm p-6 animate-fade-in">
          <h3 class="font-semibold text-conflict mb-2">{{ t('删除 {ip} 的台账记录？', { ip: deleting.ip }) }}</h3>
          <p class="text-sm text-slate-500 mb-5">{{ t('仅删除台账与标注；若设备仍在线，下次扫描会重新出现（无标注）。') }}</p>
          <div class="flex gap-3">
            <button @click="doDelete" :disabled="deleteBusy"
                    class="bg-conflict text-white rounded-xl px-5 py-2 flex-1 text-sm font-medium hover:opacity-90 active:scale-[0.98] disabled:opacity-50 transition">
              {{ deleteBusy ? t('删除中…') : t('确认删除') }}
            </button>
            <button @click="deleting = null" class="border border-slate-200 rounded-xl px-5 py-2 text-sm text-slate-500 hover:bg-slate-50 transition">{{ t('取消') }}</button>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- 登记地址弹窗 -->
    <Teleport to="body">
      <div v-if="showAdd" class="fixed inset-0 bg-slate-900/30 backdrop-blur-[2px] z-40 flex items-center justify-center p-6"
           @click.self="showAdd = false">
        <div class="bg-white rounded-2xl shadow-pop w-full max-w-md p-6 animate-fade-in">
          <h3 class="font-semibold text-slate-800 mb-1">{{ t('登记地址（保留）') }}</h3>
          <p class="text-xs text-slate-400 mb-4">{{ t('用于网关、规划预留等尚未在线的地址；登记后状态为「保留」，不会被扫描判定为闲置。') }}</p>
          <label class="block text-sm font-medium text-slate-600 mb-3">{{ t('所属子网') }}
            <select v-model.number="aSubnet" class="border border-slate-200 rounded-xl w-full px-3 py-2 mt-1.5 font-normal bg-white font-mono">
              <option v-for="s in subnets" :key="s.id" :value="s.id">{{ s.cidr }}　{{ s.name }}</option>
            </select></label>
          <label class="block text-sm font-medium text-slate-600 mb-3">{{ t('IP 地址') }}
            <input v-model="aIP" :placeholder="t('如：192.168.1.254')" class="border border-slate-200 rounded-xl w-full px-3 py-2 mt-1.5 font-mono font-normal" /></label>
          <label class="block text-sm font-medium text-slate-600 mb-3">{{ t('标注') }}
            <input v-model="aLabel" :placeholder="t('如：核心网关')" class="border border-slate-200 rounded-xl w-full px-3 py-2 mt-1.5 font-normal" /></label>
          <div class="grid grid-cols-2 gap-3 mb-4">
            <label class="block text-sm font-medium text-slate-600">{{ t('负责人') }}
              <DictSelect v-model="aOwner" :options="dictOwners" /></label>
            <label class="block text-sm font-medium text-slate-600">{{ t('类型') }}
              <DictSelect v-model="aType" :options="dictTypes" /></label>
          </div>
          <p v-if="aErr" class="text-conflict text-sm mb-3">{{ aErr }}</p>
          <div class="flex gap-3">
            <button @click="doAdd" :disabled="addBusy"
                    class="bg-brand-600 text-white rounded-xl px-5 py-2 flex-1 text-sm font-medium hover:bg-brand-700 active:scale-[0.98] disabled:opacity-50 transition">
              {{ addBusy ? t('提交中…') : t('登记') }}
            </button>
            <button @click="showAdd = false" class="border border-slate-200 rounded-xl px-5 py-2 text-sm text-slate-500 hover:bg-slate-50 transition">{{ t('取消') }}</button>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>
