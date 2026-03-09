import { memo, useEffect, useMemo, useRef, useState } from 'react'

const RANGE_OPTIONS = [
  { id: '1Y', years: 1 },
  { id: '3Y', years: 3 },
  { id: '5Y', years: 5 },
  { id: 'ALL', years: null }
]

function money(v) {
  return Number(v || 0).toLocaleString(undefined, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2
  })
}

function closePrice(row) {
  return row.adjusted_close ?? row.close_price
}

function toUtcMs(dateLike) {
  return new Date(dateLike).getTime()
}

function formatYearMonth(ms) {
  const d = new Date(ms)
  const y = d.getUTCFullYear()
  const m = String(d.getUTCMonth() + 1).padStart(2, '0')
  return `${y}/${m}`
}

function formatHorizonMonths(days) {
  const months = Math.max(1, Math.round(days / 21))
  return `${months}M`
}

function nearestIndex(points, targetX) {
  if (!points.length) return -1
  let lo = 0
  let hi = points.length - 1
  while (lo <= hi) {
    const mid = (lo + hi) >> 1
    const px = points[mid].px
    if (px < targetX) {
      lo = mid + 1
    } else if (px > targetX) {
      hi = mid - 1
    } else {
      return mid
    }
  }
  if (lo >= points.length) return points.length - 1
  if (lo <= 0) return 0
  return Math.abs(points[lo].px - targetX) < Math.abs(points[lo - 1].px - targetX) ? lo : lo - 1
}

function downsampleByStride(points, maxPoints) {
  if (points.length <= maxPoints) return points
  const step = Math.ceil(points.length / maxPoints)
  const sampled = []
  for (let i = 0; i < points.length; i += step) {
    sampled.push(points[i])
  }
  const last = points[points.length - 1]
  if (sampled[sampled.length - 1] !== last) sampled.push(last)
  return sampled
}

function movingAverage(rows, period) {
  if (!rows.length) return []
  let sum = 0
  const values = rows.map(closePrice)
  return values.map((v, i) => {
    sum += v
    if (i >= period) sum -= values[i - period]
    const y = i >= period - 1 ? sum / period : null
    return { x: toUtcMs(rows[i].trading_date), y }
  }).filter((p) => p.y !== null)
}

function getApiBase() {
  return import.meta.env.VITE_API_BASE_URL || '/api'
}

async function toCanvas(node) {
  const { default: html2canvas } = await import('html2canvas')
  return html2canvas(node, { backgroundColor: '#040b17', scale: 2, useCORS: true })
}

export const InteractiveChart = memo(function InteractiveChart({ title, subtitle, series, unit = '$', bars = false, xTickFormatter = (x) => String(Math.round(x)) }) {
  const [hover, setHover] = useState(null)
  const [enabled, setEnabled] = useState(() => Object.fromEntries(series.map((s) => [s.id, true])))
  const rafRef = useRef(null)
  const lastMoveRef = useRef(0)

  useEffect(() => () => {
    if (rafRef.current) cancelAnimationFrame(rafRef.current)
  }, [])

  const w = 1080
  const h = bars ? 290 : 390
  const pad = { t: 24, r: 24, b: 52, l: 64 }

  const activeSeries = series.filter((s) => enabled[s.id] && s.points.length > 0)

  const allPoints = activeSeries.flatMap((s) => s.points)
  const minX = Math.min(...allPoints.map((p) => p.x))
  const maxX = Math.max(...allPoints.map((p) => p.x))
  const minY = Math.min(...allPoints.map((p) => p.y))
  const maxY = Math.max(...allPoints.map((p) => p.y))

  const sx = (x) => pad.l + ((x - minX) / (maxX - minX || 1)) * (w - pad.l - pad.r)
  const sy = (y) => h - pad.b - ((y - minY) / (maxY - minY || 1)) * (h - pad.t - pad.b)

  const projectedSeries = activeSeries.map((s) => ({
    ...s,
    projected: s.points.map((p) => ({ ...p, px: sx(p.x), py: sy(p.y) }))
  }))

  const xTicks = Array.from({ length: 6 }, (_, i) => minX + ((maxX - minX) / 5) * i)
  const yTicks = Array.from({ length: 6 }, (_, i) => minY + ((maxY - minY) / 5) * i)
  const showDots = !bars && allPoints.length <= 320
  const enableHover = !bars || allPoints.length <= 220

  const hoverPayload = (() => {
    if (!enableHover || !hover) return null
    const rows = projectedSeries
      .map((s) => {
        const idx = nearestIndex(s.projected, hover.x)
        if (idx < 0) return null
        return { series: s, point: s.projected[idx] }
      })
      .filter(Boolean)
    if (!rows.length) return null
    return { rows, x: rows[0].point.px }
  })()

  if (!activeSeries.length) {
    return (
      <section className="panel chart-wrap">
        <div className="chart-head">
          <h3>{title}</h3>
          <p>{subtitle}</p>
        </div>
        <div className="empty">No chart data available.</div>
      </section>
    )
  }

  return (
    <section className="panel chart-wrap">
      <div className="chart-head">
        <h3>{title}</h3>
        <p>{subtitle}</p>
      </div>

      <div className="legend-row">
        {series.map((s) => (
          <button
            key={s.id}
            className={`legend-btn ${enabled[s.id] ? 'on' : 'off'}`}
            onClick={() => setEnabled((prev) => ({ ...prev, [s.id]: !prev[s.id] }))}
            type="button"
          >
            <span style={{ background: s.color }} />
            {s.name}
          </button>
        ))}
      </div>

      <div
        className="chart-box"
        onMouseMove={(e) => {
          if (!enableHover) return
          const now = performance.now()
          if (now - lastMoveRef.current < 40) return
          lastMoveRef.current = now
          const rect = e.currentTarget.getBoundingClientRect()
          const nextX = ((e.clientX - rect.left) / rect.width) * w
          if (rafRef.current) cancelAnimationFrame(rafRef.current)
          rafRef.current = requestAnimationFrame(() => {
            setHover((prev) => {
              if (prev && Math.abs(prev.x - nextX) < 1) return prev
              return { x: nextX }
            })
          })
        }}
        onMouseLeave={() => setHover(null)}
      >
        <svg viewBox={`0 0 ${w} ${h}`}>
          {yTicks.map((y, i) => (
            <g key={`gy-${i}`}>
              <line x1={pad.l} x2={w - pad.r} y1={sy(y)} y2={sy(y)} className="grid" />
              <text x={8} y={sy(y) + 4} className="axis">{unit}{money(y)}</text>
            </g>
          ))}
          {xTicks.map((x, i) => (
            <g key={`gx-${i}`}>
              <line y1={pad.t} y2={h - pad.b} x1={sx(x)} x2={sx(x)} className="grid v" />
              <text x={sx(x) - 18} y={h - 16} className="axis">{xTickFormatter(x)}</text>
            </g>
          ))}

          {bars && projectedSeries.map((s) => s.projected.map((p, idx) => (
            <rect
              key={`${s.id}-${idx}`}
              x={p.px - 1.8}
              y={p.py}
              width={3.6}
              height={h - pad.b - p.py}
              className="bar"
              style={{ fill: s.color }}
            />
          )))}

          {!bars && projectedSeries.map((s) => {
            const path = s.projected.map((p, i) => `${i === 0 ? 'M' : 'L'} ${p.px} ${p.py}`).join(' ')
            return (
              <path
                key={s.id}
                d={path}
                className={s.dashed ? 'series-dashed' : 'series'}
                style={{ stroke: s.color }}
              />
            )
          })}

          {showDots && projectedSeries.map((s) =>
            s.projected.map((p) => (
              <circle
                key={`${s.id}-${p.x}-${p.y}`}
                cx={p.px}
                cy={p.py}
                r={s.dot || 2.8}
                className="dot"
                style={{ fill: s.color }}
              />
            ))
          )}

          {hoverPayload && <line x1={hoverPayload.x} x2={hoverPayload.x} y1={pad.t} y2={h - pad.b} className="crosshair" />}
        </svg>

        {hoverPayload && (
          <div className="tooltip" style={{ left: `${(hoverPayload.x / w) * 100}%` }}>
            {hoverPayload.rows.map((r) => (
              <div key={r.series.id} className="tip-row">
                <span className="tip-dot" style={{ background: r.series.color }} />
                <span>{r.series.name}</span>
                <strong>{unit}{money(r.point.y)}</strong>
              </div>
            ))}
          </div>
        )}
      </div>
    </section>
  )
})

export default function App() {
  const [ticker, setTicker] = useState('PSTG')
  const [range, setRange] = useState('5Y')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [ingest, setIngest] = useState(null)
  const [rows, setRows] = useState([])
  const [forecast, setForecast] = useState(null)
  const [analysis, setAnalysis] = useState(null)
  const exportRef = useRef(null)

  const load = async () => {
    setLoading(true)
    setError('')
    try {
      const symbol = ticker.trim().toUpperCase()
      const api = getApiBase()

      const ingestRes = await fetch(`${api}/ingest?ticker=${encodeURIComponent(symbol)}`, { method: 'POST' })
      if (!ingestRes.ok) throw new Error(await ingestRes.text())
      setIngest(await ingestRes.json())

      const dataRes = await fetch(`${api}/data?ticker=${encodeURIComponent(symbol)}`)
      if (!dataRes.ok) throw new Error(await dataRes.text())
      const data = await dataRes.json()
      setRows(data.data || [])

      const forecastRes = await fetch(`${api}/forecast?ticker=${encodeURIComponent(symbol)}`)
      if (!forecastRes.ok) throw new Error(await forecastRes.text())
      setForecast(await forecastRes.json())

      const analysisRes = await fetch(`${api}/analysis?ticker=${encodeURIComponent(symbol)}`)
      if (!analysisRes.ok) throw new Error(await analysisRes.text())
      setAnalysis(await analysisRes.json())
    } catch (e) {
      setError(String(e.message || e))
    } finally {
      setLoading(false)
    }
  }

  const filteredRows = useMemo(() => {
    if (!rows.length) return []
    const option = RANGE_OPTIONS.find((r) => r.id === range)
    if (!option || option.years === null) return rows
    const cutoff = new Date()
    cutoff.setUTCFullYear(cutoff.getUTCFullYear() - option.years)
    return rows.filter((r) => new Date(r.trading_date) >= cutoff)
  }, [rows, range])

  const daily = useMemo(() => filteredRows.map((r) => ({ x: toUtcMs(r.trading_date), y: closePrice(r) })), [filteredRows])
  const ma20 = useMemo(() => movingAverage(filteredRows, 20), [filteredRows])
  const ma50 = useMemo(() => movingAverage(filteredRows, 50), [filteredRows])
  const volumes = useMemo(() => filteredRows.map((r) => ({ x: toUtcMs(r.trading_date), y: Number(r.volume || 0) })), [filteredRows])
  const dailyFast = useMemo(() => downsampleByStride(daily, 560), [daily])
  const ma20Fast = useMemo(() => downsampleByStride(ma20, 560), [ma20])
  const ma50Fast = useMemo(() => downsampleByStride(ma50, 560), [ma50])
  const volumeFast = useMemo(() => downsampleByStride(volumes, 220), [volumes])

  const annualAvg = useMemo(() => {
    const perYear = new Map()
    for (const r of filteredRows) {
      const y = new Date(r.trading_date).getUTCFullYear()
      const curr = perYear.get(y) || { s: 0, c: 0 }
      curr.s += closePrice(r)
      curr.c += 1
      perYear.set(y, curr)
    }
    return [...perYear.entries()].sort((a, b) => a[0] - b[0]).map(([x, v]) => ({ x, y: v.s / v.c }))
  }, [filteredRows])

  const regression = useMemo(() => {
    if (!forecast || annualAvg.length < 2) return []
    const start = annualAvg[0].x
    const end = forecast.year_after_next
    const pts = []
    for (let y = start; y <= end; y += 1) {
      pts.push({ x: y, y: forecast.regression_intercept + forecast.regression_slope * y })
    }
    return pts
  }, [forecast, annualAvg])

  const projected = useMemo(() => {
    if (!forecast) return []
    return [
      { x: forecast.current_year, y: forecast.current_year_remaining_forecast },
      { x: forecast.next_year, y: forecast.next_year_forecast },
      { x: forecast.year_after_next, y: forecast.year_after_next_forecast }
    ]
  }, [forecast])

  const mcP10 = useMemo(() => (analysis?.monte_carlo?.points || []).map((p) => ({ x: p.horizon_days, y: p.p10 })), [analysis])
  const mcP50 = useMemo(() => (analysis?.monte_carlo?.points || []).map((p) => ({ x: p.horizon_days, y: p.p50 })), [analysis])
  const mcP90 = useMemo(() => (analysis?.monte_carlo?.points || []).map((p) => ({ x: p.horizon_days, y: p.p90 })), [analysis])

  const exportPNG = async () => {
    if (!exportRef.current) return
    const canvas = await toCanvas(exportRef.current)
    const link = document.createElement('a')
    link.download = `${ticker.toUpperCase()}-forecast.png`
    link.href = canvas.toDataURL('image/png')
    link.click()
  }

  const exportPDF = async () => {
    if (!exportRef.current) return
    const { jsPDF } = await import('jspdf')
    const canvas = await toCanvas(exportRef.current)
    const img = canvas.toDataURL('image/png')
    const pdf = new jsPDF({ orientation: 'landscape', unit: 'pt', format: 'a4' })
    const width = 800
    const height = (canvas.height * width) / canvas.width
    pdf.addImage(img, 'PNG', 20, 20, width, height)
    pdf.save(`${ticker.toUpperCase()}-forecast.pdf`)
  }

  return (
    <div className="page">
      <div className="hero">
        <h1>Stock Forecast Terminal</h1>
        <p>Institutional-style analytics: historical trend, moving averages, volume, and in-house regression forecast</p>
      </div>

      <section className="panel controls">
        <input value={ticker} onChange={(e) => setTicker(e.target.value)} placeholder="Ticker symbol (e.g. PSTG, AAPL, GOOG)" />
        <button onClick={load} disabled={loading} type="button">{loading ? 'Loading...' : 'Refresh & Forecast'}</button>

        <div className="range-group">
          {RANGE_OPTIONS.map((opt) => (
            <button
              key={opt.id}
              type="button"
              className={`range-btn ${range === opt.id ? 'active' : ''}`}
              onClick={() => setRange(opt.id)}
            >
              {opt.id}
            </button>
          ))}
        </div>

        <button type="button" onClick={exportPNG}>Export PNG</button>
        <button type="button" onClick={exportPDF}>Export PDF</button>

        {ingest && <span className="badge">provider_used: {ingest.provider_used}</span>}
        {ingest && <span className="badge">fetched: {ingest.fetched_record_count}</span>}
        {ingest && <span className="badge">cache: {String(ingest.using_cached_data)}</span>}
      </section>

      {error && <section className="panel error">{error}</section>}

      <section className="metrics">
        <article className="panel metric"><span>Current Year Remaining</span><strong>{forecast ? `$${money(forecast.current_year_remaining_forecast)}` : '-'}</strong></article>
        <article className="panel metric"><span>Next Year</span><strong>{forecast ? `$${money(forecast.next_year_forecast)}` : '-'}</strong></article>
        <article className="panel metric"><span>Year After Next</span><strong>{forecast ? `$${money(forecast.year_after_next_forecast)}` : '-'}</strong></article>
        <article className="panel metric"><span>Regression Equation</span><strong>{forecast ? `y = ${forecast.regression_slope.toFixed(2)}x + ${forecast.regression_intercept.toFixed(2)}` : '-'}</strong></article>
      </section>

      <section className="metrics">
        <article className="panel metric"><span>AR(1) Expected 30D</span><strong>{analysis ? `$${money(analysis.ar1.expected_price_30d)}` : '-'}</strong></article>
        <article className="panel metric"><span>AR(1) 1D Return</span><strong>{analysis ? `${(analysis.ar1.forecast_return_1d * 100).toFixed(2)}%` : '-'}</strong></article>
        <article className="panel metric"><span>MC Annual Drift</span><strong>{analysis ? `${(analysis.monte_carlo.drift_annual * 100).toFixed(2)}%` : '-'}</strong></article>
        <article className="panel metric"><span>MC Annual Volatility</span><strong>{analysis ? `${(analysis.monte_carlo.volatility_annual * 100).toFixed(2)}%` : '-'}</strong></article>
      </section>

      <div ref={exportRef}>
        <InteractiveChart
          title="Price Over Time"
          subtitle="Daily close with SMA overlays"
          xTickFormatter={formatYearMonth}
          series={[
            { id: 'close', name: 'Daily Close', points: dailyFast, color: '#3ab0ff' },
            { id: 'ma20', name: 'SMA 20', points: ma20Fast, color: '#89c7ff', dashed: true, dot: 2 },
            { id: 'ma50', name: 'SMA 50', points: ma50Fast, color: '#2bd7ff', dashed: true, dot: 2 }
          ]}
        />

        <InteractiveChart
          title="Trading Volume"
          subtitle="Volume bars over selected range"
          xTickFormatter={formatYearMonth}
          series={[{ id: 'volume', name: 'Volume', points: volumeFast, color: '#1b69d6' }]}
          unit=""
          bars
        />

        <InteractiveChart
          title="Annual Average + Linear Regression Projection"
          subtitle="Actual annual averages, best-fit line, and projected values"
          series={[
            { id: 'actual', name: 'Actual Annual Avg', points: annualAvg, color: '#2f8fff' },
            { id: 'fit', name: 'Regression Fit', points: regression, color: '#95b7ff', dashed: true, dot: 2 },
            { id: 'proj', name: 'Projected Avg', points: projected, color: '#2bd7ff', dot: 4 }
          ]}
        />

        <InteractiveChart
          title="Monte Carlo Price Distribution"
          subtitle="P10 / P50 / P90 simulated price bands"
          xTickFormatter={formatHorizonMonths}
          series={[
            { id: 'mc-p10', name: 'P10', points: mcP10, color: '#3d6fb0', dashed: true, dot: 2 },
            { id: 'mc-p50', name: 'P50', points: mcP50, color: '#4bb3ff', dot: 3 },
            { id: 'mc-p90', name: 'P90', points: mcP90, color: '#77d6ff', dashed: true, dot: 2 }
          ]}
        />
      </div>
    </div>
  )
}
