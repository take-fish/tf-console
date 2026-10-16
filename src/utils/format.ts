const rtf = new Intl.RelativeTimeFormat('zh-CN', { numeric: 'auto' })

// 相对时间：刚刚 / 5 分钟前 / 2 小时前 / 3 天前
export function relativeTime(iso: string): string {
  if (!iso) return '-'
  const t = new Date(iso).getTime()
  if (Number.isNaN(t)) return '-'
  const diffSec = Math.round((t - Date.now()) / 1000)
  const abs = Math.abs(diffSec)
  if (abs < 60) return '刚刚'
  if (abs < 3600) return rtf.format(Math.round(diffSec / 60), 'minute')
  if (abs < 86400) return rtf.format(Math.round(diffSec / 3600), 'hour')
  return rtf.format(Math.round(diffSec / 86400), 'day')
}

export function shortSha(sha: string): string {
  return sha ? sha.slice(0, 7) : '-'
}

export function clampPercent(n: number): number {
  if (Number.isNaN(n)) return 0
  return Math.min(100, Math.max(0, Math.round(n)))
}
