import { useRouteError, Link } from 'react-router-dom'

export function RouteErrorBoundary() {
  const error = useRouteError()
  const message = error instanceof Error ? error.message : 'An unexpected error occurred'

  return (
    <div className="min-h-screen bg-zinc-950 flex items-center justify-center p-6">
      <div className="max-w-md w-full text-center space-y-4">
        <div className="w-12 h-12 rounded-full bg-red-500/10 flex items-center justify-center mx-auto">
          <span className="text-red-400 text-xl font-semibold">!</span>
        </div>
        <h1 className="text-lg font-semibold text-zinc-50">Something went wrong</h1>
        <p className="text-sm text-zinc-500 leading-relaxed">{message}</p>
        <div className="flex items-center justify-center gap-3 pt-2">
          <button
            onClick={() => window.location.reload()}
            className="text-sm font-medium bg-zinc-50 text-zinc-950 px-4 py-2 rounded-md hover:bg-zinc-200 transition-colors"
          >
            Reload page
          </button>
          <Link
            to="/dash"
            className="text-sm font-medium text-zinc-400 hover:text-zinc-200 transition-colors px-4 py-2"
          >
            Go to dashboard
          </Link>
        </div>
      </div>
    </div>
  )
}
