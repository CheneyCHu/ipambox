<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { RouterLink, RouterView, useRoute, useRouter } from 'vue-router'
import AIAssistant from './components/AIAssistant.vue'
import { api, token, role, isViewer, type UplinkStatus } from './api'
import { t, lang, setLang, type Lang } from './i18n'

function toggleLang() { setLang((lang.value === 'zh' ? 'en' : 'zh') as Lang) }

const route = useRoute()
const router = useRouter()
const isAuthPage = () => ['/wizard', '/login'].includes(route.path)

// 当前角色（响应式副本，登录/登出时刷新）
const currentRole = ref(role.get())
onMounted(async () => {
  if (token.get()) {
    try { const me = await api.authMe(); role.set(me.role); currentRole.value = me.role } catch { /* 忽略 */ }
  }
})
const viewer = computed(() => currentRole.value === 'viewer')

// 外网连通状态（断网续存指示）：每 30 秒轮询一次
const uplink = ref<UplinkStatus | null>(null)
const uplinkPending = ref(0)
let uplinkTimer: number | undefined
async function refreshUplink() {
  if (!token.get()) return
  try {
    const r = await api.uplink()
    uplink.value = r.status
    uplinkPending.value = r.pending
  } catch { /* 忽略（未初始化/未登录时静默） */ }
}
onMounted(() => {
  refreshUplink()
  uplinkTimer = window.setInterval(refreshUplink, 30000)
})
onUnmounted(() => { if (uplinkTimer) clearInterval(uplinkTimer) })
const uplinkTip = computed(() => {
  if (!uplink.value) return t('app.uplink.checking')
  const s = uplink.value
  const lines = [s.online ? t('app.uplink.online') : t('app.uplink.offline'), s.detail]
  if (s.since) lines.push(s.since)
  if (uplinkPending.value > 0) lines.push(`${uplinkPending.value} ${t('app.uplink.pending')}`)
  return lines.join('\n')
})

// 侧边栏折叠（记住用户选择）
const collapsed = ref(localStorage.getItem('ipambox_sidebar_collapsed') === '1')
watch(collapsed, v => localStorage.setItem('ipambox_sidebar_collapsed', v ? '1' : '0'))

function logout() {
  token.clear()
  role.clear()
  router.push('/login')
}

const nav = [
  { path: '/dashboard', i18n: 'nav.dashboard', icon: 'M3 13h8V3H3v10zm0 8h8v-6H3v6zm10 0h8V11h-8v10zm0-18v6h8V3h-8z' },
  { path: '/subnets', i18n: 'nav.map', icon: 'M4 4h4v4H4V4zm6 0h4v4h-4V4zm6 0h4v4h-4V4zM4 10h4v4H4v-4zm6 0h4v4h-4v-4zm6 0h4v4h-4v-4zM4 16h4v4H4v-4zm6 0h4v4h-4v-4zm6 0h4v4h-4v-4z' },
  { path: '/subnet-mgr', i18n: 'nav.subnets', icon: 'M12 3l9 5-9 5-9-5 9-5zm-9 8l9 5 9-5M3 15l9 5 9-5' },
  { path: '/network', i18n: 'nav.network', adminOnly: true, icon: 'M12 2a10 10 0 100 20 10 10 0 000-20zM4.1 10a8 8 0 000 4h2.5a17 17 0 010-4H4.1zm13.3 0h2.5a8 8 0 010 4h-2.5a17 17 0 000-4zM8.8 10h6.4a15 15 0 010 4H8.8a15 15 0 010-4zM12 4c1.9 1.4 3 4 3.2 6H8.8C9 8 10.1 5.4 12 4zm0 16c-1.9-1.4-3-4-3.2-6h6.4c-.2 2-1.3 4.6-3.2 6z' },
  { path: '/routes', i18n: 'nav.routes', adminOnly: true, icon: 'M4 17a3 3 0 106 0 3 3 0 00-6 0zm10-10a3 3 0 106 0 3 3 0 00-6 0zM9.2 15.2l5.6-6.4M14 17a3 3 0 106 0 3 3 0 00-6 0z' },
  { path: '/alerts', i18n: 'nav.alerts', icon: 'M12 2a7 7 0 00-7 7v4l-2 3v1h18v-1l-2-3V9a7 7 0 00-7-7zm-2 16a2 2 0 104 0h-4z' },
  { path: '/reports', i18n: 'nav.reports', icon: 'M4 20V10h4v10H4zm6 0V4h4v16h-4zm6 0v-7h4v7h-4z' },
  { path: '/settings', i18n: 'nav.settings', adminOnly: true, icon: 'M19.4 13a7.6 7.6 0 000-2l2-1.6-2-3.4-2.4 1a7.5 7.5 0 00-1.7-1L14.8 3h-4l-.5 2.6a7.5 7.5 0 00-1.7 1l-2.4-1-2 3.4 2 1.6a7.6 7.6 0 000 2l-2 1.6 2 3.4 2.4-1a7.5 7.5 0 001.7 1l.5 2.6h4l.5-2.6a7.5 7.5 0 001.7-1l2.4 1 2-3.4-2-1.6zM12.2 15.5a3.5 3.5 0 110-7 3.5 3.5 0 010 7z' },
]

// 只读账号隐藏管理类菜单
const visibleNav = computed(() => nav.filter(i => !viewer.value || !i.adminOnly))

const isActive = (path: string) => route.path.startsWith(path)
</script>

<template>
  <div class="min-h-screen">
    <!-- 认证页：全屏渲染 -->
    <RouterView v-if="isAuthPage()" />

    <!-- 主布局：深色侧边栏 + 内容区 -->
    <div v-else class="flex min-h-screen">
      <aside :class="[collapsed ? 'w-[4.5rem]' : 'w-60', 'shrink-0 bg-ink-950 text-slate-300 flex flex-col fixed inset-y-0 z-40 transition-all duration-200']">
        <!-- 品牌区 -->
        <div :class="['flex items-center h-16 border-b border-white/5', collapsed ? 'justify-center px-0' : 'gap-3 px-5']">
          <span class="w-9 h-9 shrink-0 rounded-xl bg-gradient-to-br from-brand-500 to-reserved flex items-center justify-center text-white shadow-pop">
            <svg viewBox="0 0 24 24" class="w-5 h-5" fill="currentColor"><path d="M12 2a10 10 0 100 20 10 10 0 000-20zM4.1 9h3.4a16 16 0 000 6H4.1A8 8 0 014.1 9zm7.9-6.9A14 14 0 0113.9 9h-3.8A14 14 0 0112 2.1zM10.1 15h3.8A14 14 0 0112 21.9 14 14 0 0110.1 15zM9.5 9a14.3 14.3 0 000 6h5a14.3 14.3 0 000-6h-5zm10.4 0h-3.4a16 16 0 010 6h3.4A8 8 0 0019.9 9z"/></svg>
          </span>
          <div v-if="!collapsed" class="leading-tight">
            <p class="font-bold text-white tracking-tight flex items-center gap-1.5">IPAMBox
              <span v-if="viewer" class="text-[10px] font-normal bg-amber-400/15 text-amber-300 rounded-full px-1.5 py-px">{{ t('app.readonly') }}</span>
            </p>
            <p class="text-[11px] text-slate-500">{{ t('app.brand.sub') }}</p>
          </div>
        </div>

        <!-- 导航 -->
        <nav class="flex-1 px-3 py-4 space-y-1">
          <RouterLink
            v-for="item in visibleNav" :key="item.path" :to="item.path"
            :class="['group flex items-center rounded-xl text-sm transition-all duration-150 relative',
                     collapsed ? 'justify-center px-0 py-2.5' : 'gap-3 px-3 py-2.5',
                     isActive(item.path)
                       ? 'bg-brand-600/15 text-white font-medium shadow-[inset_0_0_0_1px_rgb(99_102_241/0.35)]'
                       : 'text-slate-400 hover:text-white hover:bg-white/5']">
            <svg viewBox="0 0 24 24" class="w-[18px] h-[18px] shrink-0 transition-colors"
                 :class="isActive(item.path) ? 'text-brand-400' : 'text-slate-500 group-hover:text-slate-300'"
                 fill="currentColor"><path :d="item.icon" /></svg>
            <template v-if="!collapsed">
              {{ t(item.i18n) }}
              <span v-if="isActive(item.path)" class="ml-auto w-1.5 h-1.5 rounded-full bg-brand-400"></span>
            </template>
            <!-- 折叠态悬浮提示 -->
            <span v-if="collapsed"
                  class="pointer-events-none absolute left-full ml-3 top-1/2 -translate-y-1/2 z-50 whitespace-nowrap
                         bg-ink-800 text-white text-xs font-medium rounded-lg px-2.5 py-1.5 shadow-pop
                         opacity-0 translate-x-1 group-hover:opacity-100 group-hover:translate-x-0 transition-all duration-150">
              {{ t(item.i18n) }}
              <i class="absolute right-full top-1/2 -translate-y-1/2 border-4 border-transparent border-r-ink-800"></i>
            </span>
          </RouterLink>
        </nav>

        <!-- 底部：外网状态 + 折叠开关 + 退出 -->
        <div class="px-3 py-4 border-t border-white/5 space-y-1">
          <!-- 外网连通状态指示（断网自治模式提示） -->
          <div v-if="uplink"
               :title="uplinkTip"
               :class="['group flex items-center rounded-xl text-sm relative select-none',
                        collapsed ? 'justify-center px-0 py-2.5' : 'gap-3 px-3 py-2.5',
                        uplink.online ? 'text-emerald-300/90' : 'text-red-300 bg-red-500/10']">
            <span class="relative flex w-[18px] h-[18px] items-center justify-center shrink-0">
              <span :class="['absolute inline-flex w-2.5 h-2.5 rounded-full', uplink.online ? 'bg-emerald-400' : 'bg-red-400']"></span>
              <span v-if="!uplink.online" class="absolute inline-flex w-2.5 h-2.5 rounded-full bg-red-400 animate-ping opacity-60"></span>
            </span>
            <template v-if="!collapsed">
              <span class="text-xs font-medium">{{ uplink.online ? t('app.uplink.online') : t('app.uplink.offline') }}</span>
              <span v-if="uplinkPending > 0" class="ml-auto text-[10px] bg-amber-400/15 text-amber-300 rounded-full px-1.5 py-px">{{ t('补发') }} {{ uplinkPending }}</span>
            </template>
            <span v-if="collapsed"
                  class="pointer-events-none absolute left-full ml-3 top-1/2 -translate-y-1/2 z-50 whitespace-pre-line
                         bg-ink-800 text-white text-xs font-medium rounded-lg px-2.5 py-1.5 shadow-pop
                         opacity-0 translate-x-1 group-hover:opacity-100 group-hover:translate-x-0 transition-all duration-150">
              {{ uplinkTip }}
              <i class="absolute right-full top-1/2 -translate-y-1/2 border-4 border-transparent border-r-ink-800"></i>
            </span>
          </div>
          <button @click="toggleLang" :title="t('app.lang')"
                  :class="['w-full flex items-center rounded-xl text-sm text-slate-400 hover:text-white hover:bg-white/5 transition',
                           collapsed ? 'justify-center px-0 py-2.5' : 'gap-3 px-3 py-2.5']">
            <svg viewBox="0 0 24 24" class="w-[18px] h-[18px]" fill="currentColor"><path d="M12 2a10 10 0 100 20 10 10 0 000-20zm8.9 6h-3a15.6 15.6 0 00-1.4-3.6A8 8 0 0118.9 8zM12 4.1c.8 1 1.6 2.4 2 3.9h-4c.4-1.5 1.2-2.9 2-3.9zM4.3 14a8 8 0 010-4h3.3a16.6 16.6 0 000 4H4.3zm.8 2h3a15.6 15.6 0 001.4 3.6A8 8 0 015.1 16zm3-8h-3a8 8 0 014.4-3.6A15.6 15.6 0 008.1 8zM12 19.9c-.8-1-1.6-2.4-2-3.9h4c-.4 1.5-1.2 2.9-2 3.9zM14.3 16H9.7a14.7 14.7 0 010-4h4.6a14.7 14.7 0 010 4zm2.2 3.6a15.6 15.6 0 001.4-3.6h3a8 8 0 01-4.4 3.6zm1.9-5.6a16.6 16.6 0 000-4h3.3a8 8 0 010 4h-3.3z"/></svg>
            <span v-if="!collapsed">{{ t('app.lang') }}</span>
          </button>
          <button @click="collapsed = !collapsed" :title="collapsed ? t('app.expand') : t('app.collapse')"
                  :class="['w-full flex items-center rounded-xl text-sm text-slate-400 hover:text-white hover:bg-white/5 transition',
                           collapsed ? 'justify-center px-0 py-2.5' : 'gap-3 px-3 py-2.5']">
            <svg viewBox="0 0 24 24" :class="['w-[18px] h-[18px] transition-transform duration-200', collapsed ? 'rotate-180' : '']" fill="currentColor">
              <path d="M15.5 4.5L8 12l7.5 7.5 1.4-1.4L10.8 12l6.1-6.1-1.4-1.4z"/>
            </svg>
            <span v-if="!collapsed">{{ t('app.collapse') }}</span>
          </button>
          <button @click="logout" :title="collapsed ? t('app.logout') : ''"
                  :class="['w-full flex items-center rounded-xl text-sm text-slate-400 hover:text-white hover:bg-white/5 transition',
                           collapsed ? 'justify-center px-0 py-2.5' : 'gap-3 px-3 py-2.5']">
            <svg viewBox="0 0 24 24" class="w-[18px] h-[18px]" fill="currentColor"><path d="M10 3h9a1 1 0 011 1v16a1 1 0 01-1 1h-9v-2h8V5h-8V3zm2 8v2H5v-2h7zm0 0l-3.5-3.5L10 8l5 4-5 4-1.5-1.5L12 11z" transform="scale(-1,1) translate(-24,0)"/></svg>
            <span v-if="!collapsed">{{ t('app.logout') }}</span>
          </button>
        </div>
      </aside>

      <main :class="['flex-1 p-8 transition-all duration-200', collapsed ? 'ml-[4.5rem] max-w-[calc(100vw-4.5rem)]' : 'ml-60 max-w-[calc(100vw-15rem)]']">
        <RouterView />
      </main>
      <AIAssistant />
    </div>
  </div>
</template>
