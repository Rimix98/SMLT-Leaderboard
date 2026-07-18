// @vitest-environment happy-dom
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { shallowMount } from '@vue/test-utils'
import StaffPage from '../../components/StaffPage.vue'

vi.mock('vue-router', () => ({
  useRoute: () => ({
    name: 'staff',
    path: '/staff',
    meta: { title: 'Стафф -- SMLT' },
  }),
  useRouter: () => ({ push: vi.fn() }),
}))

vi.mock('../../api/staff', () => ({
  loadStaffRoles: vi.fn(),
  loadStaffTiers: vi.fn(),
  TIER_CONFIG: {
    priority: { label: 'Приоритет', color: '#00ffff' },
    base: { label: 'Основа', color: '#540b6d' },
    reserve: { label: 'Резерв', color: '#6d0b0d' },
    na: { label: 'N/A', color: '#888888' },
  },
}))

describe('StaffPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders the page', () => {
    const wrapper = shallowMount(StaffPage)
    expect(wrapper.exists()).toBe(true)
  })

  it('contains staff-related content', () => {
    const wrapper = shallowMount(StaffPage)
    const html = wrapper.html()
    expect(html.length).toBeGreaterThan(100)
  })
})
