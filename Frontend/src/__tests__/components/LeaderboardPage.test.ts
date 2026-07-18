// @vitest-environment happy-dom
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { shallowMount } from '@vue/test-utils'
import LeaderboardPage from '../../components/LeaderboardPage.vue'

vi.mock('vue-router', () => ({
  useRoute: () => ({
    name: 'leaderboard',
    path: '/leaderboard',
    meta: { title: 'Лидерборд -- SMLT' },
  }),
  useRouter: () => ({ push: vi.fn() }),
}))

vi.mock('../../api/leaderboard', () => ({
  loadAllPlayers: vi.fn(),
  filterPlayers: vi.fn(),
  getFilteredLevels: vi.fn(() => []),
  expandLevels: vi.fn(),
  filterLevels: vi.fn(),
  addPlayer: vi.fn(),
  removePlayer: vi.fn(),
}))

vi.mock('../../api/history', () => ({
  checkLeaderboardChanged: vi.fn(),
  setLastLeaderboardHash: vi.fn(),
}))

describe('LeaderboardPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders the page', () => {
    const wrapper = shallowMount(LeaderboardPage)
    expect(wrapper.exists()).toBe(true)
  })

  it('shows loading state initially', () => {
    const wrapper = shallowMount(LeaderboardPage)
    expect(wrapper.html()).toContain('Загрузка')
  })

  it('has the leaderboard page structure', () => {
    const wrapper = shallowMount(LeaderboardPage)
    const html = wrapper.html()
    expect(html).toContain('Загрузка')
  })
})
