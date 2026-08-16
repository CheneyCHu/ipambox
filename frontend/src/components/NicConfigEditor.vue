<script setup lang="ts">
import { ref, watch } from 'vue'
import { api, type NicConfig } from '../api'

/** 网卡 IPv4/IPv6 配置编辑器：静态指定或 DHCP 获取。高危操作，应用前二次确认。 */
const props = defineProps<{
  name: string            // 网卡名，如 en0
  family: 'ipv4' | 'ipv6'
  currentIP?: string      // 当前地址（展示 + 预填）
  currentPrefix?: number
  gateway?: string
  currentMode?: string    // 当前获取方式：'dhcp' / 'static' / ''（未知）
}>()
const emit = defineEmits<{ applied: [] }>()

const mode = ref<'dhcp' | 'static'>(props.currentMode === 'dhcp' ? 'dhcp' : 'static')
const ip = ref(props.currentIP ?? '')
const prefix = ref(props.currentPrefix ?? (props.family === 'ipv4' ? 24 : 64))
const gateway = ref(props.gateway ?? '')
const busy = ref(false)
const error = ref('')
const ok = ref('')
const confirming = ref(false)

watch(() => props.currentIP, v => { if (v) ip.value = v })
watch(() => props.currentMode, v => { if (v === 'dhcp' || v === 'static') mode.value = v })

async function apply() {
  error.value = ''; ok.value = ''
  const cfg: NicConfig = { family: props.family, mode: mode.value }
  if (mode.value === 'static') {
    if (!ip.value) { error.value = '请填写 IP 地址'; confirming.value = false; return }
    cfg.ip = ip.value
    cfg.prefix = prefix.value
    if (gateway.value) cfg.gateway = gateway.value
  }
  busy.value = true
  try {
    await api.configureInterface(props.name, cfg)
    ok.value = '✅ 已应用。若本机 IP 变化导致页面失联，请用新地址重新访问。'
    confirming.value = false
    emit('applied')
  } catch (e: any) {
    error.value = e.message
  } finally { busy.value = false }
}
</script>

<template>
  <div class="border border-slate-200 rounded-xl p-3 bg-slate-50/50">
    <div class="flex items-center gap-4 text-sm mb-2">
      <span class="text-slate-500">{{ family === 'ipv4' ? 'IPv4' : 'IPv6' }} 获取方式</span>
      <label class="inline-flex items-center gap-1.5 cursor-pointer">
        <input type="radio" value="dhcp" v-model="mode" class="accent-brand-600" /> 自动获取（DHCP）
      </label>
      <label class="inline-flex items-center gap-1.5 cursor-pointer">
        <input type="radio" value="static" v-model="mode" class="accent-brand-600" /> 手动指定
      </label>
    </div>

    <div v-if="mode === 'static'" class="flex flex-wrap gap-2 mb-2">
      <input v-model="ip" :placeholder="family === 'ipv4' ? 'IP 地址，如 192.168.1.50' : 'IPv6 地址'"
             class="border border-slate-200 rounded-lg px-3 py-1.5 font-mono text-sm flex-1 min-w-44 bg-white" />
      <input v-model.number="prefix" type="number" min="1" :max="family === 'ipv4' ? 32 : 128"
             class="border border-slate-200 rounded-lg px-3 py-1.5 font-mono text-sm w-24 bg-white" placeholder="前缀" />
      <input v-if="family === 'ipv4'" v-model="gateway" placeholder="网关（可选）"
             class="border border-slate-200 rounded-lg px-3 py-1.5 font-mono text-sm flex-1 min-w-36 bg-white" />
    </div>
    <p v-else class="text-xs text-slate-400 mb-2">切换为 DHCP 后，静态地址将被清除，由 DHCP 服务器分配新地址。</p>

    <div v-if="!confirming">
      <button @click="confirming = true" class="border border-slate-300 text-slate-600 rounded-lg px-3 py-1.5 text-sm hover:border-brand-500 hover:text-brand-600 active:scale-95 transition">
        应用配置…
      </button>
    </div>
    <div v-else class="bg-amber-50 border border-amber-200 rounded-lg p-2.5">
      <p class="text-xs text-amber-700 mb-2">⚠ 修改网卡地址可能导致连接中断，且需要 root 权限运行 IPAMBox。确认应用？</p>
      <div class="flex gap-2">
        <button @click="apply" :disabled="busy"
                class="bg-amber-600 text-white rounded-lg px-3 py-1.5 text-sm disabled:opacity-50 active:scale-95 transition">
          {{ busy ? '应用中…' : '确认应用' }}
        </button>
        <button @click="confirming = false" class="border border-slate-200 rounded-lg px-3 py-1.5 text-sm text-slate-500">取消</button>
      </div>
    </div>
    <p v-if="error" class="text-xs text-conflict mt-2 whitespace-pre-wrap">{{ error }}</p>
    <p v-if="ok" class="text-xs text-online mt-2">{{ ok }}</p>
  </div>
</template>
