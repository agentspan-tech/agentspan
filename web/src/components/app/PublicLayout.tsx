import { Outlet } from 'react-router-dom'
import { VersionBadge } from './VersionBadge'

// Wraps public (unauthenticated) pages and the create-org page so the
// VersionBadge sits at the end of the document flow — aligned to the bottom
// of the viewport on short pages, at the bottom of the content on long ones.
export function PublicLayout() {
  return (
    <div className="min-h-screen flex flex-col">
      <Outlet />
      <VersionBadge />
    </div>
  )
}
