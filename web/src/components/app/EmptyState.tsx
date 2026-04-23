import { Link } from 'react-router-dom'
import { cn } from '@/lib/utils'

interface EmptyStateAction {
  label: string
  onClick?: () => void
  href?: string
}

interface EmptyStateProps {
  heading: string
  body: string
  action?: EmptyStateAction
  className?: string
}

export function EmptyState({ heading, body, action, className }: EmptyStateProps) {
  return (
    <div className={cn('flex flex-col items-center justify-center text-center py-16 relative', className)}>
      {/* Subtle decorative element */}
      <div className="mb-5 relative">
        <div className="w-10 h-10 rounded-lg border border-zinc-800 bg-zinc-900 flex items-center justify-center">
          <div className="w-3 h-3 rounded-sm border border-dashed border-zinc-700" />
        </div>
        <div className="absolute -top-1 -right-1 w-2 h-2 rounded-full bg-zinc-800" />
      </div>
      <h2 className="text-base font-medium text-zinc-200">{heading}</h2>
      <p className="text-sm text-zinc-500 mt-2 max-w-md leading-relaxed">{body}</p>
      {action && (
        <div className="mt-6">
          {action.href ? (
            <Link
              to={action.href}
              className="inline-flex items-center justify-center text-sm font-medium bg-zinc-800 text-zinc-200 px-4 py-2 rounded-md hover:bg-zinc-700 transition-colors duration-150 btn-press"
            >
              {action.label}
            </Link>
          ) : (
            <button
              onClick={action.onClick}
              className="inline-flex items-center justify-center text-sm font-medium bg-zinc-800 text-zinc-200 px-4 py-2 rounded-md hover:bg-zinc-700 transition-colors duration-150 btn-press"
            >
              {action.label}
            </button>
          )}
        </div>
      )}
    </div>
  )
}
