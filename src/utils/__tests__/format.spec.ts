import { describe, expect, it } from 'vitest'
import { clampPercent, relativeTime, shortSha } from '@/utils/format'

describe('shortSha', () => {
  it('截断为 7 位', () => {
    expect(shortSha('a04e6aef1234567890')).toBe('a04e6ae')
  })
  it('空值返回 -', () => {
    expect(shortSha('')).toBe('-')
  })
})

describe('clampPercent', () => {
  it('限定 0-100', () => {
    expect(clampPercent(-5)).toBe(0)
    expect(clampPercent(140)).toBe(100)
    expect(clampPercent(44.6)).toBe(45)
  })
  it('NaN 归零', () => {
    expect(clampPercent(Number.NaN)).toBe(0)
  })
})

describe('relativeTime', () => {
  it('一分钟内显示刚刚', () => {
    expect(relativeTime(new Date(Date.now() - 10_000).toISOString())).toBe('刚刚')
  })
  it('分钟级', () => {
    const t = relativeTime(new Date(Date.now() - 5 * 60_000).toISOString())
    expect(t).toContain('分钟')
  })
  it('非法输入返回 -', () => {
    expect(relativeTime('')).toBe('-')
    expect(relativeTime('not-a-date')).toBe('-')
  })
})
