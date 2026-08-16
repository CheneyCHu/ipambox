<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api, isViewer, type NICInfo, type Subnet } from '../api'
import { t } from '../i18n'

const router = useRouter()
const subnets = ref<Subnet[]>([])
const statsMap = ref<Record<number, { total: number; online: number; free: number; usage_pct: number }>>({})
const loadError = ref('')
const msg = ref('')

// ---- 新增 / 编辑弹窗 ----
const showForm = ref(false)
const editing = ref<Subnet | null>(null) // null = 新增
const fCIDR = ref('')
const fName = ref('')
const fDesc = ref('')
const fVLAN = ref(0)
const fIface = ref('')
const fErr = ref('')
const saving = ref(false)

// ---- 删除确认 ----
const deleting = ref<Subnet | null>(null)
const deleteBusy = ref(false)

const nics = ref<NICInfo[]>([])

async function load() {
  loadError.value = ''
  try {
    subnets.value = await api.subnets()
    const entries = await Promise.all(
      subnets.value.map(async s => [s.id, await api.stats(s.id).catch(() => null)] as const))
    statsMap.value = Object.fromEntries(entries.filter(([, v]) => v)) as any
  } catch (e: any) { loadError.value = t('加载失败：') + e.message }
}

onMounted(async () => {
  load()
  nics.value = await api.interfaces().catch(() => [])
})

function openCreate() {
  editing.value = null
  fCIDR.value = ''; fName.value = ''; fDesc.value = ''; fVLAN.value = 0; fIface.value = ''
  fErr.value = ''; showForm.value = true
}

function openEdit(s: Subnet) {
  editing.value = s
  fCIDR.value = s.cidr; fName.value = s.name; fDesc.value = s.description
  fVLAN.value = s.vlan; fIface.value = s.iface
  fErr.value = ''; showForm.value = true
}

// 点选网卡：新增时自动带出 CIDR；编辑时只改接入网卡
function pickNIC(nic: NICInfo) {
  fIface.value = nic.name
  if (!editing.value) fCIDR.value = nic.cidr
}

async function save() {
  fErr.value = ''
  if (!editing.value && !fCIDR.value.includes('/')) { fErr.value = t('请填写合法的 CIDR，如 192.168.1.0/24'); return }
  saving.value = true
  try {
    if (editing.value) {
      await api.updateSubnet(editing.value.id, {
        name: fName.value, description: fDesc.value, vlan: fVLAN.value, iface: fIface.value,
      })
      msg.value = t('✅ 子网已更新')
    } else {
      await api.createSubnet({
        cidr: fCIDR.value, name: fName.value || fCIDR.value,
        description: fDesc.value, vlan: fVLAN.value, iface: fIface.value,
      })
      msg.value = t('✅ 子网已创建')
    }
    showForm.value = false
    await load()
  } catch (e: any) { fErr.value = e.message } finally { saving.value = false }
}

async function doDelete() {
  if (!deleting.value) return
  deleteBusy.value = true
  try {
    await api.deleteSubnet(deleting.value.id)
    msg.value = t('✅ 已删除 {cidr}', { cidr: deleting.value.cidr })
    deleting.value = null
    await load()
  } catch (e: any) { msg.value = t('❌ 删除失败：') + e.message } finally { deleteBusy.value = false }
}

async function scan(s: Subnet) {
  await api.scanNow(s.id).catch(() => {})
  msg.value = t('⚡ 已触发 {cidr} 扫描，稍后在 IP 地图查看结果', { cidr: s.cidr })
}
</script>

<template>
  <div class="animate-fade-in">
    <header class="mb-5 flex flex-wrap items-end justify-between gap-3">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 tracking-tight">{{ t('子网管理') }}</h1>
        <p class="text-sm text-slate-400 mt-1">{{ t('受管网段的增删改查，共 {n} 个', { n: subnets.length }) }}</p>
      </div>
      <button v-if="!isViewer()" @click="openCreate"
              class="bg-brand-600 text-white rounded-xl px-4 py-2 text-sm font-medium hover:bg-brand-700 active:scale-95 transition shadow-sm shadow-brand-600/30">
        + {{ t('添加子网') }}
      </button>
    </header>

    <div v-if="loadError" class="bg-red-50 border border-red-200 text-conflict rounded-xl px-4 py-2.5 mb-4 text-sm">{{ loadError }}</div>
    <p v-if="msg" class="text-sm mb-4 bg-slate-50 rounded-xl px-3 py-2">{{ msg }}</p>

    <div class="bg-white rounded-2xl shadow-card border border-slate-100 overflow-x-auto">
      <table class="w-full text-sm">
        <thead>
          <tr class="text-left text-xs uppercase tracking-wider text-slate-400 border-b border-slate-100 bg-slate-50/60">
            <th class="px-5 py-3 font-medium">{{ t('网段') }}</th><th class="px-4 py-3 font-medium">{{ t('名称') }}</th>
            <th class="px-4 py-3 font-medium">{{ t('接入网卡') }}</th><th class="px-4 py-3 font-medium">VLAN</th>
            <th class="px-4 py-3 font-medium">{{ t('使用率') }}</th><th class="px-4 py-3 font-medium">{{ t('在线') }}</th>
            <th class="px-4 py-3 font-medium text-right">{{ t('操作') }}</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-slate-50">
          <tr v-for="s in subnets" :key="s.id" class="hover:bg-brand-50/40 transition-colors">
            <td class="px-5 py-3 font-mono text-slate-800">{{ s.cidr }}</td>
            <td class="px-4 py-3 text-slate-700">{{ s.name || '—' }}</td>
            <td class="px-4 py-3 font-mono text-xs text-slate-500">{{ s.iface || '—' }}</td>
            <td class="px-4 py-3 text-slate-500">{{ s.vlan || '—' }}</td>
            <td class="px-4 py-3 w-44">
              <template v-if="statsMap[s.id]">
                <div class="flex items-center gap-2">
                  <div class="flex-1 h-1.5 bg-slate-100 rounded-full overflow-hidden">
                    <div :class="['h-full rounded-full', statsMap[s.id].usage_pct > 85 ? 'bg-conflict' : 'bg-brand-500']"
                         :style="{ width: Math.min(100, statsMap[s.id].usage_pct) + '%' }"></div>
                  </div>
                  <span class="text-xs text-slate-500 tabular-nums w-12">{{ statsMap[s.id].usage_pct.toFixed(0) }}%</span>
                </div>
              </template>
              <span v-else class="text-slate-300">—</span>
            </td>
            <td class="px-4 py-3 text-slate-600 tabular-nums">{{ statsMap[s.id]?.online ?? '—' }}</td>
            <td class="px-4 py-3 text-right whitespace-nowrap">
              <button @click="router.push(`/subnets/${s.id}`)" class="text-brand-600 hover:text-brand-700 text-xs font-medium mr-3">{{ t('地图') }}</button>
              <button @click="scan(s)" class="text-slate-500 hover:text-brand-600 text-xs font-medium mr-3">{{ t('扫描') }}</button>
              <button v-if="!isViewer()" @click="openEdit(s)" class="text-slate-500 hover:text-brand-600 text-xs font-medium mr-3">{{ t('编辑') }}</button>
              <button v-if="!isViewer()" @click="deleting = s" class="text-conflict hover:opacity-80 text-xs font-medium">{{ t('删除') }}</button>
            </td>
          </tr>
          <tr v-if="!subnets.length">
            <td colspan="7" class="px-4 py-16 text-center text-slate-400">{{ t('暂无子网，点击右上角「添加子网」') }}</td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- 新增 / 编辑弹窗 -->
    <Teleport to="body">
      <div v-if="showForm" class="fixed inset-0 bg-slate-900/30 backdrop-blur-[2px] z-40 flex items-center justify-center p-6"
           @click.self="showForm = false">
        <div class="bg-white rounded-2xl shadow-pop w-full max-w-md p-6 animate-fade-in">
          <h3 class="font-semibold text-slate-800 mb-4">{{ editing ? t('编辑子网') : t('添加子网') }}</h3>

          <div v-if="nics.length" class="mb-4">
            <p class="text-xs text-slate-400 mb-2">{{ t('从本机网卡选择') }}（{{ editing ? t('仅修改接入网卡') : t('自动填入网段') }}）：</p>
            <div class="space-y-2 max-h-36 overflow-y-auto">
              <button v-for="nic in nics" :key="nic.name + nic.ip" @click="pickNIC(nic)" type="button"
                      :class="['w-full text-left border rounded-xl px-3 py-2 text-sm transition flex items-center gap-2',
                               fIface === nic.name ? 'border-brand-500 bg-brand-50/60' : 'border-slate-200 hover:border-slate-300']">
                <span class="font-mono font-medium text-slate-700">{{ nic.name }}</span>
                <span class="font-mono text-slate-400 text-xs">{{ nic.ip }}/{{ nic.prefix }}</span>
                <span class="ml-auto font-mono text-brand-600 text-xs">{{ nic.cidr }}</span>
              </button>
            </div>
          </div>

          <label class="block text-sm font-medium text-slate-600 mb-3">{{ t('网段（CIDR）') }}
            <input v-model="fCIDR" :disabled="!!editing" placeholder="192.168.1.0/24"
                   class="border border-slate-200 rounded-xl w-full px-3 py-2 mt-1.5 font-mono font-normal disabled:bg-slate-50 disabled:text-slate-400" />
          </label>
          <label class="block text-sm font-medium text-slate-600 mb-3">{{ t('名称') }}
            <input v-model="fName" :placeholder="t('如：办公网')" class="border border-slate-200 rounded-xl w-full px-3 py-2 mt-1.5 font-normal" /></label>
          <div class="grid grid-cols-2 gap-3 mb-3">
            <label class="block text-sm font-medium text-slate-600">{{ t('接入网卡') }}
              <input v-model="fIface" :placeholder="t('如：en0')" class="border border-slate-200 rounded-xl w-full px-3 py-2 mt-1.5 font-mono font-normal" /></label>
            <label class="block text-sm font-medium text-slate-600">{{ t('VLAN（可选）') }}
              <input v-model.number="fVLAN" type="number" min="0" max="4094" class="border border-slate-200 rounded-xl w-full px-3 py-2 mt-1.5 font-normal" /></label>
          </div>
          <label class="block text-sm font-medium text-slate-600 mb-4">{{ t('描述（可选）') }}
            <input v-model="fDesc" class="border border-slate-200 rounded-xl w-full px-3 py-2 mt-1.5 font-normal" /></label>

          <p v-if="fErr" class="text-conflict text-sm mb-3">{{ fErr }}</p>
          <div class="flex gap-3">
            <button @click="save" :disabled="saving"
                    class="bg-brand-600 text-white rounded-xl px-5 py-2 flex-1 text-sm font-medium hover:bg-brand-700 active:scale-[0.98] disabled:opacity-50 transition">
              {{ saving ? t('保存中…') : t('保存') }}
            </button>
            <button @click="showForm = false" class="border border-slate-200 rounded-xl px-5 py-2 text-sm text-slate-500 hover:bg-slate-50 transition">{{ t('取消') }}</button>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- 删除确认 -->
    <Teleport to="body">
      <div v-if="deleting" class="fixed inset-0 bg-slate-900/30 backdrop-blur-[2px] z-40 flex items-center justify-center p-6"
           @click.self="deleting = null">
        <div class="bg-white rounded-2xl shadow-pop w-full max-w-sm p-6 animate-fade-in">
          <h3 class="font-semibold text-conflict mb-2">{{ t('删除子网 {cidr}？', { cidr: deleting.cidr }) }}</h3>
          <p class="text-sm text-slate-500 mb-5">{{ t('该子网下的全部 IP 观测与标注记录将一并删除，不可恢复。') }}</p>
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
  </div>
</template>
