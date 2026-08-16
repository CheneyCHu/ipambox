<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api, isViewer, type Overview, type Subnet, type Alert, type UplinkStatus, type UplinkEvent } from '../api'
import { t } from '../i18n'

const overview = ref<Overview | null>(null)
const subnets = ref<Subnet[]>([])
const alerts = ref<Alert[]>([])
const uplink = ref<UplinkStatus | null>(null)
const uplinkPending = ref(0)
const uplinkEvents = ref<UplinkEvent[]>([])
const checking = ref(false)
const loadError = ref('')

onMounted(async () => {
  try {
    ;[overview.value, subnets.value, alerts.value] = await Promise.all([
      api.overview(), api.subnets(), api.alerts(true),
    ])
    const up = await api.uplink()
    uplink.value = up.status
    uplinkPending.value = up.pending
    uplinkEvents.value = up.events
  } catch (e: any) {
    loadError.value = t('dash.loadErr')
  }
})

async function checkNow() {
  checking.value = true
  try {
    const r = await api.uplinkCheck()
    uplink.value = r.status
    uplinkPending.value = r.pending
    const up = await api.uplink()
    uplinkEvents.value = up.events
  } finally {
    checking.value = false
  }
}

const cards = computed(() => [
  { key: 'total', label: t('dash.total'), icon: 'M4 4h4v4H4V4zm6 0h4v4h-4V4zm6 0h4v4h-4V4zM4 10h4v4H4v-4zm6 0h4v4h-4v-4zm6 0h4v4h-4v-4zM4 16h4v4H4v-4zm6 0h4v4h-4v-4zm6 0h4v4h-4v-4z', tint: 'text-brand-600 bg-brand-50' },
  { key: 'usage', label: t('dash.usage'), icon: 'M4 20V10h4v10H4zm6 0V4h4v16h-4zm6 0v-7h4v7h-4z', tint: 'text-reserved bg-purple-50' },
  { key: 'online', label: t('dash.online'), icon: 'M12 2a10 10 0 100 20 10 10 0 000-20zm-1.5 14.5l-4-4 1.4-1.4 2.6 2.6 5.6-5.6 1.4 1.4-7 7z', tint: 'text-online bg-emerald-50' },
  { key: 'alerts', label: t('dash.alerts'), icon: 'M12 2a7 7 0 00-7 7v4l-2 3v1h18v-1l-2-3V9a7 7 0 00-7-7zm-2 16a2 2 0 104 0h-4z', tint: 'text-conflict bg-red-50' },
])
</script>

<template>
  <div class="animate-fade-in">
    <header class="mb-6">
      <h1 class="text-2xl font-bold text-slate-900 tracking-tight">{{ t('dash.title') }}</h1>
      <p class="text-sm text-slate-400 mt-1">{{ t('dash.sub') }}</p>
    </header>

    <div v-if="loadError" class="bg-amber-50 border border-amber-200 text-amber-700 rounded-xl px-4 py-3 mb-5 text-sm">
      ⚠ {{ loadError }}
    </div>

    <!-- 统计卡片 -->
    <div class="grid grid-cols-2 xl:grid-cols-4 gap-4 mb-8">
      <div v-for="c in cards" :key="c.key"
           class="bg-white rounded-2xl p-5 shadow-card border border-slate-100 hover:shadow-pop hover:-translate-y-0.5 transition-all duration-200">
        <div class="flex items-center justify-between mb-3">
          <span class="text-sm text-slate-500">{{ c.label }}</span>
          <span :class="['w-9 h-9 rounded-xl flex items-center justify-center', c.tint]">
            <svg viewBox="0 0 24 24" class="w-5 h-5" fill="currentColor"><path :d="c.icon" /></svg>
          </span>
        </div>
        <div class="text-3xl font-bold text-slate-900 tracking-tight tabular-nums">
          <template v-if="c.key === 'total'">{{ overview?.stats.total ?? '—' }}</template>
          <template v-else-if="c.key === 'usage'">{{ overview ? overview.usage_pct.toFixed(0) + '%' : '—' }}</template>
          <template v-else-if="c.key === 'online'"><span class="text-online">{{ overview?.stats.online ?? '—' }}</span></template>
          <template v-else><span :class="overview?.unread_alerts ? 'text-conflict' : ''">{{ overview?.unread_alerts ?? '—' }}</span></template>
        </div>
        <!-- 使用率微趋势条 -->
        <div v-if="c.key === 'usage' && overview" class="mt-3 h-1.5 bg-slate-100 rounded-full overflow-hidden">
          <div class="h-full rounded-full transition-all duration-700"
               :class="overview.usage_pct > 85 ? 'bg-conflict' : 'bg-gradient-to-r from-brand-500 to-reserved'"
               :style="{ width: overview.usage_pct + '%' }" />
        </div>
        <p v-else-if="c.key === 'online' && overview" class="mt-2 text-xs text-slate-400">
          {{ t('dash.offline') }} {{ overview.stats.offline }} · {{ t('dash.conflict') }} <span class="text-conflict font-medium">{{ overview.stats.conflict }}</span>
        </p>
        <p v-else class="mt-2 text-xs text-slate-300 select-none">　</p>
      </div>
    </div>

    <div class="grid grid-cols-1 lg:grid-cols-2 gap-5">
      <!-- 外网连通状态（断网续存 / 边缘自治） -->
      <section class="bg-white rounded-2xl shadow-card border border-slate-100 overflow-hidden">
        <header class="px-5 py-4 border-b border-slate-100 flex items-center justify-between">
          <h2 class="font-semibold text-slate-800">{{ t('dash.uplink') }}</h2>
          <button v-if="!isViewer()" @click="checkNow" :disabled="checking"
                  class="text-xs text-brand-600 hover:text-brand-700 font-medium disabled:opacity-50">
            {{ checking ? t('dash.checking') : t('dash.check') }}
          </button>
        </header>
        <div class="px-5 py-4">
          <div v-if="uplink" class="flex items-center gap-3 mb-3">
            <span :class="['inline-flex items-center gap-2 rounded-full px-3 py-1 text-xs font-medium',
                            uplink.online ? 'bg-emerald-50 text-online' : 'bg-red-50 text-conflict']">
              <span :class="['w-2 h-2 rounded-full', uplink.online ? 'bg-emerald-400' : 'bg-red-400 animate-pulse']"></span>
              {{ uplink.online ? t('dash.onlineTag') : t('dash.offlineTag') }}
            </span>
            <span v-if="uplinkPending > 0" class="text-xs bg-amber-50 text-amber-600 rounded-full px-2.5 py-1">
              {{ uplinkPending }} {{ t('dash.pendingNotify') }}
            </span>
          </div>
          <dl v-if="uplink" class="text-xs text-slate-500 space-y-1.5">
            <div class="flex gap-2"><dt class="w-20 shrink-0 text-slate-400">{{ t('dash.probe') }}</dt><dd class="font-mono">{{ uplink.probe }}</dd></div>
            <div class="flex gap-2"><dt class="w-20 shrink-0 text-slate-400">{{ t('dash.lastProbe') }}</dt><dd>{{ uplink.detail }}</dd></div>
            <div class="flex gap-2" v-if="uplink.last_online"><dt class="w-20 shrink-0 text-slate-400">{{ t('dash.lastOnline') }}</dt><dd>{{ uplink.last_online }}</dd></div>
            <div class="flex gap-2" v-if="!uplink.online && uplink.since"><dt class="w-20 shrink-0 text-slate-400">{{ t('dash.offlineSince') }}</dt><dd>{{ uplink.since }}</dd></div>
          </dl>
          <p v-if="uplink && !uplink.online" class="mt-3 text-xs text-amber-600 bg-amber-50 rounded-lg px-3 py-2">
            {{ t('dash.offlineTip') }}
          </p>
          <!-- 最近连通事件 -->
          <ul v-if="uplinkEvents.length" class="mt-3 border-t border-slate-50 pt-2 space-y-1">
            <li v-for="e in uplinkEvents.slice(0, 4)" :key="e.id" class="flex items-center gap-2 text-xs text-slate-500">
              <span :class="['w-1.5 h-1.5 rounded-full shrink-0', e.online ? 'bg-emerald-400' : 'bg-red-400']"></span>
              <span :class="e.online ? 'text-online' : 'text-conflict'">{{ e.online ? t('dash.recovered') : t('dash.lost') }}</span>
              <span class="text-slate-300">{{ e.created_at }}</span>
            </li>
          </ul>
        </div>
      </section>

      <!-- 子网列表 -->
      <section class="bg-white rounded-2xl shadow-card border border-slate-100 overflow-hidden">
        <header class="px-5 py-4 border-b border-slate-100 flex items-center justify-between">
          <h2 class="font-semibold text-slate-800">{{ t('dash.subnets') }}</h2>
          <RouterLink to="/subnets" class="text-xs text-brand-600 hover:text-brand-700 font-medium">{{ t('dash.enterMap') }}</RouterLink>
        </header>
        <ul class="divide-y divide-slate-50">
          <li v-for="s in subnets" :key="s.id" class="px-5 py-3.5 flex items-center gap-4 group">
            <span class="w-9 h-9 rounded-xl bg-brand-50 text-brand-600 flex items-center justify-center shrink-0">
              <svg viewBox="0 0 24 24" class="w-5 h-5" fill="currentColor"><path d="M12 2a10 10 0 100 20 10 10 0 000-20zM4.1 9h3.4a16 16 0 000 6H4.1A8 8 0 014.1 9zm7.9-6.9A14 14 0 0113.9 9h-3.8A14 14 0 0112 2.1zM10.1 15h3.8A14 14 0 0112 21.9 14 14 0 0110.1 15zM9.5 9a14.3 14.3 0 000 6h5a14.3 14.3 0 000-6h-5zm10.4 0h-3.4a16 16 0 010 6h3.4A8 8 0 0019.9 9z"/></svg>
            </span>
            <RouterLink :to="`/subnets/${s.id}`" class="flex-1 min-w-0">
              <p class="font-mono text-sm font-medium text-slate-800 group-hover:text-brand-600 transition">{{ s.cidr }}</p>
              <p class="text-xs text-slate-400 truncate">{{ s.name }}</p>
            </RouterLink>
          </li>
          <li v-if="!subnets.length" class="px-5 py-12 text-center text-slate-400 text-sm">{{ t('dash.noSubnet') }}</li>
        </ul>
      </section>

      <!-- 未读告警 -->
      <section class="bg-white rounded-2xl shadow-card border border-slate-100 overflow-hidden">
        <header class="px-5 py-4 border-b border-slate-100 flex items-center justify-between">
          <h2 class="font-semibold text-slate-800">{{ t('dash.unread') }}</h2>
          <RouterLink to="/alerts" class="text-xs text-brand-600 hover:text-brand-700 font-medium">{{ t('dash.allAlerts') }}</RouterLink>
        </header>
        <ul class="divide-y divide-slate-50">
          <li v-for="a in alerts.slice(0, 5)" :key="a.id" class="px-5 py-3.5 text-sm flex items-start gap-3">
            <span :class="['mt-1.5 w-2 h-2 rounded-full shrink-0', a.level === 'critical' ? 'bg-conflict' : 'bg-rogue']"></span>
            <span class="min-w-0">
              <span class="font-mono text-slate-700">{{ a.ip }}</span>
              <span class="text-slate-500 ml-2">{{ a.message }}</span>
            </span>
          </li>
          <li v-if="!alerts.length" class="px-5 py-12 text-center text-slate-400 text-sm">
            <p class="text-3xl mb-2">🎉</p>{{ t('dash.noAlert') }}
          </li>
        </ul>
      </section>
    </div>
  </div>
</template>
