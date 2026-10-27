<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { api, type Page, type Ranking, type Stock } from '../api'

const page = ref<Page<Ranking>>({ items: [], total: 0, limit: 50, offset: 0 })
const stocks = ref<Stock[]>([])
const search = ref('')
const sector = ref('')
const affordableOnly = ref(false)
const sortBy = ref<'score' | 'momentum' | 'volatility'>('score')
const offset = ref(0)
const loading = ref(true)
const error = ref('')
const catalogError = ref('')
const hasFilters = computed(() => Boolean(search.value.trim() || sector.value || affordableOnly.value))

const sectors = computed(() => [...new Set(page.value.items.map(item => item.sector).filter(Boolean))].sort())
const visible = computed(() => page.value.items
  .filter(item => item.symbol.toLowerCase().includes(search.value.toLowerCase()))
  .filter(item => !sector.value || item.sector === sector.value)
  .filter(item => !affordableOnly.value || item.affordable)
  .slice().sort((a, b) => {
    if (sortBy.value === 'score') return b.score - a.score || a.symbol.localeCompare(b.symbol)
    const key = sortBy.value === 'momentum' ? 'momentum' : 'volatility_adjusted_momentum'
    return (b.components[key] ?? 0) - (a.components[key] ?? 0) || a.symbol.localeCompare(b.symbol)
  }))
const catalogMatches = computed(() => stocks.value
  .filter(item => item.symbol.toLowerCase().includes(search.value.trim().toLowerCase())
    || item.name.toLowerCase().includes(search.value.trim().toLowerCase()))
  .filter(item => !sector.value || item.sector === sector.value))

async function load() {
  loading.value = true
  error.value = ''
  try {
    const query = new URLSearchParams({ limit: '50', offset: String(offset.value), sort: sortBy.value })
    if (search.value.trim()) query.set('search', search.value.trim())
    if (sector.value) query.set('sector', sector.value)
    if (affordableOnly.value) query.set('affordable_only', 'true')
    page.value = await api<Page<Ranking>>(`/rankings?${query}`)
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : 'Rankings could not be loaded.'
  } finally {
    loading.value = false
  }
}

async function loadCatalog() {
  try {
    const stockPage = await api<Page<Stock>>('/stocks?limit=200')
    stocks.value = stockPage.items
  } catch (cause) {
    catalogError.value = cause instanceof Error ? cause.message : 'Instrument catalog could not be loaded.'
  }
}

function clearFilters() {
  search.value = ''
  sector.value = ''
  affordableOnly.value = false
  offset.value = 0
  load()
}

onMounted(() => Promise.all([load(), loadCatalog()]))
</script>

<template>
  <section class="page-grid stock-explorer">
    <header class="page-heading">
      <p class="eyebrow">
        Research workspace
      </p>
      <h1>Stock Explorer</h1>
      <p>Explore ranked stocks and filter by affordability. Scores are historical research measures, not recommendations.</p>
    </header>
    <section class="card stock-explorer-card">
      <div class="filters stock-filters">
        <label class="stock-search">Search symbol or company
          <input
            v-model="search"
            type="search"
            placeholder="e.g. Aiza"
            @keyup.enter="offset = 0; load()"
          >
        </label>
        <label class="stock-sector">Sector
          <select
            v-model="sector"
            @change="offset = 0; load()"
          >
            <option value="">All sectors</option>
            <option
              v-for="item in sectors"
              :key="item"
              :value="item"
            >{{ item }}</option>
          </select>
        </label>
        <label class="stock-sort">Sort by
          <select
            v-model="sortBy"
            @change="offset = 0; load()"
          >
            <option value="score">Score</option>
            <option value="momentum">Momentum</option>
            <option value="volatility">Volatility-adjusted momentum</option>
          </select>
        </label>
        <label class="check-filter stock-affordable">
          <input
            v-model="affordableOnly"
            type="checkbox"
            @change="offset = 0; load()"
          >
          <span>Affordable only</span>
        </label>
        <button
          class="stock-search-button"
          type="button"
          @click="offset = 0; load()"
        >
          Search
        </button>
      </div>
      <p
        v-if="loading"
        role="status"
        class="message"
      >
        Loading rankings…
      </p>
      <p
        v-else-if="error"
        role="alert"
        class="message error"
      >
        {{ error }}
      </p>
      <section
        v-else-if="visible.length === 0"
        class="empty-state"
        aria-live="polite"
      >
        <h2>{{ page.total === 0 && !hasFilters ? 'No rankings yet' : 'No stocks match these filters' }}</h2>
        <p v-if="page.total === 0 && !hasFilters">
          Import market data and run the rankings worker to populate the explorer.
        </p>
        <p v-else>
          Try adjusting your search or filters to see more results.
        </p>
        <button
          v-if="hasFilters"
          type="button"
          class="secondary-button"
          @click="clearFilters"
        >
          Clear filters
        </button>
      </section>
      <div
        v-else
        class="table-scroll"
      >
        <table>
          <thead><tr><th>Rank</th><th>Symbol</th><th>Sector</th><th>Score</th><th>Momentum</th><th>Affordable</th></tr></thead>
          <tbody>
            <tr
              v-for="item in visible"
              :key="item.stock_id"
            >
              <td>{{ item.rank }}</td>
              <th scope="row">
                <RouterLink :to="`/stocks/${item.stock_id}`">{{ item.symbol }}</RouterLink>
              </th>
              <td>{{ item.sector || '—' }}</td>
              <td>{{ item.score.toFixed(1) }}</td>
              <td>{{ item.components.momentum?.toFixed(1) ?? '—' }}</td>
              <td :class="item.affordable ? 'yes' : 'no'">
                {{ item.affordable ? 'Yes' : 'No' }}
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <div class="pagination">
        <button
          :disabled="loading || offset === 0"
          @click="offset = Math.max(0, offset - page.limit); load()"
        >
          Previous
        </button>
        <span>{{ page.total }} ranked stocks</span>
        <button
          :disabled="loading || offset + page.limit >= page.total"
          @click="offset += page.limit; load()"
        >
          Next
        </button>
      </div>
    </section>
    <section
      v-if="catalogMatches.length > 0"
      class="card"
    >
      <div class="section-heading">
        <div>
          <p class="eyebrow">
            Market-data catalog
          </p>
          <h2>Available instruments</h2>
        </div>
        <span class="result-count">{{ catalogMatches.length }} instruments</span>
      </div>
      <div class="table-scroll">
        <table>
          <thead><tr><th>Symbol</th><th>Company / instrument</th><th>Exchange</th><th>Currency</th><th>Price history</th></tr></thead>
          <tbody>
            <tr
              v-for="item in catalogMatches"
              :key="item.id"
            >
              <th scope="row">
                <RouterLink :to="`/stocks/${item.id}`">{{ item.symbol }}</RouterLink>
              </th>
              <td>{{ item.name }}</td>
              <td>{{ item.exchange }}</td>
              <td>{{ item.currency }}</td>
              <td><RouterLink :to="`/stocks/${item.id}`">View instrument</RouterLink></td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>
    <p
      v-else-if="catalogError"
      role="alert"
      class="message error"
    >
      Instrument catalog could not be loaded: {{ catalogError }}
    </p>
  </section>
</template>
