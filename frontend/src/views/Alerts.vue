<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api, type Alert } from '../api'

const alerts = ref<Alert[]>([])
onMounted(async () => { alerts.value = await api.alerts().catch(() => []) })
</script>

<template>
  <div class="animate-fade-in max-w-4xl">
    <header class="mb-5">
      <h1 class="text-2xl font-bold text-slate-900 tracking-tight">告警中心</h1>
      <p class="text-sm text-slate-400 mt-1">IP 冲突与未授权设备事件</p>
    </header>

    <ul class="space-y-2.5">
      <li v-for="a in alerts" :key="a.id"
          :class="['bg-white rounded-2xl shadow-card border px-5 py-4 flex items-center gap-4 transition',
                   a.read ? 'border-slate-100 opacity-60' : 'border-slate-100 hover:shadow-pop']">
        <span :class="['w-9 h-9 rounded-xl flex items-center justify-center shrink-0',
                       a.level === 'critical' ? 'bg-red-50 text-conflict' : 'bg-orange-50 text-rogue']">
          <svg viewBox="0 0 24 24" class="w-5 h-5" fill="currentColor"><path d="M12 2L1 21h22L12 2zm0 4.3L19.5 19h-15L12 6.3zM11 10h2v5h-2v-5zm0 6h2v2h-2v-2z"/></svg>
        </span>
        <div class="flex-1 min-w-0">
          <p class="text-sm">
            <span class="font-mono font-medium text-slate-800">{{ a.ip }}</span>
            <span :class="a.read ? 'text-slate-400' : 'text-slate-600'" class="ml-2">{{ a.message }}</span>
          </p>
          <p class="text-xs text-slate-400 mt-1">{{ a.created_at }}
            <span :class="['ml-2 rounded-full px-2 py-0.5 font-medium', a.level === 'critical' ? 'bg-red-50 text-conflict' : 'bg-orange-50 text-rogue']">
              {{ a.level === 'critical' ? '严重' : '警告' }}
            </span>
          </p>
        </div>
        <button v-if="!a.read" @click="api.markAlertRead(a.id).then(() => a.read = true)"
                class="shrink-0 text-sm text-brand-600 hover:text-brand-700 font-medium border border-brand-200 rounded-xl px-3.5 py-1.5 hover:bg-brand-50 active:scale-95 transition">标记已读</button>
        <span v-else class="shrink-0 text-xs text-slate-300">已读</span>
      </li>
      <li v-if="!alerts.length" class="bg-white rounded-2xl shadow-card border border-slate-100 px-5 py-16 text-center text-slate-400">
        <p class="text-4xl mb-3">🎉</p>暂无告警，网络状态良好
      </li>
    </ul>
  </div>
</template>
