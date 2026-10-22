const LOG_TOKEN_KEY = 'tf-console.log-token'

export type LogEnv = 'all' | 'prod' | 'test'
export type LogContextLines = 30 | 50 | 100

export interface LogMeta {
  authRequired: boolean
  lokiAvailable: boolean
  retentionDays: number
  contextOptions: number[]
  rangeOptions: string[]
}

export interface LogEntry {
  timestamp: string
  timestampNs: string
  level: string
  service: string
  env: string
  logger?: string
  message: string
  line: string
  matched: boolean
}

export interface LogSearchResponse {
  id: string
  env: LogEnv
  contextLines: number
  range: string
  start: string
  end: string
  matched: number
  streams: number
  truncated: boolean
  warnings?: string[]
  entries: LogEntry[]
}

export interface LogSearchParams {
  id: string
  env: LogEnv
  lines: LogContextLines
  range: string
}

export class LogApiError extends Error {
  constructor(
    readonly status: number,
    message: string,
    readonly code?: string,
  ) {
    super(message)
    this.name = 'LogApiError'
  }
}

export function getLogToken(): string {
  try {
    return sessionStorage.getItem(LOG_TOKEN_KEY) ?? ''
  } catch {
    return ''
  }
}

export function setLogToken(token: string) {
  try {
    sessionStorage.setItem(LOG_TOKEN_KEY, token)
  } catch {
    // 无痕模式等场景下忽略存储失败
  }
}

export function clearLogToken() {
  try {
    sessionStorage.removeItem(LOG_TOKEN_KEY)
  } catch {
    // ignore
  }
}

/**
 * 从粘贴内容中提取链路编号。
 * 前端错误提示形如“发送过于频繁（编号 4bf9...）”，允许用户整段粘贴。
 */
export function normalizeLogId(raw: string): string {
  const match = raw.trim().match(/[0-9a-fA-F]{8,64}/)
  if (!match) return ''
  const id = match[0].toLowerCase()
  return /^[0-9a-f]{8,64}$/.test(id) ? id : ''
}

async function logRequest<T>(url: string, token = ''): Promise<T> {
  const headers: Record<string, string> = { Accept: 'application/json' }
  if (token) headers['X-Log-Token'] = token

  let response: Response
  try {
    response = await fetch(url, { headers })
  } catch {
    throw new LogApiError(0, '无法连接日志服务，请检查网络')
  }

  let body: unknown = null
  try {
    body = await response.json()
  } catch {
    // 保留状态码，使用兜底文案
  }

  if (!response.ok) {
    const detail = body as { message?: string; error?: string } | null
    throw new LogApiError(
      response.status,
      detail?.message || `日志服务返回 HTTP ${response.status}`,
      detail?.error,
    )
  }
  if (body === null) {
    throw new LogApiError(response.status, '日志服务返回了无法解析的数据')
  }
  return body as T
}

export function fetchLogMeta(): Promise<LogMeta> {
  return logRequest<LogMeta>('/api/v1/logs/meta')
}

export function searchLogs(params: LogSearchParams, token = ''): Promise<LogSearchResponse> {
  const query = new URLSearchParams({
    id: params.id,
    env: params.env,
    lines: String(params.lines),
    range: params.range,
  })
  return logRequest<LogSearchResponse>(`/api/v1/logs/search?${query.toString()}`, token)
}
