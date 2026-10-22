import { afterEach, describe, expect, it, vi } from 'vitest'

import {
  LogApiError,
  clearLogToken,
  fetchLogMeta,
  getLogToken,
  normalizeLogId,
  searchLogs,
  setLogToken,
} from '../logs'

const TRACE_ID = '4bf92f3577b34da6a3ce929d0e0e4736'

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

const searchBody = {
  id: TRACE_ID,
  env: 'prod' as const,
  contextLines: 50,
  range: '7d',
  start: '2026-10-15T00:00:00Z',
  end: '2026-10-22T00:00:00Z',
  matched: 1,
  streams: 1,
  truncated: false,
  entries: [
    {
      timestamp: '2026-10-22T10:00:00+08:00',
      timestampNs: '1761098400000000000',
      level: 'error',
      service: 'tf-user',
      env: 'production',
      message: 'boom',
      line: '{"message":"boom"}',
      matched: true,
    },
  ],
}

afterEach(() => {
  vi.unstubAllGlobals()
  sessionStorage.clear()
})

describe('normalizeLogId', () => {
  it('从完整错误提示中提取编号并转小写', () => {
    expect(normalizeLogId(`发送过于频繁（编号 ${TRACE_ID.toUpperCase()}）`)).toBe(TRACE_ID)
    expect(normalizeLogId(`  ${TRACE_ID}  `)).toBe(TRACE_ID)
    expect(normalizeLogId('deadbeef')).toBe('deadbeef')
  })

  it('无法识别时返回空字符串', () => {
    expect(normalizeLogId('')).toBe('')
    expect(normalizeLogId('请求失败')).toBe('')
    expect(normalizeLogId('xyz12345')).toBe('')
  })
})

describe('searchLogs', () => {
  it('携带筛选参数与口令，并解析结果', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(jsonResponse(searchBody))
    vi.stubGlobal('fetch', fetchMock)

    const result = await searchLogs(
      { id: TRACE_ID, env: 'prod', lines: 50, range: '7d' },
      'log-secret',
    )

    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe(
      `/api/v1/logs/search?id=${TRACE_ID}&env=prod&lines=50&range=7d`,
    )
    expect(init.headers).toMatchObject({ 'X-Log-Token': 'log-secret' })
    expect(result.matched).toBe(1)
    expect(result.entries[0]?.level).toBe('error')
  })

  it('口令错误时抛出带状态码的 LogApiError', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn<typeof fetch>().mockResolvedValue(
        jsonResponse({ error: 'UNAUTHORIZED', message: '日志检索口令无效' }, 401),
      ),
    )

    const error = await searchLogs(
      { id: TRACE_ID, env: 'all', lines: 30, range: '24h' },
      'wrong',
    ).catch((value: unknown) => value)

    expect(error).toBeInstanceOf(LogApiError)
    expect(error).toMatchObject({ status: 401, code: 'UNAUTHORIZED', message: '日志检索口令无效' })
  })

  it('网络不可用时返回状态码 0', async () => {
    vi.stubGlobal('fetch', vi.fn<typeof fetch>().mockRejectedValue(new TypeError('fetch failed')))
    const error = await fetchLogMeta().catch((value: unknown) => value)
    expect(error).toBeInstanceOf(LogApiError)
    expect(error).toMatchObject({ status: 0 })
  })
})

describe('日志检索口令存储', () => {
  it('写入、读取与清除', () => {
    setLogToken('secret')
    expect(getLogToken()).toBe('secret')
    clearLogToken()
    expect(getLogToken()).toBe('')
  })
})
