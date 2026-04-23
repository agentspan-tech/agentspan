import { useRouteError, isRouteErrorResponse, Link } from 'react-router-dom'
import { useI18n } from '@/i18n'

export function RouteErrorBoundary() {
  const error = useRouteError()
  const { t } = useI18n()

  const is404 = isRouteErrorResponse(error) && error.status === 404
  const title = is404 ? t.app_not_found_title : t.app_error_title
  const description = is404
    ? t.app_not_found_description
    : error instanceof Error
      ? error.message
      : t.app_error_description_fallback

  return (
    <div className="min-h-screen bg-zinc-950 flex items-center justify-center p-6">
      <div className="max-w-md w-full text-center space-y-4">
        <div className="w-12 h-12 rounded-full bg-red-500/10 flex items-center justify-center mx-auto">
          <span className="text-red-400 text-xl font-semibold">!</span>
        </div>
        <h1 className="text-lg font-semibold text-zinc-50">{title}</h1>
        <p className="text-sm text-zinc-500 leading-relaxed">{description}</p>
        <div className="flex items-center justify-center gap-3 pt-2">
          {!is404 && (
            <button
              onClick={() => window.location.reload()}
              className="text-sm font-medium bg-zinc-50 text-zinc-950 px-4 py-2 rounded-md hover:bg-zinc-200 transition-colors"
            >
              {t.app_error_reload}
            </button>
          )}
          <Link
            to="/"
            className="text-sm font-medium text-zinc-400 hover:text-zinc-200 transition-colors px-4 py-2"
          >
            {t.app_error_back_home}
          </Link>
        </div>
      </div>
    </div>
  )
}

export function NotFoundPage() {
  const { t } = useI18n()
  return (
    <div className="min-h-screen bg-zinc-950 flex items-center justify-center p-6">
      <div className="max-w-md w-full text-center space-y-4">
        <h1 className="text-lg font-semibold text-zinc-50">{t.app_not_found_title}</h1>
        <p className="text-sm text-zinc-500 leading-relaxed">{t.app_not_found_description}</p>
        <div className="pt-2">
          <Link
            to="/"
            className="text-sm font-medium bg-zinc-50 text-zinc-950 px-4 py-2 rounded-md hover:bg-zinc-200 transition-colors inline-block"
          >
            {t.app_error_back_home}
          </Link>
        </div>
      </div>
    </div>
  )
}
