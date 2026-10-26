import { flushPromises, mount } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import { afterEach, describe, expect, it, vi } from 'vitest'
import App from '../App.vue'
import HomeView from './HomeView.vue'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('foundation views', () => {
  it('shows rankings and the capital disclaimer', async () => {
    vi.stubGlobal('fetch', vi.fn().mockImplementation((url: string) => Promise.resolve({
      ok: true,
      json: async () => url.includes('/rankings')
        ? ({
            total: 1,
            items: [{
              stock_id: 'stock-1',
              symbol: 'TEST',
              sector: 'Technology',
              score: 87.5,
              rank: 1,
              affordable: true,
            }],
          })
        : ({ total: 0, items: [] }),
    })))
    const wrapper = mount(HomeView, { global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } } })
    await flushPromises()

    expect(wrapper.text()).toContain('Latest rankings')
    expect(wrapper.text()).toContain('TEST')
    expect(wrapper.text()).toContain('87.5')
    expect(wrapper.text()).toContain('CHF 5,000')
  })

  it('shows an API error instead of silently hiding failed requests', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, status: 503 }))
    const wrapper = mount(HomeView, { global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } } })
    await flushPromises()

    expect(wrapper.get('[role="alert"]').text()).toContain('HTTP 503')
  })

  it('renders the home route', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ items: [], total: 0 }),
    }))
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: '/', component: HomeView }],
    })
    await router.push('/')
    await router.isReady()
    const wrapper = mount(App, { global: { plugins: [router] } })
    await flushPromises()
    expect(wrapper.text()).toContain('Understand the trends')
    expect(wrapper.text()).toContain('CHF 5,000')
  })
})
