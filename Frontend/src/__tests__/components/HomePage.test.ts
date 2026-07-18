// @vitest-environment happy-dom
import { describe, it, expect, vi } from 'vitest'
import { shallowMount } from '@vue/test-utils'
import HomePage from '../../components/HomePage.vue'

vi.mock('vue-router', () => ({
  useRoute: () => ({
    name: 'home',
    path: '/',
    meta: { title: 'SMLT - Главная', bodyClass: 'home-page' },
  }),
  useRouter: () => ({ push: vi.fn() }),
}))

describe('HomePage', () => {
  it('renders the hero section', () => {
    const wrapper = shallowMount(HomePage)
    expect(wrapper.html()).toContain('Что такое SMLT')
  })

  it('renders navigation cards', () => {
    const wrapper = shallowMount(HomePage)
    const html = wrapper.html()
    expect(html).toContain('Лидерборд')
    expect(html).toContain('Проекты')
    expect(html).toContain('Стафф')
  })

  it('contains nav-card links', () => {
    const wrapper = shallowMount(HomePage)
    const links = wrapper.findAll('.nav-card')
    expect(links.length).toBeGreaterThanOrEqual(3)
  })

  it('renders description text', () => {
    const wrapper = shallowMount(HomePage)
    const html = wrapper.html()
    expect(html).toContain('дискорд сервер')
    expect(html).toContain('коллабы')
  })

  it('has the highlight class', () => {
    const wrapper = shallowMount(HomePage)
    expect(wrapper.find('.highlight').exists()).toBe(true)
  })
})
