import { Outlet } from 'react-router-dom'
import { cn } from '@/lib/utils'
import { Sidebar, MobileTopBar, useSidebarState } from './Sidebar'
import { VersionBadge } from './VersionBadge'

export function AppShell() {
  const { collapsed, toggle, mobileOpen, openMobile, closeMobile } = useSidebarState()

  return (
    <div className="min-h-screen bg-zinc-950">
      <Sidebar collapsed={collapsed} onToggle={toggle} mobileOpen={mobileOpen} onMobileClose={closeMobile} />
      <div
        className={cn(
          'transition-all duration-200 min-h-screen flex flex-col',
          'md:ml-14',
          !collapsed && 'md:ml-60'
        )}
      >
        <MobileTopBar onMenuClick={openMobile} />
        <main className="flex-1 overflow-auto flex flex-col">
          <Outlet />
          <VersionBadge />
        </main>
      </div>
    </div>
  )
}
