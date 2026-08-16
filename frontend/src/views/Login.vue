<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { api, token, role } from '../api'
import { t, lang, setLang, type Lang } from '../i18n'

const router = useRouter()
const password = ref('')
const error = ref('')

function toggleLang() { setLang((lang.value === 'zh' ? 'en' : 'zh') as Lang) }

async function login() {
  error.value = ''
  try {
    const { token: tk, role: r } = await api.login(password.value)
    token.set(tk)
    role.set(r)
    router.push('/dashboard')
  } catch {
    error.value = t('login.err')
  }
}
</script>

<template>
  <div class="min-h-screen flex items-center justify-center auth-bg relative overflow-hidden">
    <div class="absolute inset-0 auth-grid"></div>
    <button @click="toggleLang"
            class="absolute top-5 right-6 text-xs text-slate-500 hover:text-brand-600 border border-slate-200 bg-white/70 backdrop-blur rounded-full px-3 py-1.5 transition">
      {{ t('app.lang') }}
    </button>
    <div class="relative bg-white rounded-2xl shadow-pop p-8 w-96 animate-fade-in">
      <div class="flex items-center gap-3 mb-8">
        <span class="w-11 h-11 rounded-2xl bg-gradient-to-br from-brand-500 to-reserved flex items-center justify-center text-white shadow-pop">
          <svg viewBox="0 0 24 24" class="w-6 h-6" fill="currentColor"><path d="M12 2a10 10 0 100 20 10 10 0 000-20zM4.1 9h3.4a16 16 0 000 6H4.1A8 8 0 014.1 9zm7.9-6.9A14 14 0 0113.9 9h-3.8A14 14 0 0112 2.1zM10.1 15h3.8A14 14 0 0112 21.9 14 14 0 0110.1 15zM9.5 9a14.3 14.3 0 000 6h5a14.3 14.3 0 000-6h-5zm10.4 0h-3.4a16 16 0 010 6h3.4A8 8 0 0019.9 9z"/></svg>
        </span>
        <div>
          <h1 class="text-lg font-bold text-slate-900 tracking-tight leading-tight">IPAMBox</h1>
          <p class="text-xs text-slate-400">{{ t('login.sub') }}</p>
        </div>
      </div>
      <input v-model="password" type="password" :placeholder="t('login.password')" @keyup.enter="login"
             class="border border-slate-200 rounded-xl w-full px-4 py-2.5 mb-4" autofocus />
      <button @click="login"
              class="bg-brand-600 text-white rounded-xl px-5 py-2.5 w-full font-medium hover:bg-brand-700 active:scale-[0.98] transition shadow-sm shadow-brand-600/30">{{ t('login.submit') }}</button>
      <p v-if="error" class="text-conflict text-sm mt-3 text-center animate-fade-in">{{ error }}</p>
    </div>
  </div>
</template>
