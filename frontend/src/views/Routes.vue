<script setup lang="ts">
/** 路由设置：查看本机路由表，添加/删除静态路由。增删为高危操作，需二次确认。 */
import { computed, onMounted, ref } from 'vue'
import { api, type RouteEntry } from '../api'
import { t } from '../i18n'

const routes = ref<RouteEntry[]>([])
const loading = ref(true)
const error = ref('')
const msg = ref('')
const family = ref<'ipv4' | 'ipv6'>('ipv4')
const search = ref('')

// ---- 添加路由 ----
const dest = ref('')
const gateway = ref('')
const addBusy = ref(false)
const confirmingAdd = ref(false)

// ---- 删除路由 ----
const deleting = ref<RouteEntry | null>(null)
const delBusy = ref(false)

const filtered = computed(() => routes.value.filter(r => {
  if (r.family !== family.value) return false
  if (!search.value) return true
  const q = search.value.toLowerCase()
  return r.destination.toLowerCase().includes(q) || r.gateway.toLowerCase().includes(q) || r.iface.toLowerCase().includes(q)
}))

async function load() {
  loading.value = true
  error.value = ''
  try {
    routes.value = await api.routes()
  } catch (e: any) {
    error.value = e.message
    routes.value = []
  } finally { loading.value = false }
}

function flash(m: string) { msg.value = m; setTimeout(() => { if (msg.value === m) msg.value = '' }, 4000) }

async function doAdd() {
  addBusy.value = true
  try {
    await api.addRoute({ destination: dest.value.trim(), gateway: gateway.value.trim() })
    flash(t('✅ 已添加路由 {dest}', { dest: dest.value }))
    dest.value = ''; gateway.value = ''; confirmingAdd.value = false
    await load()
  } catch (e: any) {
    flash(t('❌ 添加失败：') + e.message)
  } finally { addBusy.value = false }
}

async function doDelete() {
  if (!deleting.value) return
  delBusy.value = true
  try {
    await api.deleteRoute({ destination: deleting.value.destination, gateway: deleting.value.gateway || undefined })
    flash(t('✅ 已删除路由 {dest}', { dest: deleting.value.destination }))
    deleting.value = null
    await load()
  } catch (e: any) {
    flash(t('❌ 删除失败：') + e.message)
    deleting.value = null
  } finally { delBusy.value = false }
}

const canAdd = computed(() => {
  const d = dest.value.trim()
  if (d === 'default') return gateway.value.trim().length > 0
  return /^\d{1,3}(\.\d{1,3}){3}\/\d{1,2}$/.test(d) && gateway.value.trim().length > 0
})

onMounted(load)
</script>

<template>
  <div class="animate-fade-in">
    <header class="mb-5 flex flex-wrap items-end justify-between gap-3">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 tracking-tight">{{ t('路由设置') }}</h1>
        <p class="text-sm text-slate-400 mt-1">{{ t('本机路由表查看与静态路由管理') }}</p>
      </div>
      <button @click="load" :disabled="loading"
              class="border border-slate-200 bg-white rounded-xl px-4 py-2 text-sm text-slate-600 hover:border-brand-400 hover:text-brand-600 disabled:opacity-50 active:scale-95 transition flex items-center gap-2">
        <svg :class="['w-4 h-4', loading && 'animate-spin']" viewBox="0 0 24 24" fill="none"><circle cx="12" cy="12" r="9" stroke="currentColor" stroke-opacity=".25" stroke-width="3"/><path d="M21 12a9 9 0 00-9-9" stroke="currentColor" stroke-width="3" stroke-linecap="round"/></svg>
        {{ t('刷新') }}
      </button>
    </header>

    <div v-if="error" class="bg-red-50 border border-red-200 text-conflict rounded-xl px-4 py-2.5 mb-4 text-sm">{{ t('读取路由表失败：') }}{{ error }}</div>
    <div v-if="msg" class="bg-emerald-50 border border-emerald-200 text-online rounded-xl px-4 py-2.5 mb-4 text-sm animate-fade-in">{{ msg }}</div>

    <!-- 添加静态路由 -->
    <section class="bg-white rounded-2xl shadow-card border border-slate-100 p-5 mb-4">
      <h2 class="font-semibold text-slate-800 mb-3">{{ t('添加静态路由') }}</h2>
      <div class="flex flex-wrap items-end gap-3">
        <label class="block text-sm text-slate-600 flex-1 min-w-48">{{ t('目的网段') }}
          <input v-model="dest" :placeholder="t('如 10.0.0.0/24 或 default')"
                 class="border border-slate-200 rounded-xl w-full px-3 py-2 mt-1.5 font-mono font-normal" /></label>
        <label class="block text-sm text-slate-600 flex-1 min-w-48">{{ t('网关（下一跳）') }}
          <input v-model="gateway" :placeholder="t('如 192.168.1.1')"
                 class="border border-slate-200 rounded-xl w-full px-3 py-2 mt-1.5 font-mono font-normal" /></label>
        <button v-if="!confirmingAdd" @click="confirmingAdd = true" :disabled="!canAdd"
                class="bg-brand-600 text-white rounded-xl px-5 py-2 text-sm font-medium hover:bg-brand-700 active:scale-95 disabled:opacity-50 transition shadow-sm shadow-brand-600/30">
          {{ t('添加') }}
        </button>
        <div v-else class="flex items-center gap-2">
          <button @click="doAdd" :disabled="addBusy"
                  class="bg-conflict text-white rounded-xl px-4 py-2 text-sm font-medium hover:opacity-90 active:scale-95 disabled:opacity-50 transition">
            {{ addBusy ? t('添加中…') : t('确认添加') }}
          </button>
          <button @click="confirmingAdd = false" class="border border-slate-200 rounded-xl px-4 py-2 text-sm text-slate-500 hover:bg-slate-50 transition">{{ t('取消') }}</button>
        </div>
      </div>
      <p class="text-xs text-slate-400 mt-2">{{ t('⚠ 需要以 root 权限运行 IPAMBox；错误的路由可能导致网络中断，请确认无误后添加。') }}</p>
    </section>

    <!-- 路由表 -->
    <section class="bg-white rounded-2xl shadow-card border border-slate-100 overflow-hidden">
      <div class="px-5 py-3.5 border-b border-slate-100 flex flex-wrap items-center gap-3">
        <div class="flex bg-slate-100 rounded-xl p-0.5">
          <button v-for="f in (['ipv4', 'ipv6'] as const)" :key="f" @click="family = f"
                  :class="['px-3.5 py-1.5 text-xs font-medium rounded-[10px] transition',
                           family === f ? 'bg-white text-brand-700 shadow-sm' : 'text-slate-500 hover:text-slate-700']">
            {{ f === 'ipv4' ? 'IPv4' : 'IPv6' }}
          </button>
        </div>
        <input v-model="search" :placeholder="t('搜索目的地 / 网关 / 接口…')"
               class="border border-slate-200 rounded-xl px-3 py-1.5 text-sm flex-1 max-w-xs" />
        <span class="ml-auto text-xs text-slate-400 tabular-nums">{{ t('{n} 条', { n: filtered.length }) }}</span>
      </div>

      <div v-if="loading" class="py-16 text-center text-slate-400 text-sm">{{ t('读取中…') }}</div>
      <div v-else-if="!filtered.length" class="py-16 text-center text-slate-400 text-sm">{{ t('没有匹配的路由') }}</div>
      <div v-else class="overflow-x-auto">
        <table class="w-full text-sm">
          <thead>
            <tr class="text-left text-xs text-slate-400 border-b border-slate-100">
              <th class="px-5 py-3 font-medium">{{ t('目的地') }}</th>
              <th class="px-4 py-3 font-medium">{{ t('网关') }}</th>
              <th class="px-4 py-3 font-medium">{{ t('接口') }}</th>
              <th class="px-4 py-3 font-medium text-right">{{ t('操作') }}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-50">
            <tr v-for="(r, i) in filtered" :key="i" class="hover:bg-slate-50/60 transition-colors">
              <td class="px-5 py-2.5 font-mono text-slate-800">
                <span v-if="r.destination === 'default'" class="bg-brand-50 text-brand-600 text-xs rounded-full px-2 py-0.5 font-medium">{{ t('默认路由') }}</span>
                <template v-else>{{ r.destination }}</template>
              </td>
              <td class="px-4 py-2.5 font-mono text-xs text-slate-500">{{ r.gateway || t('直连') }}</td>
              <td class="px-4 py-2.5 font-mono text-xs text-slate-500">{{ r.iface }}</td>
              <td class="px-4 py-2.5 text-right">
                <button @click="deleting = r" class="text-conflict hover:opacity-80 text-xs font-medium">{{ t('删除') }}</button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <!-- 删除确认 -->
    <Teleport to="body">
      <div v-if="deleting" class="fixed inset-0 z-50 bg-ink-950/40 backdrop-blur-sm flex items-center justify-center p-4"
           @click.self="deleting = null">
        <div class="bg-white rounded-2xl shadow-pop w-full max-w-md p-6 animate-fade-in">
          <h3 class="font-semibold text-conflict mb-2">{{ t('删除路由 {dest}？', { dest: deleting.destination }) }}</h3>
          <p class="text-sm text-slate-500 mb-1">{{ t('网关：') }}<span class="font-mono">{{ deleting.gateway || t('直连') }}</span>　{{ t('接口：') }}<span class="font-mono">{{ deleting.iface }}</span></p>
          <p class="text-sm text-slate-500 mb-5">{{ t('删除系统路由可能影响网络连通性（删除默认路由会直接断网），请确认后继续。') }}</p>
          <div class="flex gap-3">
            <button @click="doDelete" :disabled="delBusy"
                    class="flex-1 bg-conflict text-white rounded-xl px-4 py-2 text-sm font-medium hover:opacity-90 active:scale-95 disabled:opacity-50 transition">
              {{ delBusy ? t('删除中…') : t('确认删除') }}
            </button>
            <button @click="deleting = null" class="flex-1 border border-slate-200 rounded-xl px-4 py-2 text-sm text-slate-500 hover:bg-slate-50 transition">{{ t('取消') }}</button>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>
