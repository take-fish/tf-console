import { createPinia, setActivePinia } from 'pinia'
import { flushPromises } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { usePipelineStore } from '@/stores/pipeline'

const payload = {
  generated_at: '2026-10-19T06:00:00Z',
  repos: [{ name: 'tf-gateway' }],
  events: [],
  rate_limit: { limit: 5000, remaining: 4999, reset_at: '' },
  token_configured: true,
  degraded: false,
  degraded_reason: '',
  poll_interval_seconds: 30,
}

describe('pipeline store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => new Response(JSON.stringify(payload), { status: 200 })),
    )
  })

  afterEach(() => {
    usePipelineStore().stop()
    vi.unstubAllGlobals()
  })

  it('启动后拉取快照并提供派生数据', async () => {
    const store = usePipelineStore()
    store.start()
    await flushPromises()

    expect(store.loading).toBe(false)
    expect(store.repos).toHaveLength(1)
    expect(store.repoByName('tf-gateway')?.name).toBe('tf-gateway')
    expect(store.rateText).toContain('4999/5000')
  })

  it('请求失败时记录错误并保留上一次数据', async () => {
    const store = usePipelineStore()
    store.start()
    await flushPromises()

    vi.stubGlobal(
      'fetch',
      vi.fn(async () => new Response('boom', { status: 500 })),
    )
    await store.refresh()

    expect(store.error).toContain('HTTP 500')
    expect(store.repos).toHaveLength(1)
  })
})
