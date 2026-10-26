<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { api, type Page, type Ranking } from '../api'

const page = ref<Page<Ranking>>({ items: [], total: 0, limit: 50, offset: 0 })
const search = ref('')
const sector = ref('')
const affordableOnly = ref(false)
const sortBy = ref<'score' | 'momentum' | 'volatility'>('score')
const offset = ref(0)
const loading = ref(true)
const error = ref('')

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

onMounted(load)
</script>

<template>
  <section class="page-grid">
    <header class="page-heading">
      <p class="eyebrow">
        Research workspace
      </p>
      <h1>Stock Explorer</h1>
      <p>Explore ranked stocks and filter by affordability. Scores are historical research measures, not recommendations.</p>
    </header>
    <section class="card">
      <div class="filters">
        <label>Search symbol or company
          <input
            v-model="search"
            type="search"
            placeholder="e.g. Microsoft"
            @keyup.enter="offset = 0; load()"
          >
        </label>
        <label>Sector
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
        <label>Sort by
          <select
            v-model="sortBy"
            @change="offset = 0; load()"
          >
            <option value="score">Score</option>
            <option value="momentum">Momentum</option>
            <option value="volatility">Volatility-adjusted momentum</option>
          </select>
        </label>
        <label class="check-filter"><input
          v-model="affordableOnly"
          type="checkbox"
          @change="offset = 0; load()"
        > Affordable only</label>
        <button
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
      <p
        v-else-if="visible.length === 0"
        class="message"
      >
        No stocks match these filters.
      </p>
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
  </section>
</template>
