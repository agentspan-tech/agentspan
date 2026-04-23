import { useState, useEffect, useCallback } from 'react'
import { NavLink, useLocation } from 'react-router-dom'
import {
  LayoutDashboard,
  List,
  Key,
  FileText,
  AlertTriangle,
  Settings,
  ChevronLeft,
  ChevronRight,
  Menu,
  X,
  LogOut,
} from 'lucide-react'
import { cn } from '@/lib/utils'
import { useI18n } from '@/i18n'
import { OrgSwitcher } from './OrgSwitcher'
import { UsageBar } from './UsageBar'
import { useWSStore } from '@/hooks/use-websocket'
import { useAuthStore } from '@/store'

interface NavItem {
  path: string
  label: string
  icon: React.ComponentType<{ className?: string; size?: number }>
}

interface SidebarProps {
  collapsed: boolean
  onToggle: () => void
  mobileOpen: boolean
  onMobileClose: () => void
}

function SidebarContent({ collapsed, onToggle, onNavClick }: { collapsed: boolean; onToggle: () => void; onNavClick?: () => void }) {
  const { t } = useI18n()
  const location = useLocation()
  const wsStatus = useWSStore((s) => s.status)
  const logout = useAuthStore((s) => s.logout)

  const navItems: NavItem[] = [
    { path: '/dash', label: t.sidebar_dashboard, icon: LayoutDashboard },
    { path: '/sessions', label: t.sidebar_sessions, icon: List },
    { path: '/keys', label: t.sidebar_keys, icon: Key },
    { path: '/system-prompts', label: t.sidebar_system_prompts, icon: FileText },
    { path: '/failure-clusters', label: t.sidebar_failure_clusters, icon: AlertTriangle },
    { path: '/settings', label: t.sidebar_settings, icon: Settings },
  ]

  return (
    <>
      {/* Logo */}
      <div className={cn('flex items-center h-14 px-5 border-b border-zinc-900', collapsed && 'px-0 justify-center')}>
        {collapsed ? (
          <img src="/logo.png" alt="AgentOrbit" className="w-5 h-5" />
        ) : (
          <div className="flex items-center gap-2">
            <img src="/logo.png" alt="AgentOrbit" className="w-5 h-5" />
            <span className="text-sm font-semibold text-zinc-50 tracking-tight">AgentOrbit</span>
          </div>
        )}
      </div>

      {/* Org switcher */}
      {!collapsed && (
        <div className="px-3 pt-3 pb-1">
          <OrgSwitcher />
        </div>
      )}

      {/* Nav items */}
      <nav className="flex-1 px-3 py-4 space-y-0.5">
        {navItems.map(({ path, label, icon: Icon }) => {
          const isActive = location.pathname === path || location.pathname.startsWith(path + '/')
          return (
            <NavLink
              key={path}
              to={path}
              title={collapsed ? label : undefined}
              onClick={onNavClick}
              className={cn(
                'flex items-center gap-2.5 px-3 py-2 rounded-md text-sm transition-all duration-150',
                isActive
                  ? 'bg-indigo-500/[0.08] text-zinc-50 nav-active-indicator'
                  : 'text-zinc-500 hover:text-zinc-300 hover:bg-zinc-800/50',
                collapsed && 'justify-center px-0'
              )}
            >
              <Icon size={15} className={isActive ? 'text-indigo-400' : 'text-zinc-600'} />
              {!collapsed && <span>{label}</span>}
            </NavLink>
          )
        })}
      </nav>

      {/* Usage bar (Free plan only) */}
      <UsageBar collapsed={collapsed} />

      {/* Bottom controls */}
      <div className="px-3 py-4 border-t border-zinc-900 space-y-2">
        {/* Logout */}
        <button
          onClick={() => {
            logout()
            window.location.href = '/login'
          }}
          title={t.sidebar_logout}
          className={cn(
            'flex items-center gap-2 text-xs text-zinc-600 hover:text-red-400 transition-colors duration-150 px-3 py-1.5 w-full',
            collapsed && 'justify-center px-0'
          )}
        >
          <LogOut size={14} />
          {!collapsed && <span>{t.sidebar_logout}</span>}
        </button>

        {/* Toggle — desktop only */}
        <button
          onClick={onToggle}
          title={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
          className={cn(
            'hidden md:flex items-center gap-2 text-xs text-zinc-600 hover:text-zinc-300 transition-colors duration-150 px-3 py-1.5',
            collapsed && 'justify-center px-0'
          )}
        >
          {collapsed ? <ChevronRight size={14} /> : <ChevronLeft size={14} />}
        </button>

        {/* WS status */}
        <div
          title={wsStatus === 'connected' ? t.sidebar_ws_connected : wsStatus === 'connecting' ? t.sidebar_ws_reconnecting : t.sidebar_ws_offline}
          className={cn('flex items-center gap-2 px-3', collapsed && 'justify-center px-0')}
        >
          <span
            className={cn(
              'w-1.5 h-1.5 rounded-full shrink-0',
              wsStatus === 'connected' && 'bg-emerald-500',
              wsStatus === 'connecting' && 'bg-zinc-400 animate-pulse',
              wsStatus === 'disconnected' && 'bg-zinc-600'
            )}
          />
          {!collapsed && (
            <span className="text-xs text-zinc-600">
              {wsStatus === 'connected' ? t.sidebar_ws_connected : wsStatus === 'disconnected' ? t.sidebar_ws_offline : t.sidebar_ws_reconnecting}
            </span>
          )}
        </div>
      </div>
    </>
  )
}

export function Sidebar({ collapsed, onToggle, mobileOpen, onMobileClose }: SidebarProps) {
  const location = useLocation()

  // Close mobile drawer on route change
  useEffect(() => {
    onMobileClose()
  }, [location.pathname, onMobileClose])

  return (
    <>
      {/* Desktop sidebar */}
      <aside
        className={cn(
          'hidden md:flex fixed left-0 top-0 h-screen border-r border-zinc-900 flex-col bg-zinc-950 z-40 transition-all duration-200',
          collapsed ? 'w-14' : 'w-60'
        )}
      >
        <SidebarContent collapsed={collapsed} onToggle={onToggle} />
      </aside>

      {/* Mobile overlay backdrop */}
      {mobileOpen && (
        <div
          className="md:hidden fixed inset-0 bg-black/60 z-40 animate-fade-in"
          onClick={onMobileClose}
        />
      )}

      {/* Mobile drawer */}
      <aside
        className={cn(
          'md:hidden fixed left-0 top-0 h-screen w-60 border-r border-zinc-900 flex flex-col bg-zinc-950 z-50 transition-transform duration-200',
          mobileOpen ? 'translate-x-0' : '-translate-x-full'
        )}
      >
        {/* Close button in mobile drawer */}
        <div className="absolute right-2 top-3 z-10">
          <button
            onClick={onMobileClose}
            className="p-1.5 text-zinc-500 hover:text-zinc-200 transition-colors"
          >
            <X size={16} />
          </button>
        </div>
        <SidebarContent collapsed={false} onToggle={onToggle} onNavClick={onMobileClose} />
      </aside>
    </>
  )
}

export function MobileTopBar({ onMenuClick }: { onMenuClick: () => void }) {
  return (
    <div className="md:hidden sticky top-0 z-30 flex items-center h-12 px-4 border-b border-zinc-900 bg-zinc-950">
      <button onClick={onMenuClick} className="p-1.5 -ml-1.5 text-zinc-400 hover:text-zinc-200 transition-colors">
        <Menu size={18} />
      </button>
      <img src="/logo.png" alt="AgentOrbit" className="ml-3 w-5 h-5" />
      <span className="ml-1.5 text-sm font-semibold text-zinc-50 tracking-tight">AgentOrbit</span>
    </div>
  )
}

export function useSidebarState() {
  const [collapsed, setCollapsed] = useState(false)
  const [mobileOpen, setMobileOpen] = useState(false)
  const toggle = useCallback(() => setCollapsed((c) => !c), [])
  const openMobile = useCallback(() => setMobileOpen(true), [])
  const closeMobile = useCallback(() => setMobileOpen(false), [])
  return { collapsed, toggle, mobileOpen, openMobile, closeMobile }
}
