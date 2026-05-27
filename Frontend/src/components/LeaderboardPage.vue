<script setup>
import { computed, onMounted, ref } from 'vue'
import { store, initTheme } from '../store'
import { refreshCsrfToken, resolveCountry, CODE_TO_NAME, getFlagHTML } from '../api/utils'
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

const hardestLevelInfo = computed(() => {
  let hardest = null
  let hardestPlayer = null
  store.players.forEach(p => {
    if (p.hardest && p.hardest.level) {
      if (!hardest || p.hardest.level.placement < hardest.placement) {
        hardest = p.hardest.level
        hardestPlayer = p
      }
    }
  })
  return hardest ? { name: hardest.name || '—', player: hardestPlayer?.name } : null
})

const countryStats = computed(() => {
  const counts = {}
  store.players.forEach(p => {
    const country = p.nationality
    if (country) {
      const key = country.toLowerCase().trim().replace(/\s+/g, '-')
      if (!counts[key]) counts[key] = { name: country, count: 0 }
      counts[key].count++
    }
  })
  return Object.values(counts)
    .map(c => {
      const code = resolveCountry(c.name)
      return { ...c, code, displayName: code ? (CODE_TO_NAME[code] || code) : 'Unknown' }
    })
    .sort((a, b) => b.count - a.count)
})

const displayedLevels = computed(() => getFilteredLevels())

onMounted(async () => {
  initTheme()
  await refreshCsrfToken()
  loadLeaderboard()
})

function retryLoad() {
  loadLeaderboard()
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
      <button class="btn btn-secondary btn-lg" @click="openInfoModal">ℹ️ Информация</button>
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
            <button class="btn btn-primary" @click="showAddPlayerModal">➕ Добавить игрока</button>
          </div>
        </div>

        <div class="demonlist-tabs">
          <div class="demonlist-tab-header">
            <button class="demonlist-tab-btn" :class="{ active: activeTab === 'players' }" @click="activeTab = 'players'">🏆 Топ игроков</button>
            <button class="demonlist-tab-btn" :class="{ active: activeTab === 'levels' }" @click="activeTab = 'levels'">📔 Топ уровней</button>
          </div>

          <div v-show="activeTab === 'players'">
            <div class="leaderboard-section">
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
                  <div class="cell cell-points">Очки</div>
                  <div class="cell cell-records">Hardest</div>
                </div>
                <div v-for="(p, index) in store.players" :key="p.id ?? index" class="player-row" @click="showProfile(index)">
                  <div class="cell cell-position" :class="index === 0 ? 'rank-1' : index === 1 ? 'rank-2' : index === 2 ? 'rank-3' : 'rank-other'">
                    {{ index + 1 }}
                  </div>
                  <div class="cell cell-player">
                    <span class="player-flag" v-html="getFlagHTML(p.nationality)"></span>
                    <div class="player-info">
                      <span class="player-name">{{ p.name }}</span>
                      <span class="player-score">{{ (p.score || 0).toFixed(2) }} pts · #{{ p.rank || '—' }}</span>
                    </div>
                    <button v-if="store.isHost" class="btn btn-danger btn-xs player-delete-btn" @click.stop="removePlayer(p.name)">✕</button>
                  </div>
                  <div class="cell cell-points">{{ (p.score || 0).toFixed(2) }}</div>
                  <div class="cell cell-records">{{ p.hardest?.level?.name || '—' }}</div>
                </div>
                <div v-if="store.players.length === 0" class="empty-state">
                  <div class="empty-state-icon">🏆</div>
                  <p>Игроки не найдены</p>
                </div>
              </div>
            </div>
          </div>

        <div v-show="activeTab === 'levels'">
          <div class="leaderboard-section">
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
              <div v-for="(level, index) in displayedLevels" :key="level.id" class="player-row" @click="showLevelVictors(level.id)">
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
        </div>
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
              <div class="stat-value">{{ totalPoints.toFixed(2) }}</div>
              <div class="stat-label">Сумма очков</div>
            </div>
            <div class="stat-card">
              <div class="stat-value" style="font-size: var(--font-size-sm);"
                :title="hardestLevelInfo ? `${hardestLevelInfo.name} — ${hardestLevelInfo.player}` : ''">
                {{ hardestLevelInfo?.name || '—' }}
              </div>
              <div class="stat-label">Сложнейший уровень</div>
            </div>
          </div>
        </div>
        <div class="stats-section">
          <h3>🌍 По странам</h3>
          <div class="country-list" id="countryList">
            <div v-for="c in countryStats" :key="c.code" class="country-item" style="cursor:pointer" @click="showCountryTop(c.name)">
              <div class="country-info">
                <span class="country-flag" v-html="getFlagHTML(c.name)"></span>
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
    <div id="profileModal" class="modal-overlay" @click.self="closeProfileModal">
      <div class="modal" @click.stop>
        <div class="modal-header">
          <div class="modal-title" id="profileTitle">Профиль</div>
          <button class="modal-close" @click="closeProfileModal">✕</button>
        </div>
        <div class="modal-body" id="profileBody"></div>
      </div>
    </div>

    <div id="countryModal" class="modal-overlay" @click.self="closeCountryModal">
      <div class="modal" @click.stop>
        <div class="modal-header">
          <div class="modal-title" id="countryTitle">Топ страны</div>
          <button class="modal-close" @click="closeCountryModal">✕</button>
        </div>
        <div class="modal-body" id="countryBody"></div>
      </div>
    </div>

    <div id="levelModal" class="modal-overlay" @click.self="closeLevelModal">
      <div class="modal" @click.stop>
        <div class="modal-header">
          <div class="modal-title" id="levelTitle">Викторы уровня</div>
          <button class="modal-close" @click="closeLevelModal">✕</button>
        </div>
        <div class="modal-body" id="levelBody"></div>
      </div>
    </div>

    <div id="addPlayerModal" class="modal-overlay">
      <div class="modal" @click.stop>
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
            <button class="btn btn-secondary" @click="closeAddPlayerModal">Отмена</button>
            <button class="btn btn-primary" @click="addPlayer">Добавить</button>
          </div>
        </div>
      </div>
    </div>
  </Teleport>
</template>
