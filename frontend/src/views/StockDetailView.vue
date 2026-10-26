<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import VChart from 'vue-echarts'
import { use } from 'echarts/core'
import { LineChart, BarChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, DataZoomComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { api, type Page, type PriceBar, type Ranking, type Stock } from '../api'

use([LineChart, BarChart, GridComponent, TooltipComponent, DataZoomComponent, CanvasRenderer])
const route = useRoute()
const stock = ref<Stock | null>(null)
const bars = ref<PriceBar[]>([])
const ranking = ref<Ranking | null>(null)
const error = ref('')
const loading = ref(true)
const dates = computed(() => bars.value.map(bar => bar.date))
const prices = computed(() => bars.value.map(bar => Number(bar.adjusted_close)))
const volumes = computed(() => bars.value.map(bar => bar.volume))
function movingAverage(period: number): Array<number | null> {
  return prices.value.map((_, index) => {
    if (index + 1 < period) return null
    const values = prices.value.slice(index + 1 - period, index + 1)
    return values.reduce((sum, value) => sum + value, 0) / period
  })
}
const sma20 = computed(() => movingAverage(20))
const sma50 = computed(() => movingAverage(50))
const sma200 = computed(() => movingAverage(200))
const priceOption = computed(() => ({
  tooltip: { trigger: 'axis' },
  grid: { left: 55, right: 20, top: 25, bottom: 35 },
  xAxis: { type: 'category', data: dates.value, axisLabel: { hideOverlap: true } },
  yAxis: { type: 'value', scale: true, name: 'USD' },
  dataZoom: [{ type: 'inside' }, { type: 'slider', height: 18 }],
  series: [
    { type: 'line', name: 'Adjusted close', data: prices.value, showSymbol: false, smooth: false },
    { type: 'line', name: 'SMA20', data: sma20.value, showSymbol: false, show: true },
    { type: 'line', name: 'SMA50', data: sma50.value, showSymbol: false },
    { type: 'line', name: 'SMA200', data: sma200.value, showSymbol: false },
  ],
}))
const volumeOption = computed(() => ({
  tooltip: { trigger: 'axis' },
  grid: { left: 55, right: 20, top: 15, bottom: 30 },
  xAxis: { type: 'category', data: dates.value, axisLabel: { show: false } },
  yAxis: { type: 'value', name: 'Volume' },
  series: [{ type: 'bar', name: 'Volume', data: volumes.value, itemStyle: { color: '#91b4ef' } }],
}))

onMounted(async () => {
  try {
    const id = encodeURIComponent(String(route.params.id))
    const [stockResult, priceResult] = await Promise.all([
      api<Stock>(`/stocks/${id}`),
      api<Page<PriceBar>>(`/stocks/${id}/prices?limit=2000`),
    ])
    stock.value = stockResult
    bars.value = priceResult.items.slice().reverse()
    const rankingPage = await api<{ items: Ranking[] }>(`/rankings?search=${encodeURIComponent(stockResult.symbol)}&limit=200`)
    ranking.value = rankingPage.items.find(item => item.stock_id === stockResult.id) ?? null
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : 'Stock details could not be loaded.'
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <section class="page-grid">
    <p
      v-if="loading"
      role="status"
      class="message"
    >
      Loading stock details…
    </p>
    <p
      v-else-if="error"
      role="alert"
      class="message error"
    >
      {{ error }}
    </p>
    <template v-else-if="stock">
      <header class="page-heading">
        <p class="eyebrow">
          {{ stock.exchange }} · {{ stock.currency }}
        </p>
        <h1>{{ stock.symbol }} — {{ stock.name }}</h1>
        <p>{{ stock.sector || 'Sector unavailable' }}<span v-if="stock.industry"> · {{ stock.industry }}</span></p>
      </header>
      <section class="card">
        <div class="section-heading">
          <div>
            <p class="eyebrow">
              Price history
            </p><h2>Adjusted close</h2>
          </div><span class="result-count">{{ bars.length }} observations</span>
        </div>
        <p
          v-if="bars.length === 0"
          class="message"
        >
          No price history is available.
        </p>
        <template v-else>
          <div class="metric-strip">
            <span>Latest adjusted close: <strong>{{ bars.at(-1)?.adjusted_close }} {{ stock.currency }}</strong></span>
            <span v-if="sma20.at(-1)">SMA20: <strong>{{ sma20.at(-1)?.toFixed(2) }}</strong></span>
            <span v-if="sma50.at(-1)">SMA50: <strong>{{ sma50.at(-1)?.toFixed(2) }}</strong></span>
            <span v-if="sma200.at(-1)">SMA200: <strong>{{ sma200.at(-1)?.toFixed(2) }}</strong></span>
            <span v-if="ranking">Latest score: <strong>{{ ranking.score.toFixed(1) }}</strong></span>
            <span v-if="ranking">Relative-strength component: <strong>{{ ranking.components.relative_strength.toFixed(1) }}</strong></span>
          </div>
          <VChart
            class="chart"
            :option="priceOption"
            autoresize
          />
          <VChart
            class="volume-chart"
            :option="volumeOption"
            autoresize
          />
          <p class="disclaimer">
            Prices are shown in {{ stock.currency }}. A CHF equivalent is not shown without a matching historical FX series.
          </p>
        </template>
      </section>
    </template>
  </section>
</template>
