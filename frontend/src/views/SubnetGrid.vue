<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api, isViewer, type IPAddress, type Subnet, type NICInfo } from '../api'
import DictSelect from '../components/DictSelect.vue'

const route = useRoute()
const router = useRouter()
const subnets = ref<Subnet[]>([])
const addresses = ref<IPAddress[]>([])
const selected = ref<IPAddress | null>(null)
const filter = ref('')
const scanning = ref(false)
const error = ref('')
const notice = ref('')

// 台账字典（设置页维护）：详情抽屉的类型下拉 / 负责人补全
const dictTypes = ref<string[]>([])
const dictOwners = ref<string[]>([])
function parseList(s: string | undefined): string[] {
  try { const a = JSON.parse(s || '[]'); return Array.isArray(a) ? a.filter(x => typeof x === 'string') : [] } catch { return [] }
}

// 添加子网
const showAdd = ref(false)
const newCIDR = ref('')
const newName = ref('')
const newIface = ref('')
const nics = ref<NICInfo[]>([])

let poller: ReturnType<typeof setInterval> | null = null

const colors: Record<string, string> = {
  online: 'bg-online shadow-[0_0_6px_rgb(16_185_129/0.5)]', offline: 'bg-offline',
  free: 'bg-white border border-slate-200',
  conflict: 'bg-conflict animate-pulse', reserved: 'bg-reserved', rogue: 'bg-rogue',
}
const statusText: Record<string, string> = {
  online: '在线', offline: '离线', free: '闲置', conflict: '冲突', reserved: '保留', rogue: '未授权',
}

// fmtTime 把 ISO 时间（2026-08-11T12:36:24Z）格式化为本地可读时间
function fmtTime(v?: string): string {
  if (!v) return ''
  const d = new Date(v)
  if (isNaN(d.getTime())) return v
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`
}
const legend = [
  { key: 'online', label: '在线', dot: 'bg-online' },
  { key: 'offline', label: '离线', dot: 'bg-offline' },
  { key: 'free', label: '闲置', dot: 'bg-white border border-slate-300' },
  { key: 'conflict', label: '冲突', dot: 'bg-conflict' },
  { key: 'reserved', label: '保留', dot: 'bg-reserved' },
  { key: 'rogue', label: '未授权', dot: 'bg-rogue' },
]

const subnetId = computed(() => Number(route.params.id || subnets.value[0]?.id || 0))
const currentSubnet = computed(() => subnets.value.find(s => s.id === subnetId.value))

/** 展开 CIDR 全部主机地址（仅 /22 及更小网段做全量渲染，大网段只显示已观测）。 */
function expandCIDR(cidr: string, maxHosts = 1022): string[] | null {
  const m = cidr.match(/^(\d+)\.(\d+)\.(\d+)\.(\d+)\/(\d+)$/)
  if (!m) return null
  const prefix = Number(m[5])
  const hostBits = 32 - prefix
  const total = Math.pow(2, hostBits)
  if (total - 2 > maxHosts) return null
  const base = (Number(m[1]) << 24) + (Number(m[2]) << 16) + (Number(m[3]) << 8) + Number(m[4])
  const toIP = (n: number) => [(n >>> 24) & 255, (n >>> 16) & 255, (n >>> 8) & 255, n & 255].join('.')
  const out: string[] = []
  const start = hostBits > 1 ? 1 : 0 // 跳过网络地址
  const end = hostBits > 1 ? total - 1 : total // 跳过广播地址
  for (let i = start; i < end; i++) out.push(toIP(base + i))
  return out
}

/** 网格单元：全量网段地址 + 已观测记录合并。 */
const gridCells = computed(() => {
  const cidr = currentSubnet.value?.cidr
  const all = cidr ? expandCIDR(cidr) : null
  if (!all) return addresses.value // 大网段降级为仅已观测
  const byIP = new Map(addresses.value.map(a => [a.ip, a]))
  return all.map(ip => byIP.get(ip) ?? ({ ip, status: 'free' } as IPAddress))
})

const isFullGrid = computed(() => !!currentSubnet.value?.cidr && !!expandCIDR(currentSubnet.value.cidr))

const visible = computed(() =>
  filter.value
    ? gridCells.value.filter(a => a.ip.includes(filter.value) || a.label?.includes(filter.value) || a.mac?.includes(filter.value))
    : gridCells.value,
)

// 网格 / 列表视图切换；列表只显示非闲置地址
const viewMode = ref<'grid' | 'list'>('grid')
const listRows = computed(() => visible.value.filter(a => a.status !== 'free'))

async function load() {
  if (!subnetId.value) { addresses.value = []; return }
  addresses.value = await api.addresses(subnetId.value).catch(() => [])
}

async function scan() {
  error.value = ''; notice.value = ''
  if (!subnetId.value) {
    error.value = '请先添加一个子网，再开始扫描'
    showAdd.value = true
    return
  }
  scanning.value = true
  notice.value = '扫描已启动，正在发现设备…'
  try {
    await api.scanNow(subnetId.value)
  } catch (e: any) {
    scanning.value = false
    error.value = '扫描启动失败：' + e.message + '（请确认后端服务已启动）'
    return
  }
  let n = 0
  poller = setInterval(async () => {
    await load()
    if (++n >= 10) {
      scanning.value = false
      poller && clearInterval(poller)
      notice.value = `扫描完成，发现 ${addresses.value.filter(a => a.status === 'online').length} 台在线设备`
      setTimeout(() => (notice.value = ''), 5000)
    }
  }, 2000)
}

async function addSubnet() {
  error.value = ''
  if (!newCIDR.value.includes('/')) { error.value = '请输入 CIDR 格式，如 192.168.1.0/24'; return }
  try {
    const sub = await api.createSubnet({ cidr: newCIDR.value, name: newName.value || newCIDR.value, iface: newIface.value })
    subnets.value.push(sub)
    showAdd.value = false
    newCIDR.value = ''; newName.value = ''; newIface.value = ''
    router.push(`/subnets/${sub.id}`)
  } catch (e: any) { error.value = e.message }
}

function pickNIC(nic: NICInfo) {
  newCIDR.value = nic.cidr
  newIface.value = nic.name
  if (!newName.value) newName.value = `${nic.name} 网段`
}

async function save() {
  if (!selected.value) return
  try {
    await api.annotate(selected.value.id, {
      label: selected.value.label ?? '', owner: selected.value.owner ?? '', dev_type: selected.value.dev_type ?? '',
    })
    notice.value = '已保存'
    setTimeout(() => (notice.value = ''), 2000)
  } catch (e: any) { error.value = e.message }
}

watch(subnetId, load)
onMounted(async () => {
  subnets.value = await api.subnets().catch(() => [])
  nics.value = await api.interfaces().catch(() => [])
  const s = await api.getSettings().catch(() => null)
  if (s) {
    dictTypes.value = parseList(s.dev_types)
    dictOwners.value = parseList(s.owners)
  }
  if (!route.params.id && subnets.value[0]) router.replace(`/subnets/${subnets.value[0].id}`)
  await load()
})
onUnmounted(() => poller && clearInterval(poller))
</script>

<template>
  <div class="animate-fade-in">
    <header class="mb-5">
      <h1 class="text-2xl font-bold text-slate-900 tracking-tight">IP 地图</h1>
      <p class="text-sm text-slate-400 mt-1">以网格俯瞰网段内每一个地址的状态</p>
    </header>

    <!-- 工具栏 -->
    <div class="bg-white rounded-2xl shadow-card border border-slate-100 p-3 mb-4 flex flex-wrap items-center gap-3">
      <label v-if="subnets.length" class="flex items-center gap-2">
        <span class="text-sm text-slate-400 whitespace-nowrap">选择子网：</span>
        <select :value="subnetId"
                @change="router.push(`/subnets/${($event.target as HTMLSelectElement).value}`)"
                class="border border-slate-200 rounded-xl px-3 py-2 font-mono text-sm bg-white cursor-pointer hover:border-slate-300">
          <option v-for="s in subnets" :key="s.id" :value="s.id">{{ s.cidr }}　{{ s.name }}{{ s.iface ? `　[${s.iface}]` : '' }}</option>
        </select>
      </label>
      <div class="relative flex-1 max-w-xs">
        <svg viewBox="0 0 24 24" class="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 text-slate-400" fill="currentColor"><path d="M10 2a8 8 0 105.3 14l4.4 4.3 1.4-1.4-4.3-4.4A8 8 0 0010 2zm0 2a6 6 0 110 12 6 6 0 010-12z"/></svg>
        <input v-model="filter" placeholder="搜索 IP / 标注 / MAC…"
               class="border border-slate-200 rounded-xl pl-9 pr-3 py-2 w-full text-sm" />
      </div>
      <div class="flex bg-slate-100 rounded-xl p-0.5">
        <button @click="viewMode = 'grid'"
                :class="['px-3 py-1.5 text-xs font-medium rounded-[10px] transition', viewMode === 'grid' ? 'bg-white text-slate-800 shadow-sm' : 'text-slate-400 hover:text-slate-600']">
          ▦ 网格
        </button>
        <button @click="viewMode = 'list'"
                :class="['px-3 py-1.5 text-xs font-medium rounded-[10px] transition', viewMode === 'list' ? 'bg-white text-slate-800 shadow-sm' : 'text-slate-400 hover:text-slate-600']">
          ☰ 列表
        </button>
      </div>
      <button v-if="!isViewer()" @click="scan" :disabled="scanning"
              class="ml-auto bg-brand-600 text-white rounded-xl px-5 py-2 text-sm font-medium hover:bg-brand-700 active:scale-95 disabled:opacity-50 disabled:active:scale-100 shadow-sm shadow-brand-600/30 transition flex items-center gap-2">
        <svg v-if="scanning" class="w-4 h-4 animate-spin" viewBox="0 0 24 24" fill="none"><circle cx="12" cy="12" r="9" stroke="currentColor" stroke-opacity=".25" stroke-width="3"/><path d="M21 12a9 9 0 00-9-9" stroke="currentColor" stroke-width="3" stroke-linecap="round"/></svg>
        <span v-else>⚡</span>
        {{ scanning ? '扫描中…' : '立即扫描' }}
      </button>
      <button v-if="!isViewer()" @click="showAdd = !showAdd"
              class="border border-brand-200 text-brand-600 rounded-xl px-4 py-2 text-sm font-medium hover:bg-brand-50 active:scale-95 transition">
        + 添加子网
      </button>
    </div>

    <!-- 添加子网表单 -->
    <div v-if="showAdd" class="bg-brand-50/60 border border-brand-100 rounded-2xl p-4 mb-4 animate-fade-in">
      <!-- 本机物理网卡 -->
      <div v-if="nics.length" class="mb-3">
        <p class="text-xs text-slate-500 mb-2">本机物理网卡（点选自动填充网段并绑定接入网卡）：</p>
        <div class="flex flex-wrap gap-2">
          <button v-for="nic in nics" :key="nic.name + nic.ip" @click="pickNIC(nic)"
                  :class="['border rounded-xl px-3 py-2 text-left text-sm transition active:scale-95',
                           newIface === nic.name ? 'border-brand-600 bg-white ring-1 ring-brand-600' : 'border-slate-200 bg-white hover:border-slate-400']">
            <span class="font-mono font-medium">{{ nic.name }}</span>
            <span class="font-mono text-slate-400 ml-2">{{ nic.ip }}/{{ nic.prefix }}</span>
            <span class="block text-xs text-slate-400">子网 <span class="font-mono text-brand-600">{{ nic.cidr }}</span></span>
          </button>
        </div>
      </div>
      <div class="flex flex-wrap items-center gap-3">
        <input v-model="newCIDR" placeholder="CIDR，如 192.168.1.0/24" class="border border-slate-200 rounded-xl px-3 py-2 font-mono text-sm w-60 bg-white" />
        <input v-model="newName" placeholder="名称（可选），如 办公网" class="border border-slate-200 rounded-xl px-3 py-2 text-sm w-52 bg-white" />
        <select v-model="newIface" class="border border-slate-200 rounded-xl px-3 py-2 text-sm bg-white font-mono">
          <option value="">接入网卡（可选）</option>
          <option v-for="nic in nics" :key="nic.name" :value="nic.name">{{ nic.name }}</option>
        </select>
        <button @click="addSubnet" class="bg-brand-600 text-white rounded-xl px-4 py-2 text-sm font-medium hover:bg-brand-700 active:scale-95 transition">保存</button>
        <span class="text-sm text-slate-500">添加后点击「立即扫描」即可发现该网段内的设备</span>
      </div>
    </div>

    <!-- 提示条 -->
    <div v-if="error" class="bg-red-50 border border-red-200 text-conflict rounded-xl px-4 py-2.5 mb-4 text-sm animate-fade-in">{{ error }}</div>
    <div v-if="notice" class="bg-emerald-50 border border-emerald-200 text-online rounded-xl px-4 py-2.5 mb-4 text-sm animate-fade-in">{{ notice }}</div>

    <!-- 图例胶囊 + 计数 -->
    <div class="flex flex-wrap items-center gap-2 mb-3">
      <span v-for="l in legend" :key="l.key"
            class="inline-flex items-center gap-1.5 bg-white border border-slate-200 rounded-full px-2.5 py-1 text-xs text-slate-500 shadow-sm">
        <i :class="['w-2.5 h-2.5 rounded-full', l.dot]"></i>{{ l.label }}
      </span>
      <span class="ml-auto text-xs text-slate-400 font-mono">{{ currentSubnet?.cidr || '' }} ·
        {{ isFullGrid ? `全量 ${gridCells.length} 格 · 已观测 ${addresses.length}` : `大网段模式 · 已观测 ${addresses.length} 条` }}</span>
    </div>

    <!-- IP 网格 -->
    <div v-if="viewMode === 'grid'" class="bg-white rounded-2xl shadow-card border border-slate-100 p-5">
      <div v-if="!subnets.length" class="text-center py-24">
        <div class="w-20 h-20 mx-auto mb-5 rounded-3xl bg-gradient-to-br from-brand-100 to-purple-100 flex items-center justify-center text-4xl">🌐</div>
        <p class="text-slate-700 font-medium mb-1">还没有受管子网</p>
        <p class="text-slate-400 text-sm mb-5">添加第一个网段，开始绘制你的 IP 地图</p>
        <button @click="showAdd = true" class="bg-brand-600 text-white rounded-xl px-5 py-2.5 text-sm font-medium hover:bg-brand-700 active:scale-95 transition shadow-sm shadow-brand-600/30">添加第一个子网</button>
      </div>
      <div v-else-if="!visible.length" class="text-center py-24">
        <div class="w-20 h-20 mx-auto mb-5 rounded-3xl bg-slate-100 flex items-center justify-center text-4xl">📡</div>
        <p class="text-slate-700 font-medium mb-1">还没有观测数据</p>
        <p class="text-slate-400 text-sm">点击右上角「⚡ 立即扫描」开始发现设备</p>
      </div>
      <div v-else class="grid gap-[2px]" style="grid-template-columns: repeat(32, minmax(0,1fr))">
        <button
          v-for="a in visible" :key="a.ip"
          :class="['aspect-square rounded-[3px] hover:scale-[1.6] hover:rounded-md hover:z-10 relative transition-all duration-150 group',
                   'font-mono text-[8px] leading-none flex items-end justify-end p-[2px] select-none',
                   colors[a.status] || colors.free,
                   a.status === 'free' ? 'text-slate-400' : 'text-white/90',
                   selected?.ip === a.ip ? 'ring-2 ring-brand-600 ring-offset-1 scale-[1.3] z-10' : '']"
          @click="selected = a"
        >{{ a.ip.split('.').pop() }}<span
            class="pointer-events-none absolute bottom-full left-1/2 -translate-x-1/2 mb-1 z-50 whitespace-nowrap
                   bg-ink-800 text-white text-[10px] leading-tight font-medium font-sans rounded-md px-1.5 py-1 shadow-pop
                   opacity-0 -translate-y-0.5 group-hover:opacity-100 group-hover:translate-y-0 transition-all duration-150">
            {{ a.ip }}　{{ statusText[a.status] || a.status }}<template v-if="a.label">　{{ a.label }}</template>
            <i class="absolute left-1/2 -translate-x-1/2 top-full border-[3px] border-transparent border-t-ink-800"></i>
          </span></button>
      </div>
    </div>

    <!-- 列表模式：只显示非闲置地址 -->
    <div v-else class="bg-white rounded-2xl shadow-card border border-slate-100 overflow-x-auto">
      <table class="w-full text-sm">
        <thead>
          <tr class="text-left text-xs uppercase tracking-wider text-slate-400 border-b border-slate-100 bg-slate-50/60">
            <th class="px-5 py-3 font-medium">IP</th><th class="px-4 py-3 font-medium">状态</th>
            <th class="px-4 py-3 font-medium">MAC</th><th class="px-4 py-3 font-medium">主机名</th>
            <th class="px-4 py-3 font-medium">标注</th><th class="px-4 py-3 font-medium">负责人</th>
            <th class="px-4 py-3 font-medium">最后在线</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-slate-50">
          <tr v-for="a in listRows" :key="a.ip" @click="a.id && (selected = a)"
              :class="['hover:bg-brand-50/40 transition-colors', a.id ? 'cursor-pointer' : '']">
            <td class="px-5 py-2.5 font-mono text-slate-800">{{ a.ip }}</td>
            <td class="px-4 py-2.5">
              <span :class="['inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-medium',
                             a.status === 'online' ? 'bg-emerald-50 text-online' :
                             a.status === 'conflict' ? 'bg-red-50 text-conflict' :
                             a.status === 'reserved' ? 'bg-purple-50 text-reserved' :
                             a.status === 'rogue' ? 'bg-orange-50 text-rogue' : 'bg-slate-100 text-slate-500']">
                {{ statusText[a.status] || a.status }}
              </span>
            </td>
            <td class="px-4 py-2.5 font-mono text-xs text-slate-500">{{ a.mac || '—' }}</td>
            <td class="px-4 py-2.5 text-slate-600">{{ a.hostname || '—' }}</td>
            <td class="px-4 py-2.5 text-slate-700">{{ a.label || '—' }}</td>
            <td class="px-4 py-2.5 text-slate-600">{{ a.owner || '—' }}</td>
            <td class="px-4 py-2.5 text-slate-400 text-xs">{{ fmtTime(a.last_seen) || '—' }}</td>
          </tr>
          <tr v-if="!listRows.length">
            <td colspan="7" class="px-4 py-16 text-center text-slate-400">
              {{ subnets.length ? '暂无已观测地址，点击「立即扫描」发现设备' : '还没有受管子网' }}
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- 详情抽屉：遮罩 + 右侧滑出 -->
    <Teleport to="body">
      <Transition name="fade">
        <div v-if="selected" class="fixed inset-0 bg-slate-900/30 backdrop-blur-[2px] z-40" @click="selected = null" />
      </Transition>
      <aside v-if="selected"
             class="fixed right-0 top-0 h-full w-[24rem] bg-white shadow-pop z-50 animate-slide-in-right flex flex-col">
        <header class="px-6 py-5 border-b border-slate-100 flex items-start justify-between">
          <div>
            <h3 class="font-mono font-semibold text-xl text-slate-900 tracking-tight">{{ selected.ip }}</h3>
            <span :class="['inline-flex items-center gap-1.5 mt-2 rounded-full px-2.5 py-0.5 text-xs font-medium',
                           selected.status === 'online' ? 'bg-emerald-50 text-online' :
                           selected.status === 'conflict' ? 'bg-red-50 text-conflict' :
                           selected.status === 'reserved' ? 'bg-purple-50 text-reserved' :
                           selected.status === 'rogue' ? 'bg-orange-50 text-rogue' : 'bg-slate-100 text-slate-500']">
              <i :class="['w-1.5 h-1.5 rounded-full', legend.find(l => l.key === selected?.status)?.dot || 'bg-slate-400']"></i>
              {{ statusText[selected.status] || selected.status }}
            </span>
          </div>
          <button @click="selected = null" class="w-8 h-8 rounded-lg text-slate-400 hover:bg-slate-100 hover:text-slate-600 transition">✕</button>
        </header>

        <div class="flex-1 overflow-y-auto px-6 py-5">
          <dl class="grid grid-cols-2 gap-3 mb-6">
            <div class="bg-slate-50 rounded-xl p-3">
              <dt class="text-xs text-slate-400 mb-1">MAC 地址</dt>
              <dd class="font-mono text-sm text-slate-700 break-all">{{ selected.mac || '—' }}</dd>
            </div>
            <div class="bg-slate-50 rounded-xl p-3">
              <dt class="text-xs text-slate-400 mb-1">主机名</dt>
              <dd class="text-sm text-slate-700 break-all">{{ selected.hostname || '—' }}</dd>
            </div>
            <div class="bg-slate-50 rounded-xl p-3">
              <dt class="text-xs text-slate-400 mb-1">首次发现</dt>
              <dd class="text-sm text-slate-700">{{ fmtTime(selected.first_seen) || '—' }}</dd>
            </div>
            <div class="bg-slate-50 rounded-xl p-3">
              <dt class="text-xs text-slate-400 mb-1">最后在线</dt>
              <dd class="text-sm text-slate-700">{{ fmtTime(selected.last_seen) || '从未在线' }}</dd>
            </div>
          </dl>

          <template v-if="selected.id">
            <label class="block text-sm font-medium text-slate-600 mb-3">标注
              <input v-model="selected.label" class="border border-slate-200 rounded-xl w-full px-3 py-2 mt-1.5 font-normal" placeholder="如：张三的笔记本" /></label>
            <label class="block text-sm font-medium text-slate-600 mb-3">负责人
              <DictSelect v-model="selected.owner" :options="dictOwners" /></label>
            <label class="block text-sm font-medium text-slate-600 mb-2">类型
              <DictSelect v-model="selected.dev_type" :options="dictTypes" placeholder="未分类" /></label>
          </template>
          <div v-else class="bg-slate-50 rounded-xl p-4 text-sm text-slate-400 text-center">
            该地址尚未被观测到，扫描发现后即可标注。
          </div>
        </div>

        <footer class="px-6 py-4 border-t border-slate-100 flex gap-3">
          <button v-if="selected.id && !isViewer()" @click="save"
                  class="flex-1 bg-brand-600 text-white rounded-xl px-4 py-2.5 text-sm font-medium hover:bg-brand-700 active:scale-[0.98] transition shadow-sm shadow-brand-600/30">保存标注</button>
          <button @click="selected = null" class="border border-slate-200 rounded-xl px-5 py-2.5 text-sm text-slate-500 hover:bg-slate-50 transition">关闭</button>
        </footer>
      </aside>
    </Teleport>
  </div>
</template>

<style scoped>
.fade-enter-active, .fade-leave-active { transition: opacity .25s ease; }
.fade-enter-from, .fade-leave-to { opacity: 0; }
</style>
