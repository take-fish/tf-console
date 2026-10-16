import { describe, expect, it, vi, afterEach } from 'vitest'
import { defineComponent, h } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { usePipeline } from '@/composables/usePipeline'

function hostComponent() {
  return defineComponent({
    setup() {
      const { snapshot, loading, error } = usePipeline('/api/v1/pipeline', 60_000)
      return () =>
        h('div', [
          h('span', { class: 'loading' }, String(loading.value)),
          h('span', { class: 'error' }, error.value ?? ''),
          h('span', { class: 'repos' }, String(snapshot.value?.repos.length ?? -1)),
        ])
    },
  })
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('usePipeline', () => {
  it('成功拉取后填充快照', async () => {
    const payload = {
      generated_at: '2026-10-16T06:00:00Z',
      repos: [{ name: 'tf-gateway' }],
      events: [],
      rate_limit: { limit: 5000, remaining: 4999, reset_at: '' },
      token_configured: true,
      degraded: false,
      degraded_reason: '',
      poll_interval_seconds: 30,
    }
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => new Response(JSON.stringify(payload), { status: 200 })),
    )
    const w = mount(hostComponent())
    await flushPromises()
    expect(w.get('.loading').text()).toBe('false')
    expect(w.get('.repos').text()).toBe('1')
    expect(w.get('.error').text()).toBe('')
  })

  it('请求失败时记录错误且保留上次数据', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => new Response('boom', { status: 500 })),
    )
    const w = mount(hostComponent())
    await flushPromises()
    expect(w.get('.error').text()).toContain('HTTP 500')
    expect(w.get('.repos').text()).toBe('-1')
  })
})
