<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api, type Alert, type Page } from '../api'

const alerts = ref<Alert[]>([])
const loading = ref(true)
const error = ref('')
const pending = ref<string | null>(null)

async function load() {
  loading.value = true
  error.value = ''
  try {
    const page = await api<Page<Alert>>('/alerts?limit=100')
    alerts.value = page.items
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : 'Alerts could not be loaded.'
  } finally {
    loading.value = false
  }
}

async function acknowledge(id: string) {
  pending.value = id
  error.value = ''
  try {
    const updated = await api<Alert>(`/alerts/${encodeURIComponent(id)}`, { method: 'PATCH' })
    alerts.value = alerts.value.map(item => item.id === id ? updated : item)
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : 'Alert could not be acknowledged.'
  } finally {
    pending.value = null
  }
}

onMounted(load)
</script>

<template>
  <section class="page-grid">
    <header class="page-heading">
      <p class="eyebrow">
        Research signals
      </p>
      <h1>Alerts</h1>
      <p>Alerts report configured score thresholds only. They are not trade instructions.</p>
    </header>
    <section class="card">
      <p
        v-if="loading"
        role="status"
        class="message"
      >
        Loading alerts…
      </p>
      <p
        v-else-if="error"
        role="alert"
        class="message error"
      >
        {{ error }}
      </p>
      <p
        v-else-if="alerts.length === 0"
        class="message"
      >
        No alerts are available.
      </p>
      <ul
        v-else
        class="alert-list"
      >
        <li
          v-for="alert in alerts"
          :key="alert.id"
        >
          <div>
            <strong>{{ alert.symbol }} · {{ alert.alert_type.replaceAll('_', ' ') }}</strong>
            <p>{{ alert.message }}</p>
            <small>{{ new Date(alert.created_at).toLocaleString() }}</small>
          </div>
          <button
            :disabled="alert.acknowledged || pending === alert.id"
            @click="acknowledge(alert.id)"
          >
            {{ alert.acknowledged ? 'Acknowledged' : pending === alert.id ? 'Saving…' : 'Acknowledge' }}
          </button>
        </li>
      </ul>
    </section>
  </section>
</template>
