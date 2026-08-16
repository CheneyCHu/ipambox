/** 轻量 i18n：中/英双语字典 + 响应式 t()。
 *  用法：模板中 {{ t('nav.dashboard') }}，脚本中 import { t, lang, setLang } from '../i18n'
 *  语言存 localStorage（ipambox_lang），默认中文。
 */
import { ref } from 'vue'

export type Lang = 'zh' | 'en'

export const lang = ref<Lang>((localStorage.getItem('ipambox_lang') as Lang) || 'zh')

export function setLang(l: Lang) {
  lang.value = l
  localStorage.setItem('ipambox_lang', l)
}

const zh = {
  // 导航 / 框架
  'nav.dashboard': '仪表盘', 'nav.map': 'IP 地图', 'nav.subnets': '子网管理',
  'nav.network': '网络设置', 'nav.routes': '路由设置', 'nav.devices': '设备台账',
  'nav.alerts': '告警', 'nav.reports': '报表', 'nav.settings': '设置',
  'app.brand.sub': 'IP 地址管理', 'app.readonly': '只读',
  'app.collapse': '折叠菜单', 'app.expand': '展开菜单', 'app.logout': '退出登录',
  'app.uplink.online': '外网在线', 'app.uplink.offline': '离线自治中',
  'app.uplink.checking': '外网状态检测中…', 'app.uplink.pending': '待补发通知',
  'app.lang': 'English',
  // 登录
  'login.title': '欢迎回来', 'login.sub': '登录 IPAMBox 管理控制台',
  'login.password': '请输入管理密码', 'login.submit': '登 录', 'login.err': '密码错误',
  // 仪表盘
  'dash.title': '仪表盘', 'dash.sub': '全网 IP 使用情况一览',
  'dash.total': '已观测地址', 'dash.usage': '总使用率', 'dash.online': '在线设备', 'dash.alerts': '未读告警',
  'dash.offline': '离线', 'dash.conflict': '冲突',
  'dash.uplink': '网络连通状态', 'dash.check': '立即探测', 'dash.checking': '探测中…',
  'dash.probe': '探测目标', 'dash.lastProbe': '最近探测', 'dash.lastOnline': '最近在线', 'dash.offlineSince': '离线始于',
  'dash.pendingNotify': '待补发通知',
  'dash.offlineTip': '离线期间本地扫描、台账与告警记录照常工作；通知推送将暂存队列，网络恢复后自动补发。',
  'dash.onlineTag': '外网在线', 'dash.offlineTag': '离线自治中',
  'dash.recovered': '恢复在线', 'dash.lost': '离线',
  'dash.subnets': '子网列表', 'dash.enterMap': '进入 IP 地图 →', 'dash.noSubnet': '暂无子网，请先到 IP 地图页添加',
  'dash.unread': '未读告警', 'dash.allAlerts': '全部告警 →', 'dash.noAlert': '暂无告警，网络状态良好',
  'dash.loadErr': '无法连接后端服务，请确认 IPAMBox 后端已启动',
}

const en: Record<keyof typeof zh, string> = {
  'nav.dashboard': 'Dashboard', 'nav.map': 'IP Map', 'nav.subnets': 'Subnets',
  'nav.network': 'Network', 'nav.routes': 'Routes', 'nav.devices': 'Devices',
  'nav.alerts': 'Alerts', 'nav.reports': 'Reports', 'nav.settings': 'Settings',
  'app.brand.sub': 'IP Address Manager', 'app.readonly': 'Read-only',
  'app.collapse': 'Collapse', 'app.expand': 'Expand', 'app.logout': 'Sign out',
  'app.uplink.online': 'Online', 'app.uplink.offline': 'Offline (autonomous)',
  'app.uplink.checking': 'Checking connectivity…', 'app.uplink.pending': 'pending notifications',
  'app.lang': '中文',
  'login.title': 'Welcome back', 'login.sub': 'Sign in to IPAMBox console',
  'login.password': 'Admin password', 'login.submit': 'Sign in', 'login.err': 'Wrong password',
  'dash.title': 'Dashboard', 'dash.sub': 'Network-wide IP usage at a glance',
  'dash.total': 'Observed IPs', 'dash.usage': 'Total Usage', 'dash.online': 'Online Devices', 'dash.alerts': 'Unread Alerts',
  'dash.offline': 'offline', 'dash.conflict': 'conflict',
  'dash.uplink': 'Connectivity', 'dash.check': 'Check now', 'dash.checking': 'Checking…',
  'dash.probe': 'Probe target', 'dash.lastProbe': 'Last probe', 'dash.lastOnline': 'Last online', 'dash.offlineSince': 'Offline since',
  'dash.pendingNotify': 'pending notifications',
  'dash.offlineTip': 'While offline, local scanning, inventory and alerts keep working; notifications are queued and resent after recovery.',
  'dash.onlineTag': 'Online', 'dash.offlineTag': 'Offline (autonomous)',
  'dash.recovered': 'Recovered', 'dash.lost': 'Offline',
  'dash.subnets': 'Subnets', 'dash.enterMap': 'Open IP Map →', 'dash.noSubnet': 'No subnets yet — add one on the IP Map page',
  'dash.unread': 'Unread Alerts', 'dash.allAlerts': 'All alerts →', 'dash.noAlert': 'No alerts. Network is healthy.',
  'dash.loadErr': 'Cannot reach the IPAMBox backend. Please make sure it is running.',
}

const dicts: Record<Lang, Record<string, string>> = { zh, en }

export function t(key: keyof typeof zh): string {
  return dicts[lang.value][key] ?? zh[key] ?? key
}
