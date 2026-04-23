import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { UsageBar } from './UsageBar'

// Mock the hooks
vi.mock('@/hooks/use-usage', () => ({
  useUsage: vi.fn(),
}))
vi.mock('@/hooks/use-org', () => ({
  useOrg: vi.fn(),
}))
vi.mock('@/store', () => ({
  useAuthStore: vi.fn((selector: (s: { activeOrgID: string | null }) => unknown) =>
    selector({ activeOrgID: 'org-123' })
  ),
}))

import { useUsage } from '@/hooks/use-usage'
import { useOrg } from '@/hooks/use-org'

const mockedUseUsage = vi.mocked(useUsage)
const mockedUseOrg = vi.mocked(useOrg)

function renderUsageBar(collapsed = false) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <UsageBar collapsed={collapsed} />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe('UsageBar', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    sessionStorage.clear()
  })

  it('renders nothing for pro plan', () => {
    mockedUseOrg.mockReturnValue({ data: { plan: 'pro' } } as any)
    mockedUseUsage.mockReturnValue({ data: { spans_used: 100, spans_limit: 0, plan: 'pro' } } as any)

    const { container } = renderUsageBar()
    expect(container.innerHTML).toBe('')
  })

  it('renders nothing when no data', () => {
    mockedUseOrg.mockReturnValue({ data: undefined } as any)
    mockedUseUsage.mockReturnValue({ data: undefined } as any)

    const { container } = renderUsageBar()
    expect(container.innerHTML).toBe('')
  })

  it('renders usage bar for free plan', () => {
    mockedUseOrg.mockReturnValue({ data: { plan: 'free' } } as any)
    mockedUseUsage.mockReturnValue({
      data: { spans_used: 847, spans_limit: 3000, plan: 'free', period_start: '', period_end: '' },
    } as any)

    const { container } = renderUsageBar()
    const text = container.textContent ?? ''
    expect(text).toContain('847')
    expect(text).toContain('3,000')
    expect(text).toContain('Free plan')
  })

  it('renders upgrade link for free plan', () => {
    mockedUseOrg.mockReturnValue({ data: { plan: 'free' } } as any)
    mockedUseUsage.mockReturnValue({
      data: { spans_used: 100, spans_limit: 3000, plan: 'free', period_start: '', period_end: '' },
    } as any)

    renderUsageBar()
    const link = screen.getByText(/Upgrade to Pro/)
    expect(link).toBeTruthy()
    expect(link.closest('a')?.getAttribute('href')).toBe('/settings')
  })

  it('renders collapsed state without text', () => {
    mockedUseOrg.mockReturnValue({ data: { plan: 'free' } } as any)
    mockedUseUsage.mockReturnValue({
      data: { spans_used: 500, spans_limit: 3000, plan: 'free', period_start: '', period_end: '' },
    } as any)

    const { container } = renderUsageBar(true)
    // Collapsed state should not show "Free plan" or "Upgrade" text
    expect(screen.queryByText(/Free plan/)).toBeNull()
    expect(screen.queryByText(/Upgrade/)).toBeNull()
    // But should still render the bar container
    expect(container.querySelector('[class*="rounded-full"]')).toBeTruthy()
  })
})
