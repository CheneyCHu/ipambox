/** API 客户端：统一封装 fetch，自动携带 token，处理 401/428 跳转。 */

export interface Subnet {
  id: number
  cidr: string
  name: string
  description: string
  vlan: number
  iface: string
}

export interface NetAddr { ip: string; prefix: number }
export interface NetInterface {
  name: string
  mac: string
  up: boolean
  mtu: number
  ipv4: NetAddr[]
  ipv6: NetAddr[]
  gateway: string
  port_name: string
  ipv4_mode: 'dhcp' | 'static' | ''
}
export interface NicConfig {
  family: 'ipv4' | 'ipv6'
  mode: 'dhcp' | 'static'
  ip?: string
  prefix?: number
  gateway?: string
}

export interface RouteEntry {
  destination: string // CIDR 或 default
  gateway: string
  iface: string
  family: 'ipv4' | 'ipv6'
}

export interface NICInfo {
  name: string
  ip: string
  mask: string
  prefix: number
  cidr: string
  ipv4_mode: 'dhcp' | 'static' | ''
}

export interface DeviceRow extends IPAddress {
  subnet_cidr: string
  subnet_name: string
}

/** 带 token 下载导出文件并触发浏览器保存。 */
export async function downloadExport(subnetId: number, cidr: string) {
  const res = await fetch(`/api/v1/subnets/${subnetId}/export`, {
    headers: { Authorization: `Bearer ${token.get()}` },
  })
  if (!res.ok) throw new Error(`导出失败: ${res.status}`)
  const blob = await res.blob()
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `ipambox_${cidr.replace('/', '_')}.csv`
  a.click()
  URL.revokeObjectURL(url)
}

export interface IPAddress {
  id: number
  subnet_id: number
  ip: string
  status: 'free' | 'online' | 'offline' | 'conflict' | 'reserved' | 'rogue'
  mac?: string
  vendor?: string
  hostname?: string
  label?: string
  owner?: string
  dev_type?: string
  last_seen?: string
}

export interface Alert {
  id: number
  type: string
  level: string
  message: string
  ip?: string
  read: boolean
  created_at: string
}

export interface UplinkStatus {
  online: boolean
  since: string
  last_online: string
  probe: string
  detail: string
  interval_sec: number
}

export interface UplinkEvent {
  id: number
  online: boolean
  detail: string
  created_at: string
}

export interface Overview {
  subnets: number
  usage_pct: number
  unread_alerts: number
  stats: { total: number; online: number; offline: number; free: number; conflict: number; rogue: number }
}

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message)
  }
}

export const token = {
  get: () => localStorage.getItem('ipambox_token') ?? '',
  set: (t: string) => localStorage.setItem('ipambox_token', t),
  clear: () => localStorage.removeItem('ipambox_token'),
}

// 当前登录角色：admin / viewer（只读）。viewer 隐藏所有写操作入口。
export const role = {
  get: () => localStorage.getItem('ipambox_role') ?? 'admin',
  set: (r: string) => localStorage.setItem('ipambox_role', r),
  clear: () => localStorage.removeItem('ipambox_role'),
}
export const isViewer = () => role.get() === 'viewer'

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`/api/v1${path}`, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      ...(token.get() ? { Authorization: `Bearer ${token.get()}` } : {}),
      ...(init?.headers ?? {}),
    },
  })
  if (res.status === 401) {
    token.clear()
    location.href = '/login'
    throw new ApiError(401, 'unauthorized')
  }
  if (res.status === 428) {
    location.href = '/wizard'
    throw new ApiError(428, 'not_initialized')
  }
  if (!res.ok) {
    // 后端错误体为 {"error":"…"}，提取可读信息而非展示整段 JSON
    const raw = await res.text()
    let msg = raw
    try { const j = JSON.parse(raw); if (j && typeof j.error === 'string') msg = j.error } catch { /* 非 JSON 原样展示 */ }
    throw new ApiError(res.status, msg)
  }
  return res.json()
}

export const api = {
  // 认证 / 向导
  setupStatus: () => req<{ initialized: boolean }>('/setup/status'),
  setupInit: (password: string) => req<{ token: string; role: string }>('/setup/init', { method: 'POST', body: JSON.stringify({ password }) }),
  login: (password: string) => req<{ token: string; role: string }>('/setup/login', { method: 'POST', body: JSON.stringify({ password }) }),
  setupReset: () => req('/setup/reset', { method: 'POST' }),
  authMe: () => req<{ role: string }>('/auth/me'),
  viewerStatus: () => req<{ enabled: boolean }>('/auth/viewer'),
  setViewer: (password: string) => req('/auth/viewer', { method: 'POST', body: JSON.stringify({ password }) }),

  overview: () => req<Overview>('/stats/overview'),
  interfaces: () => req<NICInfo[]>('/interfaces'),
  netInterfaces: () => req<NetInterface[]>('/network/interfaces/'),
  configureInterface: (name: string, cfg: NicConfig) =>
    req(`/network/interfaces/${name}/config`, { method: 'POST', body: JSON.stringify(cfg) }),
  routes: () => req<RouteEntry[]>('/network/routes/'),
  addRoute: (r: { destination: string; gateway: string; iface?: string }) =>
    req('/network/routes/', { method: 'POST', body: JSON.stringify(r) }),
  deleteRoute: (r: { destination: string; gateway?: string }) =>
    req('/network/routes/', { method: 'DELETE', body: JSON.stringify(r) }),
  notifyTest: () => req('/notify/test', { method: 'POST' }),
  backupImport: async (file: File) => {
    const res = await fetch('/api/v1/backup/import', {
      method: 'POST',
      headers: { Authorization: `Bearer ${token.get()}`, 'Content-Type': 'application/octet-stream' },
      body: file,
    })
    if (!res.ok) {
    // 后端错误体为 {"error":"…"}，提取可读信息而非展示整段 JSON
    const raw = await res.text()
    let msg = raw
    try { const j = JSON.parse(raw); if (j && typeof j.error === 'string') msg = j.error } catch { /* 非 JSON 原样展示 */ }
    throw new ApiError(res.status, msg)
  }
    return res.json()
  },
  subnets: () => req<Subnet[]>('/subnets/'),
  createSubnet: (s: Partial<Subnet>) => req<Subnet>('/subnets/', { method: 'POST', body: JSON.stringify(s) }),
  updateSubnet: (id: number, s: Partial<Subnet>) => req(`/subnets/${id}`, { method: 'PUT', body: JSON.stringify(s) }),
  deleteSubnet: (id: number) => req(`/subnets/${id}`, { method: 'DELETE' }),
  addresses: (id: number) => req<IPAddress[]>(`/subnets/${id}/addresses`),
  createAddress: (subnetId: number, a: Partial<IPAddress>) =>
    req<IPAddress>(`/subnets/${subnetId}/addresses`, { method: 'POST', body: JSON.stringify(a) }),
  deleteAddress: (id: number) => req(`/addresses/${id}`, { method: 'DELETE' }),
  getSettings: () => req<Record<string, string>>('/settings/'),
  saveSettings: (s: Record<string, string>) => req('/settings/', { method: 'PUT', body: JSON.stringify(s) }),
  renameDictItem: (r: { kind: 'dev_types' | 'owners'; from: string; to: string }) =>
    req<{ updated: number }>('/settings/dict/rename', { method: 'POST', body: JSON.stringify(r) }),
  stats: (id: number) =>
    req<{ total: number; online: number; offline: number; free: number; conflict: number; usage_pct: number }>(`/subnets/${id}/stats`),
  scanNow: (id: number) => req(`/subnets/${id}/scan`, { method: 'POST' }),
  annotate: (id: number, a: { label: string; owner: string; dev_type: string }) =>
    req(`/addresses/${id}`, { method: 'PATCH', body: JSON.stringify(a) }),
  devices: () => req<DeviceRow[]>('/devices'),
  importCSV: (id: number, csvText: string) =>
    req<{ updated: number; skipped: number }>(`/subnets/${id}/import`, {
      method: 'POST',
      headers: { 'Content-Type': 'text/csv' },
      body: csvText,
    }),
  alerts: (unread = false) => req<Alert[]>(`/alerts/${unread ? '?unread=1' : ''}`),
  markAlertRead: (id: number) => req(`/alerts/${id}/read`, { method: 'POST' }),

  // 外网连通状态（断网续存 / 边缘自治）
  uplink: () =>
    req<{ status: UplinkStatus; pending: number; events: UplinkEvent[] }>('/uplink/'),
  uplinkEvents: (limit = 50) => req<UplinkEvent[]>(`/uplink/events?limit=${limit}`),
  uplinkCheck: () => req<{ status: UplinkStatus; pending: number }>('/uplink/check', { method: 'POST' }),

  // OTA 在线升级
  version: () => req<{ version: string }>('/version'),
  updateCheck: () =>
    req<{ current: string; latest: string; has_update: boolean; notes: string }>('/update/check'),
  updateApply: () => req<{ status: string; version: string }>('/update/apply', { method: 'POST' }),

  aiChat: (message: string) => req<{ reply: string }>('/ai/chat', { method: 'POST', body: JSON.stringify({ message }) }),
  aiSaveConfig: (c: { base_url: string; model: string; api_key: string }) =>
    req('/ai/config', { method: 'POST', body: JSON.stringify(c) }),
  aiTest: () => req<{ reply: string }>('/ai/test', { method: 'POST' }),
}
