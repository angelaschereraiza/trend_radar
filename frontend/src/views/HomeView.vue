<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { api, type JobRun } from '../api'

interface RankingEntry {
  stock_id: string
  symbol: string
  sector: string
  score: number
  rank: number
  affordable: boolean
}

interface RankingPage {
  items: RankingEntry[]
  total: number
}

const rankings = ref<RankingEntry[]>([])
const total = ref(0)
const loading = ref(true)
const error = ref('')
const jobs = ref<JobRun[]>([])
const jobsError = ref('')

onMounted(async () => {
  try {
    const response = await fetch('/api/v1/rankings?limit=5')
    if (!response.ok) {
      throw new Error(`Rankings could not be loaded (HTTP ${response.status}).`)
    }
    const page = await response.json() as RankingPage
    rankings.value = page.items
    total.value = page.total
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : 'Rankings could not be loaded.'
  } finally {
    loading.value = false
  }
  try {
    const jobPage = await api<{ items: JobRun[] }>('/jobs?limit=5')
    jobs.value = jobPage.items
  } catch (cause) {
    jobsError.value = cause instanceof Error ? cause.message : 'Job status could not be loaded.'
  }
})
</script>

<template>
  <section class="dashboard">
    <div class="welcome">
      <p class="eyebrow">
        Research workspace
      </p>
      <h1>Understand the trends behind the market.</h1>
      <p>
        Explore transparent stock rankings and reproducible strategy research.
        Results are research data, not investment advice.
      </p>
    </div>

    <section
      class="ranking-card"
      aria-labelledby="rankings-heading"
    >
      <div class="section-heading">
        <div>
          <p class="eyebrow">
            Momentum strategy
          </p>
          <h2 id="rankings-heading">
            Latest rankings
          </h2>
        </div>
        <span
          v-if="!loading && !error"
          class="result-count"
        >{{ total }} stocks</span>
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
        v-else-if="rankings.length === 0"
        class="message"
      >
        No rankings are available yet. Run the ranking worker after importing market data.
      </p>
      <div
        v-else
        class="table-scroll"
      >
        <table>
          <thead>
            <tr>
              <th scope="col">
                Rank
              </th>
              <th scope="col">
                Symbol
              </th>
              <th scope="col">
                Sector
              </th>
              <th scope="col">
                Score
              </th>
              <th scope="col">
                Affordable
              </th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="entry in rankings"
              :key="entry.stock_id"
            >
              <td>{{ entry.rank }}</td>
              <th scope="row">
                {{ entry.symbol }}
              </th>
              <td>{{ entry.sector || '—' }}</td>
              <td>{{ entry.score.toFixed(1) }}</td>
              <td>
                <span :class="entry.affordable ? 'yes' : 'no'">
                  {{ entry.affordable ? 'Yes' : 'No' }}
                </span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <p class="disclaimer">
        Affordability reflects the current single-position budget and does not indicate suitability.
        Total managed capital is capped at CHF 5,000.
      </p>
    </section>
    <section class="card">
      <div class="section-heading">
        <div>
          <p class="eyebrow">
            System activity
          </p><h2>Recent jobs</h2>
        </div>
        <RouterLink to="/alerts">View alerts</RouterLink>
      </div>
      <p
        v-if="jobsError"
        role="alert"
        class="message error"
      >
        {{ jobsError }}
      </p>
      <p
        v-if="jobs.length === 0"
        class="message"
      >
        No scheduled job activity is recorded yet. Start the worker with <code>--schedule</code> to process the queue.
      </p>
      <ul
        v-else
        class="job-list"
      >
        <li
          v-for="job in jobs"
          :key="job.id"
        >
          <strong>{{ job.job_name.replaceAll('_', ' ') }}</strong>
          <span>{{ job.status }}</span>
          <time :datetime="job.scheduled_for">{{ new Date(job.scheduled_for).toLocaleString() }}</time>
          <small
            v-if="job.error_message"
            class="error"
          >{{ job.error_message }}</small>
        </li>
      </ul>
    </section>
    <nav
      class="quick-links"
      aria-label="Research workflows"
    >
      <RouterLink to="/stocks">Explore stocks</RouterLink>
      <RouterLink to="/strategies">Configure a strategy and backtest</RouterLink>
    </nav>
  </section>
</template>
