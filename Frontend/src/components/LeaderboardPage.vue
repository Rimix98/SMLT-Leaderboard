<script setup>
import { computed, nextTick, onMounted, ref } from 'vue'
import { store, initTheme } from '../store'
import { refreshCsrfToken, resolveCountry, CODE_TO_NAME, getFlagCode } from '../api/utils'
import AppShell from './AppShell.vue'
import {
  loadAllPlayers,
  filterPlayers,
  getFilteredLevels,
  expandLevels,
  filterLevels,
  showLevelVictors,
  closeLevelModal,
  showProfile,
  closeProfileModal,
  showCountryTop,
  closeCountryModal,
  showAddPlayerModal,
  removePlayer,
  addPlayer,
  closeAddPlayerModal,
} from '../api/leaderboard'

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

const averagePoints = computed(() => {
  if (store.players.length === 0) return 0
  return totalPoints.value / store.players.length
})

const totalCompletedLevels = computed(() => store.levels.all?.length || 0)

const topLevelByVictors = computed(() => {
  if (!store.levels.all || store.levels.all.length === 0) return null
  return store.levels.all.reduce((max, l) => (l.victors.length > (max?.victors?.length || 0)) ? l : max, null)
})

const countryStats = computed(() => {
  const counts = {}
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
      return { ...c, code, displayName: code ? (CODE_TO_NAME[code] || code) : 'Unknown' }
    })
    .sort((a, b) => b.count - a.count)
  if (unknownCount > 0) {
    result.push({ name: null, code: null, count: unknownCount, displayName: 'Unknown' })
  }
  return result
})

const displayedLevels = computed(() => getFilteredLevels())

onMounted(async () => {
  initTheme()
  await refreshCsrfToken()
  loadLeaderboard()
  nextTick(() => updateIndicator('players'))
})

function retryLoad() {
  loadLeaderboard()
}

// Modal close helpers: only close if mousedown AND mouseup are on overlay
function makeOverlayClose(closeFn) {
  let mousedownOverlay = false
  return {
    onMousedown(e) {
      mousedownOverlay = e.target === e.currentTarget
    },
    onMouseup(e) {
      if (mousedownOverlay && e.target === e.currentTarget) {
        closeFn()
      }
      mousedownOverlay = false
    }
  }
}

const profileModalClose = makeOverlayClose(() => {
  closeProfileModal()
  document.body.classList.remove('modal-open')
})
const countryModalClose = makeOverlayClose(() => {
  closeCountryModal()
  document.body.classList.remove('modal-open')
})
const levelModalClose = makeOverlayClose(() => {
  closeLevelModal()
  document.body.classList.remove('modal-open')
})
const addPlayerModalClose = makeOverlayClose(() => {
  closeAddPlayerModal()
  document.body.classList.remove('modal-open')
})

function onProfileOpen(index) {
  showProfile(index)
  document.body.classList.add('modal-open')
}

function onCountryTop(name) {
  showCountryTop(name)
  document.body.classList.add('modal-open')
}

function onLevelVictors(id) {
  showLevelVictors(id)
  document.body.classList.add('modal-open')
}

function showAddPlayerModalAndLock() {
  showAddPlayerModal()
  document.body.classList.add('modal-open')
}

function closeProfileAndUnlock() {
  closeProfileModal()
  document.body.classList.remove('modal-open')
}

function closeCountryAndUnlock() {
  closeCountryModal()
  document.body.classList.remove('modal-open')
}

function closeLevelAndUnlock() {
  closeLevelModal()
  document.body.classList.remove('modal-open')
}

function closeAddPlayerAndUnlock() {
  closeAddPlayerModal()
  document.body.classList.remove('modal-open')
}

function addPlayerAndClose() {
  addPlayer()
  document.body.classList.remove('modal-open')
}
</script>

<template>
  <AppShell page="leaderboard">
    <template #brand>
      <span class="header-logo">🏆</span>
      <div class="header-title">
        <h1>Лидерборд SMLT</h1>
        <span class="header-subtitle">Лидерборд и топ уровней</span>
      </div>
    </template>
    <template #actions>
    </template>
  </AppShell>

  <main class="app-main">
    <div class="container">

      <div v-if="leaderboardLoading" class="loading-state">
        <div class="bar-spinner"></div>
        <div class="loading-text">Загрузка лидерборда...</div>
      </div>

      <div v-else-if="leaderboardError" class="loading-state">
        <div class="error-icon">⚠️</div>
        <div class="error-text">Не удалось загрузить лидерборд</div>
        <button class="btn btn-primary" style="margin-top:var(--spacing-md)" @click="retryLoad">🔄 Повторить попытку</button>
      </div>

      <template v-else>
        <div v-if="store.isHost" class="admin-panel">
          <div class="admin-panel-header">👑 Управление игроками</div>
          <div class="admin-panel-content">
            <button class="btn btn-primary" @click="showAddPlayerModalAndLock()">➕ Добавить игрока</button>
          </div>
        </div>

        <div class="demonlist-tabs">
          <div class="demonlist-tab-header" ref="tabHeaderRef">
            <div class="tab-indicator" :style="indicatorStyle"></div>
            <button ref="playerTabBtn" class="demonlist-tab-btn" :class="{ active: activeTab === 'players' }" @click="switchTab('players')">🏆 Топ игроков</button>
            <button ref="levelTabBtn" class="demonlist-tab-btn" :class="{ active: activeTab === 'levels' }" @click="switchTab('levels')">📔 Топ уровней</button>
          </div>

          <Transition name="tab" mode="out-in">
            <div v-if="activeTab === 'players'" key="players" class="leaderboard-section">
              <div class="leaderboard-header">
                <h2>🏆 Топ игроков</h2>
                <div class="leaderboard-controls">
                  <input type="text" class="search-input" placeholder="🔍 Поиск по нику..." v-model="playerSearch" @input="filterPlayers(playerSearch)">
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
                  <div v-for="(p, index) in store.players" :key="p.id ?? index" class="player-row" @click="onProfileOpen(index)">
                  <div class="cell cell-position" :class="index === 0 ? 'rank-1' : index === 1 ? 'rank-2' : index === 2 ? 'rank-3' : 'rank-other'">
                    {{ index + 1 }}
                  </div>
                  <div class="cell cell-player">
                    <span class="player-flag"><img v-if="getFlagCode(p.nationality)" :src="`https://flagcdn.com/w20/${getFlagCode(p.nationality)}.png`" :alt="getFlagCode(p.nationality).toUpperCase()" width="20" class="flag-img" style="vertical-align:middle;margin-right:4px"><span v-else>{{ !resolveCountry(p.nationality) && p.nationality === null ? '❌' : '🌍' }}</span></span>
                    <div class="player-info">
                      <span class="player-name">{{ p.name }}</span>
                      <span class="player-score">{{ (p.score || 0).toFixed(2) }} pts · #{{ p.rank || '—' }}</span>
                    </div>
                    <button v-if="store.isHost" class="btn btn-danger btn-xs player-delete-btn" @click.stop="removePlayer(p.name)">✕</button>
                  </div>
                  <div class="cell cell-records">{{ p.hardest?.level?.name || '—' }}</div>
                  <div class="cell cell-points">{{ (p.score || 0).toFixed(2) }}</div>
                </div>
                <div v-if="store.players.length === 0" class="empty-state">
                  <div class="empty-state-icon">🏆</div>
                  <p>Игроки не найдены</p>
                </div>
              </div>
            </div>

            <div v-else key="levels" class="leaderboard-section">
              <div class="leaderboard-header">
                <h2>📔 Топ уровней</h2>
                <div class="leaderboard-controls">
                  <input type="text" class="search-input" placeholder="🔍 Поиск по уровню..." v-model="store.levels.filter" @input="filterLevels(store.levels.filter)">
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
                <div v-for="(level, index) in displayedLevels" :key="level.id" class="player-row" @click="onLevelVictors(level.id)">
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
                  <div class="empty-state-icon">📔</div>
                  <p>Нет данных об уровнях</p>
                </div>
              </div>
              <div v-if="store.levels.all && store.levels.all.length > 39" style="padding:var(--spacing-sm);text-align:center;border-top:1px solid var(--color-border)">
                <button class="btn btn-secondary btn-sm" @click="expandLevels">
                  {{ store.levels.expanded ? 'Свернуть' : 'Показать ещё' }}
                </button>
              </div>
            </div>
          </Transition>
      </div>

      <div class="stats-grid">
        <div class="stats-section">
          <h3>📊 Статистика</h3>
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
              <div class="stat-value" style="font-size: var(--font-size-sm);
                white-space: nowrap; overflow: hidden; text-overflow: ellipsis;"
                :title="topLevelByVictors ? `${topLevelByVictors.name} — ${topLevelByVictors.victors.length} victors` : ''">
                {{ topLevelByVictors?.name || '—' }}
              </div>
              <div class="stat-label">Топ уровень</div>
            </div>
          </div>
        </div>
        <div class="stats-section">
          <h3>🌍 По странам</h3>
          <div class="country-list" id="countryList">
            <div v-for="c in countryStats" :key="c.code" class="country-item" style="cursor:pointer" @click="onCountryTop(c.name)">
              <div class="country-info">
                <span class="country-flag"><img v-if="getFlagCode(c.name)" :src="`https://flagcdn.com/w20/${getFlagCode(c.name)}.png`" :alt="getFlagCode(c.name).toUpperCase()" width="20" class="flag-img" style="vertical-align:middle;margin-right:4px"><span v-else>{{ !resolveCountry(c.name) && c.name === null ? '❌' : '🌍' }}</span></span>
                <span class="country-name">{{ c.displayName }}</span>
              </div>
              <span class="country-count">{{ c.count }}</span>
            </div>
            <div v-if="countryStats.length === 0" style="color:var(--color-text-muted);font-size:var(--font-size-sm)">Нет данных</div>
          </div>
        </div>
      </div>
      </template>
    </div>
  </main>

  <Teleport to="body">
    <div id="profileModal" class="modal-overlay" @mousedown="profileModalClose.onMousedown" @mouseup="profileModalClose.onMouseup">
      <div class="modal" @mousedown.stop @mouseup.stop>
        <div class="modal-header">
          <div class="modal-title" id="profileTitle">Профиль</div>
          <button class="modal-close" @click="closeProfileAndUnlock()">✕</button>
        </div>
        <div class="modal-body" id="profileBody"></div>
      </div>
    </div>

    <div id="countryModal" class="modal-overlay" @mousedown="countryModalClose.onMousedown" @mouseup="countryModalClose.onMouseup">
      <div class="modal" @mousedown.stop @mouseup.stop>
        <div class="modal-header">
          <div class="modal-title" id="countryTitle">Топ страны</div>
          <button class="modal-close" @click="closeCountryAndUnlock()">✕</button>
        </div>
        <div class="modal-body" id="countryBody"></div>
      </div>
    </div>

    <div id="levelModal" class="modal-overlay" @mousedown="levelModalClose.onMousedown" @mouseup="levelModalClose.onMouseup">
      <div class="modal" @mousedown.stop @mouseup.stop>
        <div class="modal-header">
          <div class="modal-title" id="levelTitle">Викторы уровня</div>
          <button class="modal-close" @click="closeLevelAndUnlock()">✕</button>
        </div>
        <div class="modal-body" id="levelBody"></div>
      </div>
    </div>

    <div id="addPlayerModal" class="modal-overlay" @mousedown="addPlayerModalClose.onMousedown" @mouseup="addPlayerModalClose.onMouseup">
      <div class="modal" @mousedown.stop @mouseup.stop>
        <div class="modal-header">
          <div class="modal-title">➕ Добавить игрока</div>
          <button class="modal-close" @click="closeAddPlayerModal">✕</button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label for="newPlayerName">Ник или айди в GDL:</label>
            <input type="text" id="newPlayerName" class="form-input" placeholder="Например: samoletik">
          </div>
          <div style="display:flex;gap:var(--spacing-sm)">
            <button class="btn btn-secondary" @click="closeAddPlayerAndUnlock()">Отмена</button>
            <button class="btn btn-primary" @click="addPlayerAndClose()">Добавить</button>
          </div>
        </div>
      </div>
    </div>
  </Teleport>
</template>
