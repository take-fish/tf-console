import type { BranchView } from '@/types'

export interface BranchGroups {
  master: BranchView | null
  test: BranchView | null
  custom: BranchView[]
}

export function groupBranches(branches: BranchView[]): BranchGroups {
  return {
    master: branches.find((branch) => branch.name === 'master') ?? null,
    test: branches.find((branch) => branch.name === 'test') ?? null,
    custom: branches
      .filter((branch) => branch.name !== 'master' && branch.name !== 'test')
      .sort((a, b) => a.name.localeCompare(b.name)),
  }
}

export function pickDefaultBranch(groups: BranchGroups): string {
  return groups.master?.name ?? groups.test?.name ?? groups.custom[0]?.name ?? 'master'
}
