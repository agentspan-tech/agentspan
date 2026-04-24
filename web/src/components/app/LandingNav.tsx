import { Link } from 'react-router-dom'
import { useI18n } from '@/i18n'

const GITHUB_URL = 'https://github.com/agentorbit-tech/agentorbit'

function GithubIcon({ size = 14 }: { size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 16 16"
      fill="currentColor"
      aria-hidden="true"
    >
      <path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0016 8c0-4.42-3.58-8-8-8z" />
    </svg>
  )
}

export function LandingNav() {
  const { lang, t, setLang } = useI18n()

  return (
    <header className="fixed top-0 w-full z-50 border-b border-zinc-900 bg-zinc-950/90 backdrop-blur-sm">
      <div className="max-w-6xl mx-auto px-4 sm:px-6 h-14 flex items-center justify-between">
        <div className="flex items-center gap-2">
          <img src="/logo.png" alt="AgentOrbit" className="w-5 h-5" />
          <span className="text-sm font-semibold tracking-tight text-zinc-50">AgentOrbit</span>
        </div>
        <nav className="hidden md:flex items-center gap-6 text-sm text-zinc-400">
          <a href="#features" className="hover:text-zinc-200 transition-colors duration-150">{t.nav_features}</a>
          <a href="#how-it-works" className="hover:text-zinc-200 transition-colors duration-150">{t.nav_how}</a>
          <a href="#pricing" className="hover:text-zinc-200 transition-colors duration-150">{t.nav_pricing}</a>
          <a
            href={GITHUB_URL}
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-1.5 hover:text-zinc-200 transition-colors duration-150"
          >
            <GithubIcon size={14} />
            {t.nav_github}
          </a>
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
          <a
            href={GITHUB_URL}
            target="_blank"
            rel="noopener noreferrer"
            aria-label="GitHub"
            className="md:hidden text-zinc-400 hover:text-zinc-100 transition-colors duration-150"
          >
            <GithubIcon size={16} />
          </a>
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
