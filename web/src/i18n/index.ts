import { create } from 'zustand'
import { en, type TranslationKey } from './en'
import { ru } from './ru'

const translations = { en, ru } as const
type Lang = 'en' | 'ru'

const STORAGE_KEY = 'agentorbit_lang'

function detectLang(): Lang {
  try {
    const stored = localStorage.getItem(STORAGE_KEY)
    if (stored === 'en' || stored === 'ru') return stored
  } catch { /* localStorage unavailable */ }
  try {
    if (navigator.language.startsWith('ru')) return 'ru'
  } catch { /* navigator unavailable */ }
  return 'en'
}

interface I18nStore {
  lang: Lang
  t: Record<TranslationKey, string>
  setLang: (lang: Lang) => void
  tt: (key: TranslationKey, params: Record<string, string | number>) => string
}

const initialLang = detectLang()

export const useI18n = create<I18nStore>((set, get) => ({
  lang: initialLang,
  t: translations[initialLang],
  setLang: (lang) => {
    try { localStorage.setItem(STORAGE_KEY, lang) } catch { /* localStorage unavailable */ }
    set({ lang, t: translations[lang] })
  },
  tt: (key, params) => {
    const { lang } = get()
    const template = translations[lang][key]
    return template.replace(/\{(\w+)\}/g, (_, k) => String(params[k] ?? ''))
  },
}))

/**
 * Pluralize a count using Russian/English plural rules.
 * Russian: one (1, 21, 31...), few (2-4, 22-24...), many (5-20, 25-30...).
 * English: one (1), many (everything else). `few` param accepted but unused.
 */
export function pluralize(count: number, one: string, few: string, many: string): string {
  const n = Math.abs(count)
  // Russian plural rules
  if (n % 10 === 1 && n % 100 !== 11) return one
  if (n % 10 >= 2 && n % 10 <= 4 && (n % 100 < 10 || n % 100 >= 20)) return few
  return many
}
