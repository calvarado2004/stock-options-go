import { render, screen } from '@testing-library/react'
import App, { InteractiveChart } from './App'

describe('App', () => {
  it('renders dashboard header', () => {
    render(<App />)
    expect(screen.getByRole('heading', { name: /stock forecast terminal/i })).toBeInTheDocument()
  })
})

describe('InteractiveChart', () => {
  it('supports empty to populated rerender without hook-order crash', () => {
    const { rerender } = render(
      <InteractiveChart
        title="Test Chart"
        subtitle="empty first"
        series={[{ id: 's1', name: 'Series 1', points: [], color: '#2f8fff' }]}
      />
    )

    expect(screen.getByText(/no chart data available/i)).toBeInTheDocument()

    rerender(
      <InteractiveChart
        title="Test Chart"
        subtitle="with data"
        series={[{ id: 's1', name: 'Series 1', points: [{ x: 1, y: 1 }, { x: 2, y: 2 }], color: '#2f8fff' }]}
      />
    )

    expect(screen.getByRole('button', { name: /series 1/i })).toBeInTheDocument()
  })
})
