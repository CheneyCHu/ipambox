<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api, type NetInterface } from '../api'
import { t } from '../i18n'
import NicConfigEditor from '../components/NicConfigEditor.vue'

const ifaces = ref<NetInterface[]>([])
const loadError = ref('')
const editing = ref('') // 当前展开编辑器的网卡名

// 预填用：优先取全球单播 IPv6，跳过链路本地（fe80::/带 %zone 不可作为静态配置）
function globalIPv6(nic: NetInterface) {
  return nic.ipv6?.find(a => !a.ip.startsWith('fe80') && !a.ip.includes('%'))
}

async function load() {
  loadError.value = ''
  try {
    ifaces.value = await api.netInterfaces()
  } catch (e: any) { loadError.value = t('加载失败：') + e.message }
}
onMounted(load)
</script>

<template>
  <div class="animate-fade-in max-w-3xl">
    <header class="mb-5">
      <h1 class="text-2xl font-bold text-slate-900 tracking-tight">{{ t('网络设置') }}</h1>
      <p class="text-sm text-slate-400 mt-1">{{ t('查看并配置本机物理网卡的 IPv4 / IPv6 地址（修改需要 root 权限）') }}</p>
    </header>

    <div v-if="loadError" class="bg-red-50 border border-red-200 text-conflict rounded-xl px-4 py-2.5 mb-4 text-sm">{{ loadError }}</div>

    <div v-for="nic in ifaces" :key="nic.name" class="bg-white rounded-2xl shadow-card border border-slate-100 p-5 mb-4">
      <div class="flex items-center gap-3 mb-3">
        <div class="w-10 h-10 rounded-xl bg-gradient-to-br from-brand-100 to-purple-100 flex items-center justify-center text-lg">🖧</div>
        <div class="flex-1">
          <p class="font-mono font-semibold text-slate-800">{{ nic.name }}
            <span v-if="nic.port_name && nic.port_name !== nic.name" class="text-xs font-sans font-normal text-slate-400 ml-1">{{ nic.port_name }}</span>
            <span :class="nic.up ? 'bg-emerald-100 text-online' : 'bg-slate-100 text-slate-400'"
                  class="text-xs font-sans font-medium rounded-full px-2 py-0.5 ml-2">{{ nic.up ? t('已启用') : t('未启用') }}</span>
            <span v-if="nic.ipv4_mode"
                  :class="['text-xs font-sans font-medium rounded-full px-2 py-0.5 ml-1.5',
                           nic.ipv4_mode === 'dhcp' ? 'bg-sky-100 text-sky-600' : 'bg-amber-100 text-amber-600']">
              {{ nic.ipv4_mode === 'dhcp' ? 'DHCP' : t('手动') }}
            </span>
          </p>
          <p class="text-xs text-slate-400 font-mono">{{ nic.mac || t('无 MAC') }} · MTU {{ nic.mtu }}</p>
        </div>
        <button @click="editing = editing === nic.name ? '' : nic.name"
                class="border border-slate-200 rounded-xl px-4 py-1.5 text-sm text-slate-600 hover:border-brand-500 hover:text-brand-600 active:scale-95 transition">
          {{ editing === nic.name ? t('收起') : t('配置地址') }}
        </button>
      </div>

      <dl class="grid grid-cols-1 sm:grid-cols-2 gap-2 text-sm mb-2">
        <div class="bg-slate-50 rounded-lg px-3 py-2">
          <dt class="text-xs text-slate-400">IPv4</dt>
          <dd class="font-mono text-slate-700">
            <span v-if="!nic.ipv4?.length" class="text-slate-300">—</span>
            <span v-for="a in nic.ipv4" :key="a.ip" class="block">{{ a.ip }}/{{ a.prefix }}</span>
          </dd>
        </div>
        <div class="bg-slate-50 rounded-lg px-3 py-2">
          <dt class="text-xs text-slate-400">IPv6</dt>
          <dd class="font-mono text-slate-700 break-all">
            <span v-if="!nic.ipv6?.length" class="text-slate-300">—</span>
            <span v-for="a in nic.ipv6" :key="a.ip" class="block">{{ a.ip }}/{{ a.prefix }}</span>
          </dd>
        </div>
      </dl>
      <p v-if="nic.gateway" class="text-xs text-slate-400 font-mono mb-2">{{ t('默认网关：') }}{{ nic.gateway }}</p>

      <div v-if="editing === nic.name" class="space-y-2 animate-fade-in">
        <NicConfigEditor :name="nic.name" family="ipv4"
                         :currentIP="nic.ipv4?.[0]?.ip" :currentPrefix="nic.ipv4?.[0]?.prefix"
                         :gateway="nic.gateway" :currentMode="nic.ipv4_mode" @applied="load" />
        <NicConfigEditor :name="nic.name" family="ipv6"
                         :currentIP="globalIPv6(nic)?.ip" :currentPrefix="globalIPv6(nic)?.prefix"
                         @applied="load" />
      </div>
    </div>

    <p v-if="!ifaces.length && !loadError" class="text-slate-400 text-center py-16">{{ t('未检测到物理网卡') }}</p>
  </div>
</template>
