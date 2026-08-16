<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api, downloadExport, type Subnet, type Overview } from '../api'

const subnets = ref<Subnet[]>([])
const overview = ref<Overview | null>(null)
const statsMap = ref<Record<number, { total: number; online: number; offline: number; free: number; conflict: number; usage_pct: number }>>({})
const msg = ref('')

onMounted(async () => {
  subnets.value = await api.subnets().catch(() => [])
  overview.value = await api.overview().catch(() => null)
  for (const s of subnets.value) {
    statsMap.value[s.id] = await api.stats(s.id).catch(() => null) as any
  }
})

async function doExport(s: Subnet) {
  msg.value = ''
  try {
    await downloadExport(s.id, s.cidr)
    msg.value = `✅ 已导出 ${s.cidr} 台账`
  } catch (e: any) { msg.value = '❌ ' + e.message }
}

async function doImport(s: Subnet, ev: Event) {
  const file = (ev.target as HTMLInputElement).files?.[0]
  if (!file) return
  const text = await file.text()
  try {
    const res = await api.importCSV(s.id, text)
    msg.value = `✅ 导入完成：更新 ${res.updated} 条，跳过 ${res.skipped} 条`
  } catch (e: any) { msg.value = '❌ 导入失败：' + e.message }
  ;(ev.target as HTMLInputElement).value = ''
}
</script>

<template>
  <div class="max-w-3xl animate-fade-in">
    <header class="mb-5">
      <h1 class="text-2xl font-bold text-slate-900 tracking-tight">报表</h1>
      <p class="text-sm text-slate-400 mt-1">子网使用率分析与台账批量维护</p>
    </header>

    <div v-if="msg" class="bg-brand-50 border border-brand-100 text-brand-700 rounded-xl px-4 py-2.5 mb-4 text-sm animate-fade-in">{{ msg }}</div>

    <section class="bg-white rounded-2xl shadow-card border border-slate-100 p-6 mb-5">
      <h2 class="font-semibold text-slate-800 mb-5">子网使用率</h2>
      <div v-for="s in subnets" :key="s.id" class="mb-5 last:mb-0">
        <div class="flex justify-between items-baseline text-sm mb-2">
          <span><span class="font-mono font-medium text-slate-800">{{ s.cidr }}</span> <span class="text-slate-400 ml-1">{{ s.name }}</span></span>
          <span class="tabular-nums" :class="(statsMap[s.id]?.usage_pct ?? 0) > 85 ? 'text-conflict font-semibold' : 'text-slate-500'">
            {{ statsMap[s.id] ? statsMap[s.id].usage_pct.toFixed(0) + '%（在线 ' + statsMap[s.id].online + ' / 共 ' + statsMap[s.id].total + ' 条观测）' : '—' }}
          </span>
        </div>
        <div class="h-2.5 bg-slate-100 rounded-full overflow-hidden">
          <div class="h-full rounded-full transition-all duration-700"
               :class="(statsMap[s.id]?.usage_pct ?? 0) > 85 ? 'bg-gradient-to-r from-conflict to-rogue' : 'bg-gradient-to-r from-brand-500 to-reserved'"
               :style="{ width: (statsMap[s.id]?.usage_pct ?? 0) + '%' }" />
        </div>
      </div>
      <p v-if="!subnets.length" class="text-slate-400 text-sm text-center py-6">暂无子网</p>
    </section>

    <section class="bg-white rounded-2xl shadow-card border border-slate-100 p-6">
      <h2 class="font-semibold text-slate-800 mb-1">台账导入 / 导出</h2>
      <p class="text-sm text-slate-400 mb-5">Excel 兼容 CSV：导出后编辑「标注 / 负责人 / 类型」三列，再导入即可批量更新。</p>
      <div v-for="s in subnets" :key="s.id" class="flex items-center gap-3 py-3 border-b border-slate-50 last:border-0">
        <span class="w-9 h-9 rounded-xl bg-brand-50 text-brand-600 flex items-center justify-center shrink-0 text-sm">⇄</span>
        <span class="flex-1"><span class="font-mono text-sm font-medium text-slate-800">{{ s.cidr }}</span> <span class="text-slate-400 text-sm ml-1">{{ s.name }}</span></span>
        <button @click="doExport(s)" class="border border-slate-200 rounded-xl px-4 py-1.5 text-sm text-slate-600 hover:border-brand-400 hover:text-brand-600 active:scale-95 transition">⬇ 导出</button>
        <label class="border border-slate-200 rounded-xl px-4 py-1.5 text-sm text-slate-600 hover:border-brand-400 hover:text-brand-600 active:scale-95 cursor-pointer transition">
          ⬆ 导入
          <input type="file" accept=".csv" class="hidden" @change="doImport(s, $event)" />
        </label>
      </div>
      <p v-if="!subnets.length" class="text-slate-400 text-sm text-center py-6">暂无子网</p>
    </section>
  </div>
</template>
