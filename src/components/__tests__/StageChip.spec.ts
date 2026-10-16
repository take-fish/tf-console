import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import StageChip from '@/components/StageChip.vue'

describe('StageChip', () => {
  it('无 href 时渲染 span 并带 tone class', () => {
    const w = mount(StageChip, { props: { value: '待审核 #3', tone: 'warn' } })
    expect(w.element.tagName).toBe('SPAN')
    expect(w.classes()).toContain('chip--warn')
    expect(w.text()).toBe('待审核 #3')
  })

  it('有 href 时渲染外链 a 标签', () => {
    const w = mount(StageChip, {
      props: { value: 'v0.1.1', tone: 'ok', href: 'https://github.com/x/y' },
    })
    const a = w.get('a')
    expect(a.attributes('href')).toBe('https://github.com/x/y')
    expect(a.attributes('target')).toBe('_blank')
    expect(a.classes()).toContain('chip--ok')
  })

  it('hint 渲染为 title', () => {
    const w = mount(StageChip, { props: { value: 'x', tone: 'idle', hint: '提示文本' } })
    expect(w.attributes('title')).toBe('提示文本')
  })
})
