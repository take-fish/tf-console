import { describe, expect, it } from 'vitest'

import type { BranchView } from '@/types'
import { groupBranches, pickDefaultBranch } from '../branches'

const branches: BranchView[] = [
  { name: 'feature/zeta', short_sha: '1111111', protected: false },
  { name: 'test', short_sha: '2222222', protected: false },
  { name: 'master', short_sha: '3333333', protected: true },
  { name: 'feature/alpha', short_sha: '4444444', protected: false },
]

describe('branch grouping', () => {
  it('分出 master/test/自建分支并排序', () => {
    const groups = groupBranches(branches)
    expect(groups.master?.name).toBe('master')
    expect(groups.test?.name).toBe('test')
    expect(groups.custom.map((branch) => branch.name)).toEqual([
      'feature/alpha',
      'feature/zeta',
    ])
  })

  it('默认分支优先 master，其次 test', () => {
    expect(pickDefaultBranch(groupBranches(branches))).toBe('master')
    expect(
      pickDefaultBranch(
        groupBranches(branches.filter((branch) => branch.name !== 'master')),
      ),
    ).toBe('test')
    expect(
      pickDefaultBranch(
        groupBranches([
          { name: 'feature/only', short_sha: '5555555', protected: false },
        ]),
      ),
    ).toBe('feature/only')
  })
})
