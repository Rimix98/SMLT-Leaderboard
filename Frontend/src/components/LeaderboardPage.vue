<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue'
import { store } from '../store'
import { resolveCountry, CODE_TO_NAME, getFlagCode } from '../api/utils'
import { debounce } from '../utils/debounce'
import AppShell from './AppShell.vue'
import ProfileModal from './ProfileModal.vue'
import CountryModal from './CountryModal.vue'
import LevelVictorsModal from './LevelVictorsModal.vue'
import {
  loadAllPlayers,
  filterPlayers,
  getFilteredLevels,
  expandLevels,
  filterLevels,
  addPlayer as addPlayerApi,
  removePlayer as removePlayerApi,
} from '../api/leaderboard'
import { checkLeaderboardChanged, setLastLeaderboardHash } from '../api/history'
import {
  Trophy, AlertTriangle, RefreshCw, Crown, Plus, BookOpen,
  Globe, BarChart3,
} from '@lucide/vue'

const activeTab = ref('players')
const playerSearch = ref('')
const leaderboardLoading = ref(true)
const leaderboardError = ref(false)

const tabHeaderRef = ref(null)
const playerTabBtn = ref(null)
const levelTabBtn = ref(null)
const indicatorStyle = ref({ left: '0px', width: '0px' })

function updateIndicator(tab) {
  const btn = tab === 'players' ? playerTabBtn.value : levelTabBtn.value
  if (!btn || !tabHeaderRef.value) return
  const headerRect = tabHeaderRef.value.getBoundingClientRect()
  const btnRect = btn.getBoundingClientRect()
  indicatorStyle.value = {
    left: `${btnRect.left - headerRect.left}px`,
    width: `${btnRect.width}px`,
  }
}

function switchTab(tab) {
  activeTab.value = tab
  nextTick(() => updateIndicator(tab))
}

async function loadLeaderboard() {
  leaderboardLoading.value = true
  leaderboardError.value = false
  try {
    await loadAllPlayers()
    if (store.players.length === 0) throw new Error('no data')
  } catch {
    leaderboardError.value = true
  } finally {
    leaderboardLoading.value = false
  }
}

const totalPoints = computed(() => store.players.reduce((sum, p) => sum + (p.score || 0), 0))
const averagePoints = computed(() => store.players.length === 0 ? 0 : totalPoints.value / store.players.length)
const totalCompletedLevels = computed(() => store.levels.all?.length || 0)

const topLevelByVictors = computed(() => {
  if (!store.levels.all || store.levels.all.length === 0) return null
  return store.levels.all.reduce<import('../types').LevelData | null>((max, l) => (l.victors.length > (max?.victors?.length || 0)) ? l : max, null)
})

const countryStats = computed(() => {
  const counts: Record<string, { name: string | null; count: number }> = {}
  let unknownCount = 0
  store.players.forEach(p => {
    const country = p.nationality
    if (country) {
      const key = country.toLowerCase().trim().replace(/\s+/g, '-')
      if (!counts[key]) counts[key] = { name: country, count: 0 }
      counts[key].count++
    } else {
      unknownCount++
    }
  })
  const result = Object.values(counts)
    .map(c => {
      const code = resolveCountry(c.name)
      return { ...c, code, displayName: code ? (CODE_TO_NAME[code] || code) : 'Неизвестно' }
    })
    .sort((a, b) => b.count - a.count)
  if (unknownCount > 0) {
    result.push({ name: null, code: null, count: unknownCount, displayName: 'Неизвестно' })
  }
  return result
})

const displayedLevels = computed(() => getFilteredLevels())

let pollTimer: ReturnType<typeof setInterval> | null = null

async function pollLeaderboard() {
  if (document.hidden) return
  const changed = await checkLeaderboardChanged()
  if (changed) {
    await loadAllPlayers()
  }
}

onMounted(async () => {
  await loadLeaderboard()
  setLastLeaderboardHash('')
  pollTimer = setInterval(pollLeaderboard, 30000)
  nextTick(() => updateIndicator('players'))
})

onUnmounted(() => {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
})

function retryLoad() { loadLeaderboard() }

const debouncedFilterPlayers = debounce((q: string) => filterPlayers(q), 250)
const debouncedFilterLevels = debounce((q: string) => filterLevels(q), 250)

const profileModalIndex = ref(-1)
const countryModalName = ref(null)
const countryModalVisible = ref(false)
const levelModalId = ref(null)
const levelModalVisible = ref(false)

function openProfile(index) {
  document.body.classList.add('modal-open')
  profileModalIndex.value = index
}
function closeProfile() {
  profileModalIndex.value = -1
  document.body.classList.remove('modal-open')
}
function openCountry(name) {
  document.body.classList.add('modal-open')
  countryModalName.value = name
  countryModalVisible.value = true
}
function closeCountry() {
  countryModalVisible.value = false
  document.body.classList.remove('modal-open')
}
function openLevel(id) {
  document.body.classList.add('modal-open')
  levelModalId.value = id
  levelModalVisible.value = true
}
function closeLevel() {
  levelModalVisible.value = false
  document.body.classList.remove('modal-open')
}

const addPlayerModalVisible = ref(false)
const newPlayerName = ref('')

function openAddPlayerModal() {
  newPlayerName.value = ''
  addPlayerModalVisible.value = true
  document.body.classList.add('modal-open')
}

function closeAddPlayerModal() {
  addPlayerModalVisible.value = false
  document.body.classList.remove('modal-open')
}

async function doAddPlayer() {
  const name = newPlayerName.value.trim()
  if (!name) return
  await addPlayerApi(name)
  closeAddPlayerModal()
}

function doRemovePlayer(name) {
  removePlayerApi(name)
}
</script>

<template>
  <AppShell>
    <template #brand>
      <span class="header-logo"><Trophy :size="20" /></span>
      <div class="header-title">
        <h1>Лидерборд SMLT</h1>
        <span class="header-subtitle">Лидерборд и топ уровней</span>
      </div>
    </template>
  </AppShell>

  <main class="app-main">
    <div class="container">

      <div v-if="leaderboardLoading" class="loading-state">
        <div class="leaderboard-section" style="width:100%">
          <div v-for="i in 8" :key="i" class="skeleton-row">
            <div class="skeleton skeleton-circle" style="width:24px;height:24px"></div>
            <div style="flex:1">
              <div class="skeleton skeleton-text skeleton-text-sm" style="width:120px"></div>
              <div class="skeleton skeleton-text skeleton-text-sm" style="width:80px;height:10px;margin-top:4px"></div>
            </div>
            <div class="skeleton skeleton-text" style="width:80px"></div>
            <div class="skeleton skeleton-text" style="width:50px"></div>
          </div>
        </div>
        <div class="loading-text">Загрузка лидерборда...</div>
      </div>

      <div v-else-if="leaderboardError" class="loading-state">
        <div class="error-icon"><AlertTriangle :size="48" /></div>
        <div class="error-text">Не удалось загрузить лидерборд</div>
        <button class="btn btn-primary modal-actions-row-spaced" @click="retryLoad"><RefreshCw :size="16" /> Повторить попытку</button>
      </div>

      <template v-else>
        <div v-if="store.isHost" class="admin-panel">
          <div class="admin-panel-header"><Crown :size="16" /> Управление игроками</div>
          <div class="admin-panel-content">
            <button class="btn btn-primary" @click="openAddPlayerModal"><Plus :size="16" /> Добавить игрока</button>
          </div>
        </div>

        <div class="demonlist-tabs">
          <div class="demonlist-tab-header" ref="tabHeaderRef">
            <div class="tab-indicator" :style="indicatorStyle"></div>
            <button ref="playerTabBtn" class="demonlist-tab-btn" :class="{ active: activeTab === 'players' }" @click="switchTab('players')"><Trophy :size="16" /> Топ игроков</button>
            <button ref="levelTabBtn" class="demonlist-tab-btn" :class="{ active: activeTab === 'levels' }" @click="switchTab('levels')"><BookOpen :size="16" /> Топ уровней</button>
          </div>

          <Transition name="tab" mode="out-in">
            <div v-if="activeTab === 'players'" key="players" class="leaderboard-section">
              <div class="leaderboard-header">
                <h2><Trophy :size="18" /> Топ игроков</h2>
                <div class="leaderboard-controls">
                  <input type="text" class="search-input" placeholder="Поиск по нику..." v-model="playerSearch" @input="debouncedFilterPlayers(playerSearch)">
                  <div class="leaderboard-stats">{{ store.players.length }} игроков</div>
                </div>
              </div>
              <div class="leaderboard-table" id="leaderboardTable">
                <div class="table-header">
                  <div class="cell cell-position">#</div>
                  <div class="cell cell-player">Игрок</div>
                  <div class="cell cell-records">Hardest</div>
                  <div class="cell cell-points">Очки</div>
                </div>
                <TransitionGroup name="list" tag="div">
                  <div v-for="(p, index) in store.players" :key="p.id ?? index" class="player-row" @click="openProfile(index)">
                  <div class="cell cell-position" :class="index === 0 ? 'rank-1' : index === 1 ? 'rank-2' : index === 2 ? 'rank-3' : 'rank-other'">
                    {{ index + 1 }}
                  </div>
                  <div class="cell cell-player">
                    <span class="player-flag">
                      <img v-if="getFlagCode(p.nationality)" :src="`https://flagcdn.com/w20/${getFlagCode(p.nationality)}.png`" :alt="getFlagCode(p.nationality).toUpperCase()" width="20" loading="lazy" class="flag-img flag-inline">
                      <span v-else><Globe :size="16" /></span>
                    </span>
                    <div class="player-info">
                      <span class="player-name">{{ p.name }}</span>
                      <span class="player-score">{{ (p.score || 0).toFixed(2) }} pts · #{{ p.rank || '—' }}</span>
                    </div>
                    <button v-if="store.isHost" class="btn btn-danger btn-xs player-delete-btn" @click.stop="doRemovePlayer(p.name)">✕</button>
                  </div>
                  <div class="cell cell-records">{{ p.hardest?.level?.name || '—' }}</div>
                  <div class="cell cell-points">{{ (p.score || 0).toFixed(2) }}</div>
                </div>
                <div v-if="store.players.length === 0" class="empty-state">
                <div class="empty-state-icon"><Trophy :size="48" /></div>
                  <p>Игроки не найдены</p>
                  <p class="no-data-text">Попробуйте изменить поисковый запрос</p>
                </div>
                </TransitionGroup>
              </div>
            </div>

            <div v-else key="levels" class="leaderboard-section">
              <div class="leaderboard-header">
                <h2><BookOpen :size="18" /> Топ уровней</h2>
                <div class="leaderboard-controls">
                  <input type="text" class="search-input" placeholder="Поиск по уровню..." v-model="store.levels.filter" @input="debouncedFilterLevels(store.levels.filter)">
                  <div class="leaderboard-stats">{{ store.levels.all?.length || 0 }} уровней</div>
                </div>
              </div>
              <div class="leaderboard-table" id="levelsTable">
                <div class="table-header">
                  <div class="cell cell-position">#</div>
                  <div class="cell cell-player">Уровень</div>
                  <div class="cell cell-points">Позиция</div>
                  <div class="cell cell-records">Викторов</div>
                </div>
                <TransitionGroup name="list" tag="div">
                  <div v-for="(level, index) in displayedLevels" :key="level.id" class="player-row" @click="openLevel(level.id)">
                  <div class="cell cell-position" :class="index === 0 ? 'rank-1' : index === 1 ? 'rank-2' : index === 2 ? 'rank-3' : 'rank-other'">
                    {{ index + 1 }}
                  </div>
                  <div class="cell cell-player">
                    <div class="player-info">
                      <span class="player-name">{{ level.name }}</span>
                    </div>
                  </div>
                  <div class="cell cell-points">#{{ level.placement }}</div>
                  <div class="cell cell-records">{{ level.victors.length }}</div>
                </div>
                <div v-if="!store.levels.all" class="empty-state">
                  <div class="empty-state-icon"><BookOpen :size="48" /></div>
                  <p>Нет данных об уровнях</p>
                  <p class="no-data-text">Уровни появятся после добавления записей</p>
                </div>
                </TransitionGroup>
              </div>
              <div v-if="store.levels.all && store.levels.all.length > 39" class="expand-levels-footer">
                <button class="btn btn-secondary btn-sm" @click="expandLevels">
                  {{ store.levels.expanded ? 'Свернуть' : 'Показать ещё' }}
                </button>
              </div>
            </div>
          </Transition>
      </div>

      <div class="stats-grid">
        <div class="stats-section">
          <h3><BarChart3 :size="16" /> Статистика</h3>
          <div class="stats-grid-main">
            <div class="stat-card">
              <div class="stat-value">{{ store.players.length }}</div>
              <div class="stat-label">Игроков</div>
            </div>
            <div class="stat-card">
              <div class="stat-value">{{ averagePoints.toFixed(2) }}</div>
              <div class="stat-label">Среднее очков</div>
            </div>
            <div class="stat-card">
              <div class="stat-value">{{ totalCompletedLevels }}</div>
              <div class="stat-label">Пройдено уровней</div>
            </div>
            <div class="stat-card">
              <div class="stat-value stat-card-compact"
                :title="topLevelByVictors ? `${topLevelByVictors.name} — ${topLevelByVictors.victors.length} victors` : ''">
                {{ topLevelByVictors?.name || '—' }}
              </div>
              <div class="stat-label">Топ уровень</div>
            </div>
          </div>
        </div>
        <div class="stats-section">
          <h3><Globe :size="16" /> По странам</h3>
          <div class="country-list" id="countryList">
            <div v-for="c in countryStats" :key="c.code" class="country-item country-item-clickable" @click="openCountry(c.name)">
              <div class="country-info">
                <span class="country-flag">
                  <img v-if="getFlagCode(c.name)" :src="`https://flagcdn.com/w20/${getFlagCode(c.name)}.png`" :alt="getFlagCode(c.name).toUpperCase()" width="20" loading="lazy" class="flag-img flag-inline">
                  <span v-else><Globe :size="16" /></span>
                </span>
                <span class="country-name">{{ c.displayName }}</span>
              </div>
              <span class="country-count">{{ c.count }}</span>
            </div>
            <div v-if="countryStats.length === 0" class="no-data-text">Нет данных</div>
          </div>
        </div>
      </div>
      </template>
    </div>
  </main>

  <Teleport to="body">
    <ProfileModal :player-index="profileModalIndex" @close="closeProfile" />
    <CountryModal :country-name="countryModalName" :visible="countryModalVisible" @close="closeCountry" />
    <LevelVictorsModal :level-id="levelModalId" :visible="levelModalVisible" @close="closeLevel" />

    <div class="modal-overlay" :class="{ active: addPlayerModalVisible }" @mousedown="closeAddPlayerModal" @mouseup="closeAddPlayerModal">
      <div class="modal" @mousedown.stop @mouseup.stop>
        <div class="modal-header">
          <div class="modal-title"><Plus :size="16" /> Добавить игрока</div>
          <button class="modal-close" @click="closeAddPlayerModal">✕</button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label>Ник или айди в GDL:</label>
            <input type="text" class="form-input" placeholder="Например: samoletik" v-model="newPlayerName" @keyup.enter="doAddPlayer">
          </div>
          <div class="modal-actions-row">
            <button class="btn btn-secondary" @click="closeAddPlayerModal">Отмена</button>
            <button class="btn btn-primary" @click="doAddPlayer">Добавить</button>
          </div>
        </div>
      </div>
    </div>
  </Teleport>
</template>
