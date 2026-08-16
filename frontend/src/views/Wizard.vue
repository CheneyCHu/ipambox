<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api, token, role, type NICInfo } from '../api'
import NicConfigEditor from '../components/NicConfigEditor.vue'

const router = useRouter()
const step = ref(1)

// 已初始化（从设置页进入）时跳过密码步骤，直接添加网段
onMounted(async () => {
  try {
    const s = await api.setupStatus()
    if (s.initialized && token.get()) {
      step.value = 2
      loadNICs()
    }
  } catch { /* 保持第 1 步 */ }
})
const password = ref('')
const cidr = ref('')
const name = ref('')
const error = ref('')
const busy = ref(false)
const subnetId = ref(0)

// 第 2 步：本机网卡
const nics = ref<NICInfo[]>([])
const nicsLoading = ref(false)
const picked = ref<NICInfo | null>(null)
const manualMode = ref(false)
const editingNIC = ref(false)

const canNext2 = computed(() => manualMode.value ? cidr.value.includes('/') : !!picked.value)

async function next1() {
  if (password.value.length < 6) { error.value = '密码至少 6 位'; return }
  error.value = ''; busy.value = true
  try {
    const { token: t, role: r } = await api.setupInit(password.value)
    token.set(t)
    role.set(r)
    step.value = 2
    loadNICs()
  } catch (e: any) {
    // 已初始化（例如从设置页再次进入向导）：跳过密码步骤
    if (String(e.message).includes('already initialized')) {
      step.value = 2
      loadNICs()
    } else {
      error.value = e.message
    }
  } finally { busy.value = false }
}

async function loadNICs() {
  nicsLoading.value = true
  try {
    nics.value = await api.interfaces()
    if (nics.value.length === 1) pick(nics.value[0]) // 只有一块网卡时自动选中
  } catch { nics.value = [] } finally { nicsLoading.value = false }
}

function pick(nic: NICInfo) {
  picked.value = nic
  manualMode.value = false
  editingNIC.value = false
  error.value = ''
}

async function next2() {
  error.value = ''; busy.value = true
  const finalCIDR = manualMode.value ? cidr.value : picked.value!.cidr
  const finalName = name.value || (manualMode.value ? finalCIDR : `${picked.value!.name} 网段`)
  try {
    const sub = await api.createSubnet({ cidr: finalCIDR, name: finalName })
    subnetId.value = sub.id
    step.value = 3
  } catch (e: any) { error.value = e.message } finally { busy.value = false }
}

async function startScan() {
  busy.value = true
  try { await api.scanNow(subnetId.value) } catch { /* 进入后也可手动扫 */ }
  router.push('/dashboard')
}

const steps = ['设置密码', '添加网段', '开始使用']
</script>

<template>
  <div class="min-h-screen flex items-center justify-center auth-bg relative overflow-hidden">
    <div class="absolute inset-0 auth-grid"></div>
    <div class="relative bg-white rounded-2xl shadow-pop p-8 w-[32rem] animate-fade-in">
      <div class="flex items-center gap-3 mb-2">
        <span class="w-11 h-11 rounded-2xl bg-gradient-to-br from-brand-500 to-reserved flex items-center justify-center text-white shadow-pop">
          <svg viewBox="0 0 24 24" class="w-6 h-6" fill="currentColor"><path d="M12 2a10 10 0 100 20 10 10 0 000-20zM4.1 9h3.4a16 16 0 000 6H4.1A8 8 0 014.1 9zm7.9-6.9A14 14 0 0113.9 9h-3.8A14 14 0 0112 2.1zM10.1 15h3.8A14 14 0 0112 21.9 14 14 0 0110.1 15zM9.5 9a14.3 14.3 0 000 6h5a14.3 14.3 0 000-6h-5zm10.4 0h-3.4a16 16 0 010 6h3.4A8 8 0 0019.9 9z"/></svg>
        </span>
        <div>
          <h1 class="text-lg font-bold text-slate-900 tracking-tight leading-tight">欢迎使用 IPAMBox</h1>
          <p class="text-xs text-slate-400">只需 3 步即可开始使用</p>
        </div>
      </div>

      <!-- 步骤指示器 -->
      <div class="flex items-center gap-2 my-6">
        <template v-for="(label, i) in steps" :key="i">
          <div class="flex items-center gap-2">
            <span :class="['w-6 h-6 rounded-full text-xs flex items-center justify-center font-medium transition-colors',
                           step > i + 1 ? 'bg-online text-white' : step === i + 1 ? 'bg-brand-600 text-white' : 'bg-slate-100 text-slate-400']">
              <template v-if="step > i + 1">✓</template><template v-else>{{ i + 1 }}</template>
            </span>
            <span :class="['text-xs', step === i + 1 ? 'text-slate-700 font-medium' : 'text-slate-400']">{{ label }}</span>
          </div>
          <span v-if="i < steps.length - 1" :class="['flex-1 h-px', step > i + 1 ? 'bg-online' : 'bg-slate-200']"></span>
        </template>
      </div>

      <!-- 步骤 1：设置密码 -->
      <div v-if="step === 1">
        <h2 class="font-semibold text-slate-800 mb-1">设置管理员密码</h2>
        <p class="text-sm text-slate-400 mb-4">用于之后登录管理后台，请妥善保管。</p>
        <input v-model="password" type="password" placeholder="至少 6 位" @keyup.enter="next1"
               class="border border-slate-200 rounded-xl w-full px-4 py-2.5 mb-4" autofocus />
        <button @click="next1" :disabled="busy"
                class="bg-brand-600 text-white rounded-xl px-5 py-2.5 w-full font-medium hover:bg-brand-700 active:scale-[0.98] disabled:opacity-50 transition shadow-sm shadow-brand-600/30">
          {{ busy ? '请稍候…' : '下一步' }}
        </button>
      </div>

      <!-- 步骤 2：选择本机网卡 / 手动输入 -->
      <div v-else-if="step === 2">
        <h2 class="font-semibold text-slate-800 mb-1">添加你的第一个网段</h2>
        <p class="text-sm text-slate-400 mb-4">检测到本机以下网卡，选择后将自动添加对应子网：</p>

        <div v-if="nicsLoading" class="text-slate-400 text-sm py-6 text-center flex items-center justify-center gap-2">
          <svg class="w-4 h-4 animate-spin" viewBox="0 0 24 24" fill="none"><circle cx="12" cy="12" r="9" stroke="currentColor" stroke-opacity=".25" stroke-width="3"/><path d="M21 12a9 9 0 00-9-9" stroke="currentColor" stroke-width="3" stroke-linecap="round"/></svg>
          正在检测网卡…
        </div>
        <div v-else-if="!nics.length && !manualMode" class="text-slate-400 text-sm py-2 mb-2">
          未检测到可用物理网卡，请手动输入。
        </div>

        <div v-if="!manualMode" class="space-y-2 mb-4">
          <button
            v-for="nic in nics" :key="nic.name + nic.ip"
            @click="pick(nic)"
            :class="['w-full text-left border rounded-xl px-4 py-3 transition flex items-center gap-3 active:scale-[0.99]',
                     picked === nic ? 'border-brand-500 bg-brand-50/60 ring-2 ring-brand-500/20' : 'border-slate-200 hover:border-slate-300']">
            <span class="w-9 h-9 rounded-xl bg-slate-100 flex items-center justify-center text-lg shrink-0">🖧</span>
            <span class="flex-1 min-w-0">
              <span class="font-mono font-medium text-slate-800">{{ nic.name }}</span>
              <span class="font-mono text-slate-400 text-sm ml-2">{{ nic.ip }}/{{ nic.prefix }}</span>
              <span class="block text-sm text-slate-400">子网 <span class="font-mono text-brand-600">{{ nic.cidr }}</span></span>
            </span>
            <span v-if="picked === nic" class="w-5 h-5 rounded-full bg-brand-600 text-white text-xs flex items-center justify-center shrink-0">✓</span>
          </button>
        </div>

        <!-- 已选网卡：可选修改其 IP -->
        <div v-if="picked && !manualMode" class="mb-4">
          <button @click="editingNIC = !editingNIC"
                  class="text-xs text-brand-600 hover:text-brand-700 font-medium flex items-center gap-1 transition">
            <span :class="['inline-block transition-transform', editingNIC ? 'rotate-90' : '']">▸</span>
            {{ editingNIC ? '收起网络设置' : `修改 ${picked.name} 的 IP 地址` }}
          </button>
          <div v-if="editingNIC" class="mt-2 border border-slate-200 rounded-xl p-4 bg-slate-50/60">
            <NicConfigEditor :name="picked.name" family="ipv4" :currentIP="picked.ip" :currentPrefix="picked.prefix"
                             :currentMode="picked.ipv4_mode"
                             @applied="editingNIC = false; loadNICs()" />
          </div>
        </div>

        <div v-if="manualMode" class="mb-4">
          <input v-model="cidr" placeholder="如 192.168.1.0/24" class="border border-slate-200 rounded-xl w-full px-4 py-2.5 font-mono" />
        </div>

        <div class="flex gap-3">
          <button @click="next2" :disabled="busy || !canNext2"
                  class="bg-brand-600 text-white rounded-xl px-5 py-2.5 flex-1 font-medium hover:bg-brand-700 active:scale-[0.98] disabled:opacity-50 transition shadow-sm shadow-brand-600/30">
            {{ busy ? '请稍候…' : '下一步' }}
          </button>
          <button @click="manualMode = !manualMode; picked = null; error = ''"
                  class="border border-slate-200 rounded-xl px-4 py-2.5 text-sm text-slate-500 hover:border-slate-300 transition">
            {{ manualMode ? '选择网卡' : '手动输入' }}
          </button>
        </div>
      </div>

      <!-- 步骤 3：开始扫描 -->
      <div v-else class="text-center py-4">
        <div class="w-16 h-16 mx-auto mb-4 rounded-full bg-emerald-50 text-online flex items-center justify-center text-3xl">🎉</div>
        <h2 class="font-semibold text-slate-800 mb-2">一切就绪</h2>
        <p class="text-sm text-slate-400 mb-6">点击下方按钮开始第一次扫描，稍等片刻就能在地图上看到在线设备。</p>
        <button @click="startScan" :disabled="busy"
                class="bg-brand-600 text-white rounded-xl px-5 py-2.5 w-full font-medium hover:bg-brand-700 active:scale-[0.98] disabled:opacity-50 transition shadow-sm shadow-brand-600/30">
          {{ busy ? '正在启动扫描…' : '⚡ 立即开始扫描' }}
        </button>
      </div>

      <p v-if="error" class="text-conflict text-sm mt-4 text-center animate-fade-in">{{ error }}</p>
    </div>
  </div>
</template>
