<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import VChart from 'vue-echarts'
import { use } from 'echarts/core'
import { LineChart } from 'echarts/charts'
import { GridComponent, LegendComponent, TooltipComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { api, type BacktestRun, type EquityPoint, type Trade } from '../api'

use([LineChart, GridComponent, LegendComponent, TooltipComponent, CanvasRenderer])
const route = useRoute()
const run = ref<BacktestRun | null>(null)
const equity = ref<EquityPoint[]>([])
const trades = ref<Trade[]>([])
const metrics = ref<Record<string, unknown> | null>(null)
const statisticalReport = ref<Record<string, unknown> | null>(null)
const error = ref('')
let poller: ReturnType<typeof setInterval> | undefined
const chartOption = computed(() => ({
  tooltip: { trigger: 'axis' },
  legend: { data: ['Portfolio', 'Benchmark'] },
  grid: { left: 65, right: 20, top: 45, bottom: 35 },
  xAxis: { type: 'category', data: equity.value.map(item => item.date), axisLabel: { hideOverlap: true } },
  yAxis: { type: 'value', scale: true, name: 'CHF' },
  series: [
    { type: 'line', name: 'Portfolio', data: equity.value.map(item => Number(item.portfolio_chf)), showSymbol: false },
    { type: 'line', name: 'Benchmark', data: equity.value.map(item => Number(item.benchmark_chf)), showSymbol: false },
  ],
}))

async function refresh() {
  try {
    const id = encodeURIComponent(String(route.params.id))
    run.value = await api<BacktestRun>(`/backtests/${id}`)
    if (run.value.status === 'completed') {
      const [curve, report] = await Promise.all([
        api<{ items: EquityPoint[] }>(`/backtests/${id}/equity`),
        api<{ metrics: Record<string, unknown>; statistical_report: Record<string, unknown>; warnings: string[] }>(`/backtests/${id}/metrics`),
      ])
      equity.value = curve.items
      metrics.value = report.metrics
      statisticalReport.value = report.statistical_report
      trades.value = (await api<{ items: Trade[] }>(`/backtests/${id}/trades`)).items
      if (poller) clearInterval(poller)
    } else if (run.value.status === 'failed' && poller) {
      clearInterval(poller)
    }
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : 'Backtest results could not be loaded.'
    if (poller) clearInterval(poller)
  }
}

onMounted(async () => {
  await refresh()
  if (run.value && ['queued', 'running'].includes(run.value.status)) {
    poller = setInterval(refresh, 2000)
  }
})
onUnmounted(() => {
  if (poller) clearInterval(poller)
})
</script>

<template>
  <section class="page-grid backtest-page">
    <p
      v-if="error"
      role="alert"
      class="message error"
    >
      {{ error }}
    </p>
    <p
      v-else-if="!run"
      role="status"
      class="message"
    >
      Loading backtest…
    </p>
    <template v-else>
      <header class="page-heading backtest-heading">
        <div>
          <p class="eyebrow">
            Backtest · {{ run.dataset_role }}
          </p>
          <h1>Research results</h1>
          <p class="backtest-period">
            {{ run.start_date }} through {{ run.end_date }}
          </p>
        </div>
        <span
          class="status-badge"
          :class="`status-${run.status}`"
        >
          {{ run.status }}
        </span>
      </header>

      <section
        v-if="run.status === 'failed'"
        class="backtest-status-card backtest-failed"
        role="status"
      >
        <span
          class="status-icon"
          aria-hidden="true"
        >!</span>
        <div class="backtest-status-content">
          <h2>Backtest could not be completed</h2>
          <p>{{ run.error || 'The run stopped before results were generated.' }}</p>
          <p class="backtest-next-step">
            Check the API logs for the failure details. Confirm that the selected dates are covered by imported prices, benchmark data, FX rates, and the chosen data snapshot.
          </p>
          <nav
            class="backtest-actions"
            aria-label="Next steps"
          >
            <RouterLink
              class="primary-link"
              to="/strategies"
            >
              Review strategy setup
            </RouterLink>
            <RouterLink
              class="secondary-link"
              to="/stocks"
            >
              Open stock explorer
            </RouterLink>
          </nav>
        </div>
      </section>
      <section
        v-else-if="run.status === 'queued' || run.status === 'running'"
        class="backtest-status-card backtest-pending"
        role="status"
      >
        <span
          class="status-icon"
          aria-hidden="true"
        >…</span>
        <div class="backtest-status-content">
          <h2>{{ run.status === 'queued' ? 'Backtest queued' : 'Backtest in progress' }}</h2>
          <p>Execution is underway. This page checks for updates automatically.</p>
        </div>
      </section>
      <div
        v-if="run.status === 'completed'"
        class="backtest-complete-summary"
      >
        <p>Research run completed for <strong>{{ run.start_date }} – {{ run.end_date }}</strong>.</p>
      </div>

      <section
        class="warning-list"
        aria-label="Research warnings"
      >
        <p
          v-if="run.survivorship_biased"
          class="warning"
        >
          Survivorship-biased data: historical delisted securities may be missing.
        </p>
        <p
          v-for="warning in run.warnings"
          :key="warning"
          class="warning"
        >
          {{ warning }}
        </p>
      </section>

      <section
        v-if="equity.length"
        class="card"
      >
        <div class="section-heading">
          <div>
            <p class="eyebrow">
              CHF equity
            </p><h2>Portfolio vs benchmark</h2>
          </div><span>{{ equity.length }} sessions</span>
        </div>
        <VChart
          class="chart"
          :option="chartOption"
          autoresize
        />
      </section>

      <section
        v-if="metrics"
        class="card"
      >
        <h2>Performance metrics</h2>
        <dl class="metric-grid">
          <template
            v-for="(value, key) in metrics"
            :key="key"
          >
            <dt>{{ String(key).replaceAll('_', ' ') }}</dt><dd>{{ typeof value === 'number' ? value.toFixed(3) : String(value) }}</dd>
          </template>
        </dl>
        <h3>Statistical report</h3>
        <pre class="report-json">{{ JSON.stringify(statisticalReport, null, 2) }}</pre>
      </section>

      <section
        v-if="trades.length"
        class="card"
      >
        <h2>Orders and fills</h2>
        <div class="table-scroll">
          <table>
            <thead><tr><th>Date</th><th>Symbol</th><th>Side</th><th>Quantity</th><th>Status</th><th>Gross CHF</th><th>Costs</th></tr></thead>
            <tbody>
              <tr
                v-for="(trade, index) in trades"
                :key="`${trade.date}-${trade.symbol}-${trade.side}-${index}`"
              >
                <td>{{ trade.date }}</td><th scope="row">
                  {{ trade.symbol }}
                </th><td>{{ trade.side }}</td><td>{{ trade.quantity }}</td><td>{{ trade.status }}</td>
                <td>{{ trade.gross_chf ?? '—' }}</td><td>{{ trade.commission_chf ?? '—' }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
      <p class="disclaimer">
        Historical research is not investment advice or a guarantee of future performance. No orders are placed.
      </p>
    </template>
  </section>
</template>
