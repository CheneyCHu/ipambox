import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { createRouter, createWebHistory } from 'vue-router'
import App from './App.vue'
import { api, token } from './api'
import './style.css'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', redirect: '/dashboard' },
    { path: '/wizard', component: () => import('./views/Wizard.vue'), meta: { public: true } },
    { path: '/login', component: () => import('./views/Login.vue'), meta: { public: true } },
    { path: '/dashboard', component: () => import('./views/Dashboard.vue') },
    { path: '/subnets/:id?', component: () => import('./views/SubnetGrid.vue') },
    { path: '/subnet-mgr', component: () => import('./views/Subnets.vue') },
    { path: '/devices', redirect: '/subnets' }, // 设备台账已并入 IP 地图
    { path: '/alerts', component: () => import('./views/Alerts.vue') },
    { path: '/reports', component: () => import('./views/Reports.vue') },
    { path: '/network', component: () => import('./views/NetworkSettings.vue') },
    { path: '/routes', component: () => import('./views/Routes.vue') },
    { path: '/settings', component: () => import('./views/Settings.vue') },
  ],
})

// 全局守卫：未初始化 → 向导页；未登录 → 登录页
router.beforeEach(async (to) => {
  if (to.path === '/wizard') return true
  try {
    const { initialized } = await api.setupStatus()
    if (!initialized) return '/wizard'
  } catch {
    return true // 后端未就绪时放行，页面自身会报错提示
  }
  if (to.meta.public) return true
  if (!token.get()) return '/login'
  return true
})

createApp(App).use(createPinia()).use(router).mount('#app')
