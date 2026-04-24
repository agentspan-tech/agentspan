import { createBrowserRouter, RouterProvider, Outlet } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { AuthGuard } from '@/components/app/AuthGuard'
import { AppShell } from '@/components/app/AppShell'
import { PublicLayout } from '@/components/app/PublicLayout'
import { LoginPage } from '@/pages/public/LoginPage'
import { RegisterPage } from '@/pages/public/RegisterPage'
import { VerifyEmailPage } from '@/pages/public/VerifyEmailPage'
import { RequestPasswordResetPage } from '@/pages/public/RequestPasswordResetPage'
import { ResetPasswordPage } from '@/pages/public/ResetPasswordPage'
import { LandingPage } from '@/pages/public/LandingPage'
import { PrivacyPolicyPage } from '@/pages/public/PrivacyPolicyPage'
import { TermsPage } from '@/pages/public/TermsPage'
import { ConsentPage } from '@/pages/public/ConsentPage'
import { CreateOrgPage } from '@/pages/app/CreateOrgPage'
import { DashboardPage } from '@/pages/app/DashboardPage'
import { SessionsPage } from '@/pages/app/SessionsPage'
import { SessionDetailPage } from '@/pages/app/SessionDetailPage'
import { APIKeysPage } from '@/pages/app/APIKeysPage'
import { SettingsPage } from '@/pages/app/SettingsPage'
import { SystemPromptsPage } from '@/pages/app/SystemPromptsPage'
import { SystemPromptDetailPage } from '@/pages/app/SystemPromptDetailPage'
import { FailureClustersPage } from '@/pages/app/FailureClustersPage'
import { RouteErrorBoundary, NotFoundPage } from '@/components/app/ErrorBoundary'

const queryClient = new QueryClient({
  defaultOptions: { queries: { retry: 1, staleTime: 30_000, refetchOnWindowFocus: false } },
})

const router = createBrowserRouter([
  {
    element: <Outlet />,
    errorElement: <RouteErrorBoundary />,
    children: [
      {
        element: <PublicLayout />,
        children: [
          { path: '/', element: <LandingPage /> },
          { path: '/login', element: <LoginPage /> },
          { path: '/register', element: <RegisterPage /> },
          { path: '/verify-email', element: <VerifyEmailPage /> },
          { path: '/request-password-reset', element: <RequestPasswordResetPage /> },
          { path: '/reset-password', element: <ResetPasswordPage /> },
          { path: '/privacy-policy', element: <PrivacyPolicyPage /> },
          { path: '/terms', element: <TermsPage /> },
          { path: '/consent', element: <ConsentPage /> },
          { path: '*', element: <NotFoundPage /> },
        ],
      },
      {
        element: <AuthGuard />,
        children: [
          {
            element: <PublicLayout />,
            children: [{ path: '/create-org', element: <CreateOrgPage /> }],
          },
          {
            element: <AppShell />,
            children: [
              { path: '/dash', element: <DashboardPage /> },
              { path: '/sessions', element: <SessionsPage /> },
              { path: '/sessions/:id', element: <SessionDetailPage /> },
              { path: '/keys', element: <APIKeysPage /> },
              { path: '/system-prompts', element: <SystemPromptsPage /> },
              { path: '/system-prompts/:id', element: <SystemPromptDetailPage /> },
              { path: '/failure-clusters', element: <FailureClustersPage /> },
              { path: '/settings', element: <SettingsPage /> },
            ],
          },
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
