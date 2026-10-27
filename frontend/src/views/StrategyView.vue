<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api, type BacktestRun, type Strategy, type StrategyConfig } from '../api'

const router = useRouter()
const strategies = ref<Strategy[]>([])
const loading = ref(true)
const submitting = ref(false)
const error = ref('')
const resultMessage = ref('')
const form = reactive({
  name: 'momentum_research',
  version: '1',
  initialCapital: '5000',
  maxPositions: 10,
  rankBuffer: 15,
  maxHoldingMonths: 3,
  minTradeValue: '150',
  commissionBps: '5',
  minCommission: '1',
  fxSpreadBps: '25',
  stampDutyBps: '30',
  slippageBps: '10',
  withholdingRate: '0.15',
  fractionalShares: false,
  startDate: '2023-12-01',
  endDate: '2024-02-29',
  role: 'development',
  confirmHoldout: false,
})

async function loadStrategies() {
  loading.value = true
  try {
    const page = await api<{ items: Strategy[] }>('/strategies')
    strategies.value = page.items
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : 'Strategies could not be loaded.'
  } finally {
    loading.value = false
  }
}

async function createAndRun() {
  error.value = ''
  resultMessage.value = ''
  const capital = Number(form.initialCapital)
  const minimumTrade = Number(form.minTradeValue)
  if (!Number.isFinite(capital) || capital <= 0 || capital > 5000) {
    error.value = 'Initial capital must be greater than zero and no more than CHF 5,000.'
    return
  }
  if (!Number.isFinite(minimumTrade) || minimumTrade < 0) {
    error.value = 'Minimum trade value must be a non-negative amount.'
    return
  }
  const costs = [
    form.commissionBps, form.minCommission, form.fxSpreadBps,
    form.stampDutyBps, form.slippageBps,
  ].map(Number)
  if (costs.some(value => !Number.isFinite(value) || value < 0)) {
    error.value = 'Transaction costs must be finite and non-negative.'
    return
  }
  const withholding = Number(form.withholdingRate)
  if (!Number.isFinite(withholding) || withholding < 0 || withholding > 1) {
    error.value = 'Withholding rate must be between zero and one.'
    return
  }
  if (form.rankBuffer < form.maxPositions) {
    error.value = 'Rank buffer must be at least the maximum number of positions.'
    return
  }
  if (form.startDate > form.endDate) {
    error.value = 'Start date must not be after end date.'
    return
  }
  if (form.role === 'holdout' && !form.confirmHoldout) {
    error.value = 'Confirm the holdout access before starting this run.'
    return
  }

  submitting.value = true
  try {
    const config: StrategyConfig = {
      initial_capital_chf: form.initialCapital,
      max_positions: form.maxPositions,
      rank_buffer: form.rankBuffer,
      max_holding_months: form.maxHoldingMonths,
      min_trade_value_chf: form.minTradeValue,
      fractional_shares: form.fractionalShares,
      costs: {
        commission_bps: form.commissionBps,
        min_commission_chf: form.minCommission,
        fx_spread_bps: form.fxSpreadBps,
        stamp_duty_bps: form.stampDutyBps,
        slippage_bps: form.slippageBps,
        withholding_rate: form.withholdingRate,
      },
    }
    const strategy = await api<Strategy>('/strategies', {
      method: 'POST',
      body: JSON.stringify({ name: form.name, version: form.version, parameters: config }),
    })
    strategies.value = [strategy, ...strategies.value]
    const run = await api<BacktestRun>('/backtests', {
      method: 'POST',
      body: JSON.stringify({
        strategy_id: strategy.id,
        start_date: form.startDate,
        end_date: form.endDate,
        dataset_role: form.role,
        confirm_holdout: form.confirmHoldout,
      }),
    })
    resultMessage.value = 'Strategy saved and backtest queued.'
    await router.push(`/backtests/${run.id}`)
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : 'Could not save the strategy or start the backtest.'
  } finally {
    submitting.value = false
  }
}

onMounted(loadStrategies)
</script>

<template>
  <section class="page-grid">
    <header class="page-heading">
      <p class="eyebrow">
        Research setup
      </p>
      <h1>Strategy configuration</h1>
      <p>Save a reproducible configuration and launch an asynchronous historical research run.</p>
    </header>
    <section class="card">
      <h2>New strategy and backtest</h2>
      <form
        class="form-grid strategy-form"
        @submit.prevent="createAndRun"
      >
        <fieldset class="form-section form-wide">
          <legend>Portfolio rules</legend>
          <div class="strategy-fields strategy-fields-portfolio">
            <label>Strategy name<input
              v-model.trim="form.name"
              required
              maxlength="100"
            ></label>
            <label>Version<input
              v-model.trim="form.version"
              required
              maxlength="50"
            ></label>
            <label>Initial capital (CHF)<input
              v-model="form.initialCapital"
              inputmode="decimal"
              required
            ></label>
            <label>Maximum positions<input
              v-model.number="form.maxPositions"
              type="number"
              min="1"
              max="100"
              required
            ></label>
            <label>Rank buffer<input
              v-model.number="form.rankBuffer"
              type="number"
              min="1"
              max="100"
              required
            ></label>
            <label>Maximum holding months<input
              v-model.number="form.maxHoldingMonths"
              type="number"
              min="1"
              max="60"
              required
            ></label>
            <label>Minimum trade value (CHF)<input
              v-model="form.minTradeValue"
              inputmode="decimal"
              required
            ></label>
            <label class="check-filter"><input
              v-model="form.fractionalShares"
              type="checkbox"
            > Allow fractional shares</label>
          </div>
        </fieldset>
        <fieldset class="form-section form-wide">
          <legend>Trading costs</legend>
          <div class="strategy-fields strategy-fields-costs">
            <label>Commission (basis points)<input
              v-model="form.commissionBps"
              inputmode="decimal"
              required
            ></label>
            <label>Minimum commission (CHF)<input
              v-model="form.minCommission"
              inputmode="decimal"
              required
            ></label>
            <label>FX spread (basis points)<input
              v-model="form.fxSpreadBps"
              inputmode="decimal"
              required
            ></label>
            <label>Stamp duty (basis points)<input
              v-model="form.stampDutyBps"
              inputmode="decimal"
              required
            ></label>
            <label>Slippage (basis points)<input
              v-model="form.slippageBps"
              inputmode="decimal"
              required
            ></label>
            <label>Dividend withholding rate<input
              v-model="form.withholdingRate"
              inputmode="decimal"
              required
            ></label>
          </div>
        </fieldset>
        <fieldset class="form-section form-wide">
          <legend>Research period</legend>
          <div class="strategy-fields strategy-fields-period">
            <label>Dataset role
              <select v-model="form.role">
                <option value="development">Development</option>
                <option value="walk_forward">Walk-forward</option>
                <option value="holdout">Holdout</option>
                <option value="forward">Forward</option>
              </select>
            </label>
            <label>Start date<input
              v-model="form.startDate"
              type="date"
              required
            ></label>
            <label>End date<input
              v-model="form.endDate"
              type="date"
              required
            ></label>
            <label
              v-if="form.role === 'holdout'"
              class="check-filter"
            ><input
              v-model="form.confirmHoldout"
              type="checkbox"
            > I confirm this holdout access</label>
          </div>
        </fieldset>
        <p class="form-help">
          Costs are per-side research assumptions in basis points, except minimum commission and withholding rate. The strategy is saved before the run is queued. Existing versions cannot be overwritten. Capital above CHF 5,000 is rejected.
        </p>
        <p
          v-if="error"
          role="alert"
          class="message error form-wide"
        >
          {{ error }}
        </p>
        <p
          v-if="resultMessage"
          role="status"
          class="message form-wide"
        >
          {{ resultMessage }}
        </p>
        <button
          class="form-wide primary-button"
          type="submit"
          :disabled="submitting"
        >
          {{ submitting ? 'Starting…' : 'Save and run backtest' }}
        </button>
      </form>
    </section>
    <section class="card">
      <h2>Saved strategies</h2>
      <p
        v-if="loading"
        role="status"
        class="message"
      >
        Loading strategies…
      </p>
      <ul
        v-else-if="strategies.length"
        class="strategy-list"
      >
        <li
          v-for="item in strategies"
          :key="item.id"
        >
          <strong>{{ item.name }} v{{ item.version }}</strong>
          <span>CHF {{ item.parameters.initial_capital_chf }} · {{ item.parameters.max_positions }} positions</span>
        </li>
      </ul>
      <p
        v-else
        class="message"
      >
        No strategy definitions have been saved.
      </p>
    </section>
  </section>
</template>
