import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { EmptyState } from './EmptyState'

// Wrap in MemoryRouter because EmptyState uses Link for href actions
function renderWithRouter(ui: React.ReactElement) {
  return render(<MemoryRouter>{ui}</MemoryRouter>)
}

describe('EmptyState', () => {
  it('renders heading and body', () => {
    renderWithRouter(<EmptyState heading="No data" body="Nothing here yet." />)
    expect(screen.getByText('No data')).toBeTruthy()
    expect(screen.getByText('Nothing here yet.')).toBeTruthy()
  })

  it('renders action button when provided with onClick', () => {
    renderWithRouter(
      <EmptyState heading="Empty" body="Test" action={{ label: 'Create item', onClick: vi.fn() }} />
    )
    expect(screen.getByText('Create item')).toBeTruthy()
  })

  it('renders action link when provided with href', () => {
    renderWithRouter(
      <EmptyState heading="Empty" body="Test" action={{ label: 'Go somewhere', href: '/somewhere' }} />
    )
    expect(screen.getByText('Go somewhere')).toBeTruthy()
  })

  it('does not render action when not provided', () => {
    const { container } = renderWithRouter(<EmptyState heading="Empty" body="Test" />)
    expect(container.querySelectorAll('button').length).toBe(0)
    expect(container.querySelectorAll('a').length).toBe(0)
  })
})
