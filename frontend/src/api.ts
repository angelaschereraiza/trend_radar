export interface Page<T> {
  items: T[]
  total: number
  limit: number
  offset: number
}

export interface Ranking {
  stock_id: string
  symbol: string
  sector: string
  score: number
  rank: number
  affordable: boolean
  components: Record<string, number>
}

export interface Stock {
  id: string
  symbol: string
  exchange: string
  name: string
  currency: string
  sector: string
  industry: string
  active: boolean
}

export interface PriceBar {
  date: string
  open: string
  high: string
  low: string
  close: string
  adjusted_close: string
  volume: number
  currency: string
}

export interface StrategyConfig {
  initial_capital_chf: string
  max_positions: number
  rank_buffer: number
  max_holding_months: number
  min_trade_value_chf: string
  fractional_shares: boolean
  costs: BacktestCostConfig
}

export interface BacktestCostConfig {
  commission_bps: string
  min_commission_chf: string
  fx_spread_bps: string
  stamp_duty_bps: string
  slippage_bps: string
  withholding_rate: string
}

export interface Strategy {
  id: string
  name: string
  version: string
  parameters: StrategyConfig
}

export interface BacktestRun {
  id: string
  strategy_id: string
  start_date: string
  end_date: string
  status: 'queued' | 'running' | 'completed' | 'failed'
  dataset_role: string
  snapshot_id?: string
  survivorship_biased: boolean
  error?: string
  warnings: string[]
  summary?: Record<string, unknown>
}

export interface EquityPoint {
  date: string
  portfolio_chf: string
  cash_chf: string
  invested_chf: string
  benchmark_chf: string
}

export interface Trade {
  date: string
  symbol: string
  side: string
  quantity: string
  reason: string
  status: string
  reject_reason?: string
  price_usd?: string
  gross_chf?: string
  commission_chf?: string
  fx_cost_chf?: string
  stamp_duty_chf?: string
  slippage_chf?: string
}

export interface Alert {
  id: string
  stock_id: string
  symbol: string
  strategy_id: string
  alert_type: string
  message: string
  created_at: string
  acknowledged: boolean
}

export interface JobRun {
  id: string
  job_name: string
  scheduled_for: string
  status: string
  error_message?: string
}

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`/api/v1${path}`, {
    ...init,
    headers: {
      ...(init?.body ? { 'Content-Type': 'application/json' } : {}),
      ...init?.headers,
    },
  })
  if (!response.ok) {
    let message = `Request failed (HTTP ${response.status}).`
    try {
      const body = await response.json() as { error?: { message?: string } }
      message = body.error?.message ?? message
    } catch {
      // Preserve the HTTP status when the server does not return JSON.
    }
    throw new Error(message)
  }
  return await response.json() as T
}

export function percent(value: unknown): string {
  return typeof value === 'number' && Number.isFinite(value)
    ? `${(value * 100).toFixed(2)}%`
    : '—'
}
