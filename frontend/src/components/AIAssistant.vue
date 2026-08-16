<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import { api } from '../api'
import { t, lang } from '../i18n'

const open = ref(false)
const input = ref('')
const thinking = ref(false)
const listEl = ref<HTMLElement | null>(null)
const messages = ref<{ role: 'user' | 'ai'; text: string }[]>([])

function greeting() {
  return t('你好，我是网络运维助手。可以问我："财务部还有哪些空闲 IP？"')
}
messages.value = [{ role: 'ai', text: greeting() }]
// 切换语言时，若还只有开场白则一并替换
watch(lang, () => {
  if (messages.value.length === 1 && messages.value[0].role === 'ai') messages.value[0].text = greeting()
})

async function scrollBottom() {
  await nextTick()
  listEl.value?.scrollTo({ top: listEl.value.scrollHeight, behavior: 'smooth' })
}

async function send() {
  const q = input.value.trim()
  if (!q || thinking.value) return
  messages.value.push({ role: 'user', text: q })
  input.value = ''
  scrollBottom()
  thinking.value = true
  try {
    const res = await api.aiChat(q)
    messages.value.push({ role: 'ai', text: res.reply })
  } catch {
    messages.value.push({ role: 'ai', text: t('AI 网关尚未配置，请先在 设置 → AI 助手 中填写 API 信息。') })
  } finally {
    thinking.value = false
    scrollBottom()
  }
}
</script>

<template>
  <div class="fixed bottom-5 right-5 z-50">
    <!-- 悬浮按钮 -->
    <button v-if="!open" @click="open = true"
            class="group bg-gradient-to-br from-brand-600 to-reserved text-white rounded-full pl-4 pr-5 py-3 shadow-pop flex items-center gap-2 hover:scale-105 active:scale-95 transition-transform">
      <span class="w-7 h-7 rounded-full bg-white/20 flex items-center justify-center">✨</span>
      <span class="text-sm font-medium">{{ t('AI 助手') }}</span>
    </button>

    <!-- 聊天窗口 -->
    <div v-else class="bg-white rounded-2xl shadow-pop w-96 flex flex-col overflow-hidden animate-fade-in border border-slate-100" style="height: 30rem">
      <header class="px-4 py-3 bg-gradient-to-r from-brand-600 to-reserved text-white flex items-center gap-3">
        <span class="w-8 h-8 rounded-full bg-white/20 flex items-center justify-center shrink-0">✨</span>
        <div class="flex-1 leading-tight">
          <p class="text-sm font-semibold">{{ t('AI 网络运维助手') }}</p>
          <p class="text-[11px] text-white/70">{{ thinking ? t('正在思考…') : t('在线') }}</p>
        </div>
        <button @click="open = false" class="w-7 h-7 rounded-lg hover:bg-white/20 transition text-white/80">✕</button>
      </header>

      <div ref="listEl" class="flex-1 overflow-y-auto p-4 space-y-4 bg-slate-50/60">
        <div v-for="(m, i) in messages" :key="i" :class="['flex gap-2.5', m.role === 'user' ? 'flex-row-reverse' : '']">
          <span v-if="m.role === 'ai'" class="w-7 h-7 rounded-full bg-gradient-to-br from-brand-500 to-reserved text-white text-xs flex items-center justify-center shrink-0 mt-0.5">✨</span>
          <span v-else class="w-7 h-7 rounded-full bg-slate-200 text-slate-500 text-xs flex items-center justify-center shrink-0 mt-0.5">{{ t('我') }}</span>
          <span :class="m.role === 'user'
                  ? 'bg-brand-600 text-white rounded-2xl rounded-tr-md'
                  : 'bg-white border border-slate-100 shadow-sm text-slate-700 rounded-2xl rounded-tl-md'"
                class="inline-block px-3.5 py-2.5 text-sm max-w-[78%] text-left leading-relaxed whitespace-pre-wrap">{{ m.text }}</span>
        </div>

        <!-- 打字中三点动画 -->
        <div v-if="thinking" class="flex gap-2.5">
          <span class="w-7 h-7 rounded-full bg-gradient-to-br from-brand-500 to-reserved text-white text-xs flex items-center justify-center shrink-0 mt-0.5">✨</span>
          <span class="bg-white border border-slate-100 shadow-sm rounded-2xl rounded-tl-md px-4 py-3 flex items-center gap-1.5">
            <i class="w-1.5 h-1.5 rounded-full bg-slate-400 animate-typing-1"></i>
            <i class="w-1.5 h-1.5 rounded-full bg-slate-400 animate-typing-2"></i>
            <i class="w-1.5 h-1.5 rounded-full bg-slate-400 animate-typing-3"></i>
          </span>
        </div>
      </div>

      <footer class="p-3 border-t border-slate-100 flex gap-2 bg-white">
        <input v-model="input" @keyup.enter="send" :placeholder="t('输入问题，回车发送…')"
               class="border border-slate-200 rounded-xl flex-1 px-3.5 py-2 text-sm" />
        <button @click="send" :disabled="thinking"
                class="bg-brand-600 text-white rounded-xl w-10 flex items-center justify-center hover:bg-brand-700 active:scale-95 disabled:opacity-50 transition">
          <svg viewBox="0 0 24 24" class="w-4 h-4" fill="currentColor"><path d="M3 11l18-8-8 18-2.5-7.5L3 11z"/></svg>
        </button>
      </footer>
    </div>
  </div>
</template>
