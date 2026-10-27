import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { afterEach, describe, expect, it, vi } from 'vitest'
import BacktestView from './BacktestView.vue'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('BacktestView', () => {
  it('shows a useful failure state and links to next steps', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        id: 'run-1',
        strategy_id: 'strategy-1',
        start_date: '2023-12-01',
        end_date: '2024-02-29',
        status: 'failed',
        dataset_role: 'development',
        survivorship_biased: true,
        error: 'backtest failed; consult server logs for details',
        warnings: [],
      }),
    }))
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: '/backtests/:id', component: BacktestView }],
    })
    await router.push('/backtests/run-1')
    await router.isReady()
    const wrapper = mount(BacktestView, { global: { plugins: [router] } })
    await flushPromises()

    expect(wrapper.get('.status-badge').text()).toBe('failed')
    expect(wrapper.get('.backtest-status-card h2').text()).toBe('Backtest could not be completed')
    expect(wrapper.get('.backtest-status-card').text()).toContain('Check the API logs')
    expect(wrapper.get('a.primary-link').attributes('href')).toBe('/strategies')
    expect(wrapper.get('a.secondary-link').attributes('href')).toBe('/stocks')
    expect(wrapper.get('.warning').text()).toContain('Survivorship-biased')
  })
})
