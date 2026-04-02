import { Link } from 'react-router-dom'
import { useI18n } from '@/i18n'

export function LandingNav() {
  const { lang, t, setLang } = useI18n()

  return (
    <header className="fixed top-0 w-full z-50 border-b border-zinc-900 bg-zinc-950/90 backdrop-blur-sm">
      <div className="max-w-6xl mx-auto px-4 sm:px-6 h-14 flex items-center justify-between">
        <div className="flex items-center gap-2">
          <span className="text-sm font-semibold tracking-tight text-zinc-50">AgentSpan</span>
        </div>
        <nav className="hidden md:flex items-center gap-6 text-sm text-zinc-400">
          <a href="#features" className="hover:text-zinc-200 transition-colors duration-150">{t.nav_features}</a>
          <a href="#how-it-works" className="hover:text-zinc-200 transition-colors duration-150">{t.nav_how}</a>
          <a href="#pricing" className="hover:text-zinc-200 transition-colors duration-150">{t.nav_pricing}</a>
        </nav>
        <div className="flex items-center gap-3 sm:gap-5">
          {/* Lang toggle */}
          <div className="flex items-center rounded-md overflow-hidden border border-zinc-800">
            <button
              onClick={() => setLang('en')}
              className={`px-2 py-1 text-[10px] font-medium uppercase tracking-wider transition-colors ${
                lang === 'en' ? 'bg-zinc-800 text-zinc-100' : 'text-zinc-500 hover:text-zinc-300'
              }`}
            >EN</button>
            <button
              onClick={() => setLang('ru')}
              className={`px-2 py-1 text-[10px] font-medium uppercase tracking-wider transition-colors ${
                lang === 'ru' ? 'bg-zinc-800 text-zinc-100' : 'text-zinc-500 hover:text-zinc-300'
              }`}
            >RU</button>
          </div>
          <Link to="/login" className="hidden sm:inline text-sm text-zinc-400 hover:text-zinc-100 transition-colors duration-150">
            {t.nav_login}
          </Link>
          <Link
            to="/register"
            className="text-sm font-medium bg-zinc-50 text-zinc-950 px-3.5 py-1.5 rounded-md hover:bg-zinc-200 transition-colors duration-150"
          >
            {t.nav_start}
          </Link>
        </div>
      </div>
    </header>
  )
}
