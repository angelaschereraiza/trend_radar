import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { afterEach, describe, expect, it, vi } from 'vitest'
import StockExplorerView from './StockExplorerView.vue'

afterEach(() => {
  vi.unstubAllGlobals()
})

function createTestRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/stocks', component: StockExplorerView },
      { path: '/stocks/:id', component: StockExplorerView },
    ],
  })
}

describe('StockExplorerView', () => {
  it('explains how to populate the explorer when no rankings exist', async () => {
    vi.stubGlobal('fetch', vi.fn((input: RequestInfo | URL) => {
      const path = String(input)
      const body = path.includes('/stocks?')
        ? {
            items: [{
              id: 'nestle-id',
              symbol: 'NESN',
              exchange: 'SIX',
              name: 'Nestlé S.A.',
              currency: 'CHF',
              sector: 'Consumer Staples',
              industry: 'Food Products',
              active: true,
            }],
            total: 1,
            limit: 200,
            offset: 0,
          }
        : { items: [], total: 0, limit: 50, offset: 0 }
      return Promise.resolve({ ok: true, json: async () => body } as Response)
    }))
    const router = createTestRouter()
    await router.push('/stocks')
    await router.isReady()
    const wrapper = mount(StockExplorerView, { global: { plugins: [router] } })
    await flushPromises()

    expect(wrapper.find('.stock-filters').exists()).toBe(true)
    expect(wrapper.get('.empty-state h2').text()).toBe('No rankings yet')
    expect(wrapper.get('.empty-state').text()).toContain('run the rankings worker')
    expect(wrapper.find('button.secondary-button').exists()).toBe(false)
    expect(wrapper.text()).toContain('NESN')
    expect(wrapper.text()).toContain('CHF')
  })

  it('offers to clear filters when there are no matching results', async () => {
    const ranking = {
      stock_id: 'stock-1',
      symbol: 'MSFT',
      sector: 'Technology',
      score: 90,
      rank: 1,
      affordable: true,
      components: { momentum: 80 },
    }
    let rankingRequests = 0
    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      if (String(input).includes('/stocks?')) {
        return Promise.resolve({ ok: true, json: async () => ({ items: [], total: 0, limit: 200, offset: 0 }) } as Response)
      }
      rankingRequests += 1
      const items = rankingRequests === 2 ? [] : [ranking]
      return Promise.resolve({
        ok: true,
        json: async () => ({ items, total: items.length, limit: 50, offset: 0 }),
      } as Response)
    })
    vi.stubGlobal('fetch', fetchMock)
    const router = createTestRouter()
    await router.push('/stocks')
    await router.isReady()
    const wrapper = mount(StockExplorerView, { global: { plugins: [router] } })
    await flushPromises()

    await wrapper.get('input[type="search"]').setValue('not-found')
    await wrapper.get('.stock-search-button').trigger('click')
    await flushPromises()
    expect(wrapper.get('.empty-state h2').text()).toBe('No stocks match these filters')

    await wrapper.get('button.secondary-button').trigger('click')
    await flushPromises()
    expect(wrapper.get('tbody').text()).toContain('MSFT')
    expect(fetchMock).toHaveBeenCalledTimes(6)
  })
})
