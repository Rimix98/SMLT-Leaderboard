// @vitest-environment happy-dom
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { shallowMount } from '@vue/test-utils'
import AppShell from '../../components/AppShell.vue'

vi.mock('vue-router', () => ({
  useRoute: () => ({
    name: 'home',
    path: '/',
    meta: { title: 'SMLT' },
  }),
  useRouter: () => ({ push: vi.fn() }),
}))

describe('AppShell', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders brand slot', () => {
    const wrapper = shallowMount(AppShell, {
      slots: { brand: '<span class="brand-test">SMLT Logo</span>' },
    })
    expect(wrapper.find('.brand-test').exists()).toBe(true)
    expect(wrapper.find('.brand-test').text()).toBe('SMLT Logo')
  })

  it('renders the main content area', () => {
    const wrapper = shallowMount(AppShell)
    expect(wrapper.find('.header-nav').exists()).toBe(true)
    expect(wrapper.find('.host-btn').exists()).toBe(true)
  })

  it('renders the header nav', () => {
    const wrapper = shallowMount(AppShell)
    expect(wrapper.find('.header-nav').exists()).toBe(true)
  })

  it('renders without errors when no slots provided', () => {
    const wrapper = shallowMount(AppShell)
    expect(wrapper.exists()).toBe(true)
    expect(wrapper.html().length).toBeGreaterThan(100)
  })

  it('has theme selector button', () => {
    const wrapper = shallowMount(AppShell)
    expect(wrapper.find('.theme-btn, [class*="theme"]').exists()).toBe(true)
  })
})
