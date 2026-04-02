import { createBrowserRouter, RouterProvider } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { AuthGuard } from '@/components/app/AuthGuard'
import { AppShell } from '@/components/app/AppShell'
import { LoginPage } from '@/pages/public/LoginPage'
import { RegisterPage } from '@/pages/public/RegisterPage'
import { VerifyEmailPage } from '@/pages/public/VerifyEmailPage'
import { RequestPasswordResetPage } from '@/pages/public/RequestPasswordResetPage'
import { ResetPasswordPage } from '@/pages/public/ResetPasswordPage'
import { LandingPage } from '@/pages/public/LandingPage'
import { CreateOrgPage } from '@/pages/app/CreateOrgPage'
import { DashboardPage } from '@/pages/app/DashboardPage'
import { SessionsPage } from '@/pages/app/SessionsPage'
import { SessionDetailPage } from '@/pages/app/SessionDetailPage'
import { APIKeysPage } from '@/pages/app/APIKeysPage'
import { SettingsPage } from '@/pages/app/SettingsPage'
import { SystemPromptsPage } from '@/pages/app/SystemPromptsPage'
import { SystemPromptDetailPage } from '@/pages/app/SystemPromptDetailPage'
import { RouteErrorBoundary } from '@/components/app/ErrorBoundary'

const queryClient = new QueryClient({
  defaultOptions: { queries: { retry: 1, staleTime: 30_000, refetchOnWindowFocus: false } },
})

const router = createBrowserRouter([
  { path: '/', element: <LandingPage /> },
  { path: '/login', element: <LoginPage /> },
  { path: '/register', element: <RegisterPage /> },
  { path: '/verify-email', element: <VerifyEmailPage /> },
  { path: '/request-password-reset', element: <RequestPasswordResetPage /> },
  { path: '/reset-password', element: <ResetPasswordPage /> },
  {
    element: <AuthGuard />,
    errorElement: <RouteErrorBoundary />,
    children: [
      { path: '/create-org', element: <CreateOrgPage /> },
      {
        element: <AppShell />,
        errorElement: <RouteErrorBoundary />,
        children: [
          { path: '/dash', element: <DashboardPage /> },
          { path: '/sessions', element: <SessionsPage /> },
          { path: '/sessions/:id', element: <SessionDetailPage /> },
          { path: '/keys', element: <APIKeysPage /> },
          { path: '/system-prompts', element: <SystemPromptsPage /> },
          { path: '/system-prompts/:id', element: <SystemPromptDetailPage /> },
          { path: '/settings', element: <SettingsPage /> },
        ],
      },
    ],
  },
])

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}
