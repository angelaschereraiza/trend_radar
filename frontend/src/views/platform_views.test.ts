import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { afterEach, describe, expect, it, vi } from 'vitest'
import StrategyView from './StrategyView.vue'
import AlertsView from './AlertsView.vue'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('platform workflows', () => {
  it('rejects capital above CHF 5,000 before submitting a strategy', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ items: [], total: 0 }),
    })
    vi.stubGlobal('fetch', fetchMock)
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: '/strategies', component: StrategyView }],
    })
    await router.push('/strategies')
    await router.isReady()
    const wrapper = mount(StrategyView, { global: { plugins: [router] } })
    await flushPromises()

    const capital = wrapper.findAll('label').find(label => label.text().includes('Initial capital'))
    if (!capital) throw new Error('capital field was not rendered')
    await capital.get('input').setValue('5000.01')
    await wrapper.get('form').trigger('submit')

    expect(wrapper.get('[role="alert"]').text()).toContain('CHF 5,000')
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('acknowledges an alert and updates its status', async () => {
    const alert = {
      id: 'alert-1',
      stock_id: 'stock-1',
      symbol: 'TEST',
      strategy_id: 'strategy-1',
      alert_type: 'score_threshold',
      message: 'Score threshold crossed.',
      created_at: '2024-01-02T12:00:00Z',
      acknowledged: false,
    }
    vi.stubGlobal('fetch', vi.fn()
      .mockResolvedValueOnce({ ok: true, json: async () => ({ items: [alert], total: 1, limit: 100, offset: 0 }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ ...alert, acknowledged: true }) }))
    const wrapper = mount(AlertsView)
    await flushPromises()
    await wrapper.get('button').trigger('click')
    await flushPromises()

    expect(wrapper.get('button').text()).toBe('Acknowledged')
  })
})
