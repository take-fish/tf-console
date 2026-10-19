import type { BranchView, CommitView } from '@/types'

async function getJSON<T>(url: string): Promise<T> {
  const resp = await fetch(url)
  if (!resp.ok) {
    let message = `HTTP ${resp.status}`
    try {
      const body = (await resp.json()) as { msg?: string }
      if (body.msg) message = body.msg
    } catch {
      // 非 JSON 错误体
    }
    throw new Error(message)
  }
  return (await resp.json()) as T
}

export async function fetchBranches(repo: string): Promise<BranchView[]> {
  const data = await getJSON<{ branches: BranchView[] }>(
    `/api/v1/repos/${encodeURIComponent(repo)}/branches`,
  )
  return data.branches ?? []
}

export async function fetchCommits(
  repo: string,
  branch: string,
  limit = 30,
): Promise<CommitView[]> {
  const query = new URLSearchParams({ branch, limit: String(limit) })
  const data = await getJSON<{ commits: CommitView[] }>(
    `/api/v1/repos/${encodeURIComponent(repo)}/commits?${query.toString()}`,
  )
  return data.commits ?? []
}
