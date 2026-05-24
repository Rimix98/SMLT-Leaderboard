/* ============================================
   SMLT Website JavaScript
   ============================================ */

// ============================================
// БЕЗОПАСНОСТЬ: Экранирование HTML
// ============================================

function escapeHtml(text) {
    if (!text) return '';
    return String(text)
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#x27;')
        .replace(/\//g, '&#x2F;')
        .replace(/`/g, '&#x60;');
}

function resolveCountry(input) {
    if (!input) return null;
    const upper = input.toUpperCase().trim();
    if (FLAGS[upper]) return upper;
    const lower = input.toLowerCase().trim().replace(/\s+/g, '-');
    return COUNTRY_TO_CODE[lower] || null;
}

function getRoleColor(roleName) {
    if (!store.staffRoles) return null;
    const role = store.staffRoles.find(r => r.name === roleName);
    return role ? role.color : null;
}

function createSafeRoleSpan(roleText) {
    const span = document.createElement('span');
    span.className = 'role';
    const color = getRoleColor(roleText);
    if (color) span.style.color = color;
    span.textContent = roleText;
    return span;
}

// ============================================
// КОНСТАНТЫ И КОНФИГУРАЦИЯ
// ============================================

const API_BASE = 'https://api.demonlist.org';
const BACKEND_URL = '/api';

const pendingRequests = new Map();
let captchaId = '';

function fetchWithAbort(url, options = {}, key = null) {
    if (key && pendingRequests.has(key)) {
        pendingRequests.get(key).abort();
    }
    const controller = new AbortController();
    if (key) pendingRequests.set(key, controller);

    const headers = {
        ...options.headers,
        'X-Requested-With': 'XMLHttpRequest'
    };

    if (['POST', 'PUT', 'DELETE', 'PATCH'].includes(options.method?.toUpperCase()) && csrfToken) {
        headers['X-CSRF-Token'] = csrfToken;
    }

    const timeoutMs = options.timeout || 30000;
    const timeoutId = setTimeout(() => controller.abort(), timeoutMs);

    return fetch(url, { ...options, headers, signal: controller.signal }).finally(() => {
        clearTimeout(timeoutId);
        if (key && pendingRequests.get(key) === controller) {
            pendingRequests.delete(key);
        }
    });
}

function isAbortError(err) {
    return err?.name === 'AbortError';
}

async function parseJsonResponse(res) {
    const contentType = res.headers.get('content-type') || '';
    const text = await res.text();
    if (!text) {
        return {};
    }
    if (!contentType.includes('application/json')) {
        if (text.trimStart().startsWith('<')) {
            throw new Error('API недоступен (ошибка сервера). Проверьте переменные окружения на Vercel.');
        }
        throw new Error('Сервер вернул некорректный ответ');
    }
    try {
        return JSON.parse(text);
    } catch (e) {
        console.error('Ошибка парсинга JSON:', e, text.slice(0, 200));
        throw new Error('Сервер вернул некорректный ответ');
    }
}

// Флаги стран (эмодзи)
const FLAGS = {
    'RU': '🇷🇺', 'US': '🇺🇸', 'DE': '🇩🇪', 'FR': '🇫🇷', 'GB': '🇬🇧',
    'BR': '🇧🇷', 'KR': '🇰🇷', 'JP': '🇯🇵', 'CN': '🇨🇳', 'PL': '🇵🇱',
    'UA': '🇺🇦', 'CA': '🇨🇦', 'AU': '🇦🇺', 'ES': '🇪🇸', 'IT': '🇮🇹',
    'AR': '🇦🇷', 'CL': '🇨🇱', 'MX': '🇲🇽', 'NL': '🇳🇱', 'SE': '🇸🇪',
    'NO': '🇳🇴', 'FI': '🇫🇮', 'DK': '🇩🇰', 'BE': '🇧🇪', 'AT': '🇦🇹',
    'CZ': '🇨🇿', 'SK': '🇸🇰', 'HU': '🇭🇺', 'RO': '🇷🇴', 'BG': '🇧🇬',
    'TR': '🇹🇷', 'IL': '🇮🇱', 'SA': '🇸🇦', 'AE': '🇦🇪', 'IN': '🇮🇳',
    'ID': '🇮🇩', 'TH': '🇹🇭', 'VN': '🇻🇳', 'MY': '🇲🇾', 'SG': '🇸🇬',
    'PH': '🇵🇭', 'NZ': '🇳🇿', 'ZA': '🇿🇦', 'EG': '🇪🇬', 'NG': '🇳🇬',
    'CO': '🇨🇴', 'PE': '🇵🇪', 'VE': '🇻🇪', 'EC': '🇪🇨', 'PT': '🇵🇹',
    'GR': '🇬🇷', 'HR': '🇭🇷', 'RS': '🇷🇸', 'SI': '🇸🇮', 'EE': '🇪🇪',
    'LV': '🇱🇻', 'LT': '🇱🇹', 'BY': '🇧🇾', 'KZ': '🇰🇿', 'UZ': '🇺🇿',
    'TW': '🇹🇼', 'HK': '🇭🇰', 'MO': '🇲🇴', 'AM': '🇦🇲', 'MD': '🇲🇩'
};

const COUNTRY_TO_CODE = {
    'russia': 'RU', 'united-states': 'US', 'germany': 'DE', 'france': 'FR',
    'united-kingdom': 'GB', 'brazil': 'BR', 'south-korea': 'KR', 'korea': 'KR',
    'japan': 'JP', 'china': 'CN', 'poland': 'PL', 'ukraine': 'UA',
    'canada': 'CA', 'australia': 'AU', 'spain': 'ES', 'italy': 'IT',
    'argentina': 'AR', 'chile': 'CL', 'mexico': 'MX', 'netherlands': 'NL',
    'sweden': 'SE', 'norway': 'NO', 'finland': 'FI', 'denmark': 'DK',
    'belgium': 'BE', 'austria': 'AT', 'czech-republic': 'CZ', 'czechia': 'CZ',
    'slovakia': 'SK', 'hungary': 'HU', 'romania': 'RO', 'bulgaria': 'BG',
    'turkey': 'TR', 'israel': 'IL', 'saudi-arabia': 'SA', 'united-arab-emirates': 'AE',
    'india': 'IN', 'indonesia': 'ID', 'thailand': 'TH', 'vietnam': 'VN',
    'malaysia': 'MY', 'singapore': 'SG', 'philippines': 'PH', 'new-zealand': 'NZ',
    'south-africa': 'ZA', 'egypt': 'EG', 'nigeria': 'NG', 'colombia': 'CO',
    'peru': 'PE', 'venezuela': 'VE', 'ecuador': 'EC', 'portugal': 'PT',
    'greece': 'GR', 'croatia': 'HR', 'serbia': 'RS', 'slovenia': 'SI',
    'estonia': 'EE', 'latvia': 'LV', 'lithuania': 'LT', 'belarus': 'BY',
    'kazakhstan': 'KZ', 'uzbekistan': 'UZ', 'taiwan': 'TW', 'hong-kong': 'HK',
    'macau': 'MO', 'armenia': 'AM', 'moldova': 'MD'
};

const CODE_TO_NAME = {
    'RU': 'Россия', 'US': 'США', 'DE': 'Германия', 'FR': 'Франция',
    'GB': 'Великобритания', 'BR': 'Бразилия', 'KR': 'Южная Корея',
    'JP': 'Япония', 'CN': 'Китай', 'PL': 'Польша', 'UA': 'Украина',
    'CA': 'Канада', 'AU': 'Австралия', 'ES': 'Испания', 'IT': 'Италия',
    'AR': 'Аргентина', 'CL': 'Чили', 'MX': 'Мексика', 'NL': 'Нидерланды',
    'SE': 'Швеция', 'NO': 'Норвегия', 'FI': 'Финляндия', 'DK': 'Дания',
    'BE': 'Бельгия', 'AT': 'Австрия', 'CZ': 'Чехия', 'SK': 'Словакия',
    'HU': 'Венгрия', 'RO': 'Румыния', 'BG': 'Болгария', 'TR': 'Турция',
    'IL': 'Израиль', 'SA': 'Саудовская Аравия', 'AE': 'ОАЭ', 'IN': 'Индия',
    'ID': 'Индонезия', 'TH': 'Таиланд', 'VN': 'Вьетнам', 'MY': 'Малайзия',
    'SG': 'Сингапур', 'PH': 'Филиппины', 'NZ': 'Новая Зеландия',
    'ZA': 'ЮАР', 'EG': 'Египет', 'NG': 'Нигерия', 'CO': 'Колумбия',
    'PE': 'Перу', 'VE': 'Венесуэла', 'EC': 'Эквадор', 'PT': 'Португалия',
    'GR': 'Греция', 'HR': 'Хорватия', 'RS': 'Сербия', 'SI': 'Словения',
    'EE': 'Эстония', 'LV': 'Латвия', 'LT': 'Литва', 'BY': 'Беларусь',
    'KZ': 'Казахстан', 'UZ': 'Узбекистан', 'TW': 'Тайвань',
    'HK': 'Гонконг', 'MO': 'Макао', 'AM': 'Армения', 'MD': 'Молдова'
};

// ============================================
// DOM helpers + состояние (модульный стиль без глобальных let)
// ============================================

/**
 * Создаёт элемент без innerHTML для пользовательских данных.
 * @param {string} tag
 * @param {{ className?: string, attrs?: Record<string,string|boolean>, style?: Partial<CSSStyleDeclaration>, dataset?: Record<string,string>, title?: string }} [opts]
 * @param {(Node|string|null|false)[]} [children]
 */
function h(tag, opts = {}, children = []) {
    const node = document.createElement(tag);
    if (opts.className) node.className = opts.className;
    if (opts.style) Object.assign(node.style, opts.style);
    if (opts.dataset) Object.assign(node.dataset, opts.dataset);
    if (opts.title != null) node.title = opts.title;
    if (opts.attrs) {
        for (const [k, v] of Object.entries(opts.attrs)) {
            if (v === false || v == null) continue;
            if (!k.startsWith('on')) {
                node.setAttribute(k, String(v));
            }
        }
    }
    for (const child of children) {
        if (child == null || child === false) continue;
        node.append(typeof child === 'string' ? document.createTextNode(child) : child);
    }
    return node;
}

function clearEl(el) {
    while (el.firstChild) el.firstChild.remove();
}

const store = {
    isHost: false,
    players: [],
    allPlayers: [],
    projects: [],
    levels: {
        all: null,
        levelData: null,
        expanded: false,
        filter: '',
        _body: null,
    },
    _leaderboard: { body: null, lastSig: '' },
    staffRoles: [],
    staffTiers: [],
    selectedRoleColor: '#3b82f6',
    pendingProjectParticipants: [],
};

function encodeCountryToken(country) {
    const code = resolveCountry(country);
    if (!code) return '';
    return btoa(encodeURIComponent(code));
}

function decodeCountryToken(token) {
    try {
        return decodeURIComponent(atob(token));
    } catch {
        return '';
    }
}

// ============================================
// CSRF ЗАЩИТА
// ============================================

let csrfToken = '';

async function refreshCsrfToken() {
    for (let attempt = 0; attempt < 2; attempt++) {
        try {
            const res = await fetch(`${BACKEND_URL}/csrf-token`, { credentials: 'include' });
            const data = await res.json();
            if (data.token) {
                csrfToken = data.token;
            }
            return data.token || null;
        } catch (e) {
            if (attempt === 0) {
                await new Promise(r => setTimeout(r, 1000));
                continue;
            }
            console.error('Не удалось получить CSRF токен:', e);
            return null;
        }
    }
    return null;
}

// ============================================
// ИНИЦИАЛИЗАЦИЯ
// ============================================

document.addEventListener('DOMContentLoaded', async () => {
    initTheme();
    initHostStatus();
    initEventListeners();
    mountDelegatedClicks();

    await refreshCsrfToken();

    if (document.getElementById('leaderboardTable')) {
        loadAllPlayers();
    }
    if (document.querySelector('.demonlist-tabs')) {
        initDemonlistTabs();
    }
    if (document.getElementById('projectsGrid')) {
        loadProjects();
    }
    if (document.getElementById('staffRolesContainer')) {
        initStaffPage();
    }
});

function mountDelegatedClicks() {
    document.getElementById('leaderboardTable')?.addEventListener('click', (e) => {
        const row = e.target.closest('[data-profile-index]');
        if (row) showProfile(Number(row.dataset.profileIndex));
    });
    document.getElementById('countryList')?.addEventListener('click', (e) => {
        const item = e.target.closest('[data-country-token]');
        if (item) {
            const country = decodeCountryToken(item.dataset.countryToken);
            if (country) showCountryTop(country);
        }
    });
    document.getElementById('levelsTable')?.addEventListener('click', (e) => {
        const row = e.target.closest('[data-level-id]');
        if (row) showLevelVictors(row.dataset.levelId);
    });
    document.getElementById('projectsGrid')?.addEventListener('click', (e) => {
        const editBtn = e.target.closest('[data-action="edit-project"]');
        const delBtn = e.target.closest('[data-action="delete-project"]');
        if (editBtn) editProject(Number(editBtn.dataset.projectIndex));
        else if (delBtn) deleteProject(Number(delBtn.dataset.projectIndex));
    });

    document.addEventListener('click', (e) => {
        const actionEl = e.target.closest('[data-action]');
        if (!actionEl) return;
        const action = actionEl.dataset.action;

        if (action === 'stop-propagation') {
            e.stopPropagation();
            return;
        }

        const handlers = {
            'close-host-modal': closeHostModal,
            'verify-host': () => verifyHost(document.getElementById('hostPassword').value),
            'close-project-modal': closeProjectModal,
            'save-project': saveProject,
            'close-add-player-modal': closeAddPlayerModal,
            'close-country-modal': closeCountryModal,
            'close-level-modal': closeLevelModal,
            'close-profile-modal': closeProfileModal,
            'close-info-modal': closeInfoModal,
            'show-add-player-modal': showAddPlayerModal,
            'show-add-project-modal': showAddProjectModal,
            'add-player': addPlayer,
            'add-project-participant': addProjectParticipant,
            'toggle-role-tag': (e) => {
                const btn = e.target.closest('[data-action="toggle-role-tag"]');
                if (btn) {
                    e.preventDefault();
                    btn.classList.toggle('active');
                    const color = btn.dataset.color || 'var(--color-secondary)';
                    if (btn.classList.contains('active')) {
                        btn.style.background = color;
                        btn.style.borderColor = color;
                        btn.style.color = '#fff';
                    } else {
                        btn.style.background = '';
                        btn.style.borderColor = color;
                        btn.style.color = color;
                    }
                }
            },
            'remove-player': () => {
                const btn = e.target.closest('[data-remove-player]');
                if (btn) removePlayer(btn.dataset.playerName);
            },
            'edit-project': () => {
                const btn = e.target.closest('[data-edit-project]');
                if (btn) editProject(Number(btn.dataset.projectIndex));
            },
            'delete-project': () => {
                const btn = e.target.closest('[data-delete-project]');
                if (btn) deleteProject(Number(btn.dataset.projectIndex));
            },
            'remove-project-participant': () => {
                const btn = e.target.closest('[data-action="remove-project-participant"]');
                if (btn) removeProjectParticipant(Number(btn.dataset.index));
            },
            'show-add-role-modal': showAddRoleModal,
            'show-edit-role-modal': () => {
                const btn = e.target.closest('[data-action="show-edit-role-modal"]');
                if (btn) showEditRoleModal(Number(btn.dataset.roleIndex));
            },
            'close-add-role-modal': closeAddRoleModal,
            'create-role': createRole,
            'close-add-player-modal-staff': closeAddStaffPlayerModal,
            'add-player-to-role': addPlayerToRole,
            'show-add-staff-player-modal': () => {
                const btn = e.target.closest('[data-action="show-add-staff-player-modal"]');
                if (btn) showAddStaffPlayerModal(Number(btn.dataset.roleIndex));
            },
            'delete-role': () => {
                const btn = e.target.closest('[data-action="delete-role"]');
                if (btn) deleteRole(Number(btn.dataset.roleIndex));
            },
            'remove-staff-player': () => {
                const btn = e.target.closest('[data-action="remove-staff-player"]');
                if (btn) removeStaffPlayer(Number(btn.dataset.roleIndex), Number(btn.dataset.playerIndex));
            },
            'move-role': () => {
                const btn = e.target.closest('[data-action="move-role"]');
                if (btn) moveRole(Number(btn.dataset.index), btn.dataset.direction);
            },
            'move-project': () => {
                const btn = e.target.closest('[data-action="move-project"]');
                if (btn) moveProject(Number(btn.dataset.index), btn.dataset.direction);
            },
            'open-edit-panel': openEditPanel,
            'close-edit-panel': closeEditPanel,
            'edit-add-player': editAddPlayer,
            'edit-set-player-tier': () => {
                const badge = e.target.closest('[data-action="edit-set-player-tier"]');
                if (badge) setPlayerTier(badge.dataset.nickname);
            },
            'edit-remove-player': () => {
                const btn = e.target.closest('[data-action="edit-remove-player"]');
                if (btn) {
                    const roleIndex = Number(btn.dataset.roleIndex);
                    const nickname = btn.dataset.nickname;
                    const role = store.staffRoles[roleIndex];
                    if (role) {
                        const pIdx = role.players.findIndex(p => p.nickname === nickname);
                        if (pIdx >= 0) removeStaffPlayer(roleIndex, pIdx);
                    }
                }
            },
            'edit-player-from-list': () => {
                const btn = e.target.closest('[data-action="edit-player-from-list"]');
                if (btn) editPlayerFromList(Number(btn.dataset.roleIndex), Number(btn.dataset.playerIndex));
            },
            'edit-save-player': editAddPlayer,
            'role-add-player': addPlayerFromRoleModal,
            'role-modal-edit-player': () => {
                const btn = e.target.closest('[data-action="role-modal-edit-player"]');
                if (btn) roleModalEditPlayer(Number(btn.dataset.roleIndex), Number(btn.dataset.playerIndex));
            },
            'role-modal-save-player': roleModalSavePlayer,
            'role-modal-move-player': () => {
                const btn = e.target.closest('[data-action="role-modal-move-player"]');
                if (btn) roleModalMovePlayer(Number(btn.dataset.roleIndex), Number(btn.dataset.playerIndex), btn.dataset.direction);
            },
            'role-modal-set-tier-direct': () => {
                const el = e.target.closest('[data-action="role-modal-set-tier-direct"]');
                if (el) setPlayerTierDirect(el.dataset.nickname, el.dataset.tier);
            },
            'role-modal-sort-tiers': () => {
                const roleIndex = parseInt(document.getElementById('editRoleIndex')?.value || '-1');
                if (roleIndex >= 0) roleModalSortByTiers(roleIndex);
            },
            'role-modal-toggle-tiers': () => {
                const roleIndex = parseInt(document.getElementById('editRoleIndex')?.value || '-1');
                if (roleIndex >= 0) roleModalToggleTiers(roleIndex);
            },
            'role-modal-remove-player': () => {
                const btn = e.target.closest('[data-action="role-modal-remove-player"]');
                if (btn) {
                    const roleIndex = Number(btn.dataset.roleIndex);
                    const nickname = btn.dataset.nickname;
                    const role = store.staffRoles[roleIndex];
                    if (role) {
                        const pIdx = role.players.findIndex(p => p.nickname === nickname);
                        if (pIdx >= 0) {
                            const editIndex = parseInt(document.getElementById('editRoleIndex')?.value || '-1');
                            removeStaffPlayer(roleIndex, pIdx).then(() => {
                                if (editIndex >= 0) renderRoleModalPlayerList(editIndex);
                            });
                        }
                    }
                }
            }
        };

        if (handlers[action]) {
            handlers[action](e);
        }
    });
}

function initDemonlistTabs() {
    const header = document.querySelector('.demonlist-tab-header');
    if (!header) return;
    header.addEventListener('click', (e) => {
        const btn = e.target.closest('.demonlist-tab-btn');
        if (!btn) return;

        document.querySelectorAll('.demonlist-tab-btn').forEach(b => b.classList.remove('active'));
        document.querySelectorAll('.demonlist-tab-content').forEach(c => c.classList.remove('active'));

        btn.classList.add('active');
        const tab = btn.dataset.tab;
        const content = document.getElementById(tab === 'players' ? 'tabPlayers' : 'tabLevels');
        if (content) content.classList.add('active');

        const adminPanel = document.querySelector('.admin-panel');
        if (adminPanel) {
            adminPanel.style.display = tab === 'players' && store.isHost ? '' : 'none';
        }
    });
}

function initEventListeners() {
    const themeToggle = document.getElementById('themeToggle');
    if (themeToggle) {
        themeToggle.addEventListener('click', toggleTheme);
    }

    const hostBtn = document.getElementById('hostBtn');
    if (hostBtn) {
        hostBtn.addEventListener('click', () => {
            if (store.isHost) {
                logoutHost();
            } else {
                showHostModal();
            }
        });
    }

    document.querySelectorAll('.modal-overlay').forEach(overlay => {
        overlay.addEventListener('click', (e) => {
            if (e.target === overlay) {
                overlay.classList.remove('active');
            }
        });
    });

    const searchInput = document.getElementById('searchInput');
    if (searchInput) {
        searchInput.addEventListener('input', (e) => {
            filterPlayers(e.target.value);
        });
    }

    const levelSearchInput = document.getElementById('levelSearchInput');
    if (levelSearchInput) {
        levelSearchInput.addEventListener('input', (e) => {
            filterLevels(e.target.value);
        });
    }

    const hostPassword = document.getElementById('hostPassword');
    if (hostPassword) {
        hostPassword.addEventListener('keypress', (e) => {
            if (e.key === 'Enter') {
                verifyHost(hostPassword.value);
            }
        });
    }

    const captchaInput = document.getElementById('captchaInput');
    if (captchaInput) {
        captchaInput.addEventListener('keypress', (e) => {
            if (e.key === 'Enter') {
                verifyHost(document.getElementById('hostPassword')?.value || '');
            }
        });
    }

    const hostSubmitBtn = document.getElementById('hostSubmitBtn');
    if (hostSubmitBtn) {
        hostSubmitBtn.addEventListener('click', () => {
            verifyHost(document.getElementById('hostPassword')?.value || '');
        });
    }

    const captchaRefreshBtn = document.getElementById('captchaRefreshBtn');
    if (captchaRefreshBtn) {
        captchaRefreshBtn.addEventListener('click', initCaptcha);
    }

    const infoBtn = document.getElementById('infoBtn');
    if (infoBtn) {
        infoBtn.addEventListener('click', showInfoModal);
    }

    document.querySelectorAll('.modal-close').forEach(btn => {
        btn.addEventListener('click', () => {
            const modal = btn.closest('.modal-overlay');
            if (modal) modal.classList.remove('active');
        });
    });
}

// ============================================
// ТЕМА
// ============================================

function initTheme() {
    const savedTheme = localStorage.getItem('smlt-theme') || 'dark';
    document.documentElement.setAttribute('data-theme', savedTheme);
    updateThemeIcon(savedTheme);
}

function toggleTheme() {
    const currentTheme = document.documentElement.getAttribute('data-theme');
    const newTheme = currentTheme === 'dark' ? 'light' : 'dark';

    document.body.classList.add('theme-transitioning');

    document.documentElement.setAttribute('data-theme', newTheme);
    localStorage.setItem('smlt-theme', newTheme);
    updateThemeIcon(newTheme);

    setTimeout(() => {
        document.body.classList.remove('theme-transitioning');
    }, 400);
}

function updateThemeIcon(theme) {
    const themeIcon = document.querySelector('.theme-icon');
    if (themeIcon) {
        themeIcon.textContent = theme === 'dark' ? '🌙' : '☀️';
    }
}

// ============================================
// ХОСТ АВТОРИЗАЦИЯ
// ============================================

async function initCaptcha() {
    const img = document.getElementById('captcha-img');
    const input = document.getElementById('captchaInput');
    if (!img) return;

    try {
        const res = await fetchWithAbort(`${BACKEND_URL}/captcha`, {
            credentials: 'include'
        }, 'captcha-fetch');
        const data = await parseJsonResponse(res);
        if (!res.ok || !data.captchaId) {
            console.error('Ошибка получения капчи');
            return;
        }
        captchaId = data.captchaId;
        img.src = data.captchaImage;
        img.style.display = 'block';
        if (input) input.value = '';
    } catch (err) {
        if (isAbortError(err)) return;
        console.error('Ошибка загрузки капчи:', err);
    }
}

async function initHostStatus() {
    try {
        const res = await fetchWithAbort(`${BACKEND_URL}/verify`, {
            credentials: 'include'
        }, 'auth-verify');
        const data = await parseJsonResponse(res);
        store.isHost = res.ok && data.success === true;
    } catch (err) {
        if (isAbortError(err)) return;
        store.isHost = false;
    }

    updateHostButton();
    updateAdminControls();
}

function showHostModal() {
    const modal = document.getElementById('hostModal');
    const passwordInput = document.getElementById('hostPassword');
    const errorEl = document.getElementById('hostError');

    if (modal) {
        modal.classList.add('active');
        if (passwordInput) {
            passwordInput.value = '';
            passwordInput.focus();
        }
        if (errorEl) {
            errorEl.style.display = 'none';
        }
        initCaptcha();
    }
}

function closeHostModal() {
    const modal = document.getElementById('hostModal');
    if (modal) {
        modal.classList.remove('active');
    }
}

async function verifyHost(inputPassword) {
    const captchaInput = document.getElementById('captchaInput');

    try {
        const res = await fetchWithAbort(`${BACKEND_URL}/login`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'include',
            body: JSON.stringify({
                password: inputPassword,
                captchaId: captchaId,
                captchaValue: captchaInput ? captchaInput.value : ''
            })
        }, 'host-login');

        const data = await parseJsonResponse(res);

        if (res.ok && data.success === true) {
            store.isHost = true;

            showToast('Доступ предоставлен! Вы вошли как хост.', 'success');

            const modal = document.getElementById('hostModal');
            if (modal) modal.classList.remove('active');

            updateHostButton();
            updateAdminControls();

            if (document.getElementById('projectsGrid')) loadProjects();
            if (document.getElementById('leaderboardTable')) loadAllPlayers();
        } else {
            const errorMsg = data.error || 'Неверный пароль хоста!';
            showToast(errorMsg, 'error');
            store.isHost = false;
            initCaptcha();
        }
    } catch (err) {
        if (isAbortError(err)) return;
        console.error('Ошибка входа:', err);
        showToast(err.message === 'Сервер вернул некорректный ответ'
            ? 'Ошибка сервера: некорректный формат данных'
            : 'Ошибка соединения с сервером. Проверьте сеть или статус сервера.', 'error');
        initCaptcha();
    }
}
async function logoutHost() {
    store.isHost = false;

    try {
        await fetchWithAbort(`${BACKEND_URL}/logout`, {
            method: 'POST',
            credentials: 'include'
        }, 'host-logout');
    } catch (e) {
        if (!isAbortError(e)) {
            console.error("Не удалось разлогиниться на сервере", e);
        }
    }

    updateHostButton();
    updateAdminControls();
    closeEditPanel();
    showToast('Вы вышли из режима хоста', 'info');
}

function updateHostButton() {
    const hostBtn = document.getElementById('hostBtn');
    if (!hostBtn) return;
    clearEl(hostBtn);
    if (store.isHost) {
        hostBtn.classList.add('is-host');
        hostBtn.appendChild(h('span', {}, ['👑 Хост']));
    } else {
        hostBtn.classList.remove('is-host');
        hostBtn.appendChild(h('span', {}, ['Хост']));
    }
}

function updateAdminControls() {
    const adminElements = document.querySelectorAll('.admin-only');
    adminElements.forEach(el => {
        el.style.display = store.isHost ? '' : 'none';
    });

    if (document.getElementById('staffRolesContainer')) {
        renderStaffRoles();
    }
}

// ============================================
// ДЕМОНЛИСТ
// ============================================

const DEFAULT_PLAYER_NAMES = [
    "samoletik", "paradoxiz", "clokman", "itzslxnq", "H30n41k_GmD",
    "Filkoty", "DarBeast", "Florned", "Marzyiiik", "euphoriak8",
    "npoctou_gamer", "NopanicGD", "CandyCloud22", "Vakum", "Daggit",
    "Loran", "tapxyhh", "SerGio", "Fanim59", "prostoymofficial",
    "toxik blaze", "NatrixGMD", "toxatort", "SpaceRS", "yeahme",
    "Спини", "Linqwq", "RossceorpGD", "69liqu69"
];

async function getPlayerNames() {
    try {
        const res = await fetchWithAbort(`${BACKEND_URL}/players`, {}, 'players-list');
        if (!res.ok) return DEFAULT_PLAYER_NAMES;
        const data = await res.json();
        if (Array.isArray(data) && data.length > 0) {
            // Если это массив объектов { name: "..." }, извлекаем имена
            if (typeof data[0] === 'object' && data[0].name) {
                return data.map(p => p.name);
            }
            // Если это массив строк, возвращаем как есть
            return data;
        }
        return DEFAULT_PLAYER_NAMES;
    } catch {
        return DEFAULT_PLAYER_NAMES;
    }
}


async function loadGeoStats() {
    try {
        const response = await fetchWithAbort(`${API_BASE}/stats/countries`, {}, 'geo-stats');
        const stats = await response.json();

        const container = document.getElementById('geo-stats-container');
        if (!container) return;

        clearEl(container);

        const sortedStats = Object.entries(stats).sort((a, b) => b[1] - a[1]);
        const maxPlayers = sortedStats[0] ? sortedStats[0][1] : 1;

        sortedStats.forEach(([countryCode, count]) => {
            const percentage = (count / maxPlayers) * 100;
            const row = h(
                'div',
                {
                    className: 'geo-row',
                    style: { display: 'flex', alignItems: 'center', marginBottom: '8px' },
                },
                [
                    h('span', { style: { width: '50px', display: 'flex', alignItems: 'center' } }, [
                        getFlag(countryCode),
                        document.createTextNode(` ${countryCode}`)
                    ]),
                    h(
                        'div',
                        {
                            style: {
                                flexGrow: '1',
                                background: '#222',
                                height: '12px',
                                borderRadius: '6px',
                                margin: '0 10px',
                                overflow: 'hidden',
                            },
                        },
                        [
                            h('div', {
                                style: {
                                    width: `${percentage}%`,
                                    background: '#00bcd4',
                                    height: '100%',
                                    borderRadius: '6px',
                                },
                            }),
                        ]
                    ),
                    h('span', { style: { width: '30px', textAlign: 'right' } }, [String(count)]),
                ]
            );
            container.appendChild(row);
        });
    } catch (e) {
        if (isAbortError(e)) return;
        showToast('Не удалось загрузить гео-статистику', 'error');
    }
}

async function savePlayerNames(names) {
    const formattedPlayers = names.map(n => ({ name: n }));

    const res = await fetchWithAbort(`${BACKEND_URL}/players/save`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify(formattedPlayers)
    }, 'players-save');
    if (!res.ok) {
        const err = await res.json().catch(() => ({}));
        throw new Error(err.error || 'Ошибка сохранения игроков (возможно, сессия истекла)');
    }
}


function getFlag(c) {
    const code = resolveCountry(c);
    if (!code) {
        return h('span', { className: 'flag-emoji' }, [c === null ? '❌' : '🌍']);
    }
    return h('img', {
        className: 'flag-img',
        attrs: {
            src: `https://flagcdn.com/w20/${code.toLowerCase()}.png`,
            alt: code,
            width: 20
        },
        style: {
            verticalAlign: 'middle',
            borderRadius: '2px',
            marginRight: '4px'
        }
    });
}

function getCountryLabel(c) {
    if (!c) return 'неизвестно';
    const upper = c.toUpperCase();
    if (FLAGS[upper]) return upper;
    const lower = c.toLowerCase().trim().replace(/\s+/g, '-');
    const code = COUNTRY_TO_CODE[lower];
    if (code) return CODE_TO_NAME[code] || code;
    return c;
}

function showToast(msg, type = 'error') {
    const t = document.createElement('div');
    t.className = `toast toast-${type}`;
    t.textContent = msg;
    const container = document.getElementById('toastContainer');
    if (container) container.appendChild(t);
    setTimeout(() => t.remove(), 5000);
}

function updateProgress(current, total) {
    const progressFill = document.getElementById('progressFill');
    const loadingText = document.getElementById('loadingText');
    if (progressFill) progressFill.style.width = Math.round((current / total) * 100) + '%';
    if (loadingText) loadingText.textContent = `Загрузка ${current}/${total} игроков...`;
}

async function fetchPlayerData(name) {
    try {
        const r = await fetch(`${API_BASE}/leaderboard/user/list?search=${encodeURIComponent(name)}&limit=50`);
        if (!r.ok) return null;
        const d = await r.json();
        if (d.message !== 'success' || !d.data?.users?.length) return null;

        const nl = name.toLowerCase().trim();
        const users = d.data.users;

        let fp = users.find(p => p.username?.toLowerCase().trim() === nl);
        if (!fp && !isNaN(parseInt(name))) {
            fp = users.find(p => p.id.toString() === name.trim());
        }

        if (!fp) return null;
        return fp;
    } catch (e) {
        console.error(`Ошибка для "${name}":`, e);
        return null;
    }
}

async function fetchRecords(id) {
    try {
        const r = await fetch(`${API_BASE}/user/record/list?user_id=${id}&limit=50`);
        if (!r.ok) return [];
        const d = await r.json();
        return d.message === 'success' && d.data?.records ? d.data.records : [];
    } catch {
        return [];
    }
}

function mapLeaderboardEntry(p) {
    const nl = (p.name || '').toLowerCase().trim();
    
    // Извлекаем данные пользователя из вложенной структуры
    let userData = null;
    if (p.data && p.data.data && Array.isArray(p.data.data.users) && p.data.data.users.length > 0) {
        userData = p.data.data.users[0];
    } else if (p.data && Array.isArray(p.data.users) && p.data.users.length > 0) {
        userData = p.data.users[0];
    }

    // Извлекаем records
    let pRecs = [];
    if (p.records && p.records.data && Array.isArray(p.records.data.records)) {
        pRecs = p.records.data.records;
    } else if (p.records && Array.isArray(p.records.records)) {
        pRecs = p.records.records;
    }

    let hardest = null;
    const acceptedRecs = pRecs.filter(r => r.status === 'accepted' && r.level);
    if (acceptedRecs.length > 0) {
        hardest = acceptedRecs.reduce((m, r) => (!m || r.level.placement < m.level.placement) ? r : m);
    }

    return {
        id: userData?.id || p.id,
        name: userData?.username || p.name,
        rank: userData?.placement || 0,
        score: parseFloat(userData?.points) || 0,
        nationality: userData?.country || null,
        records: pRecs,
        hardest
    };
}

function hasLeaderboardData(resData) {
    return Array.isArray(resData);
}

let _loadingLeaderboard = false;

async function loadAllPlayers() {
    if (_loadingLeaderboard) return;
    _loadingLeaderboard = true;

    const table = document.getElementById('leaderboardTable');
    const count = document.getElementById('playersCount');
    if (!table) {
        _loadingLeaderboard = false;
        return;
    }

    try {
        let playersToMap = [];
        const res = await fetchWithAbort('/api/leaderboard', {}, 'leaderboard');

        if (res.ok) {
            const responseData = await parseJsonResponse(res);
            if (hasLeaderboardData(responseData)) {
                playersToMap = responseData;
            }
        }

        if (playersToMap.length === 0) {
            await loadPlayersFromClientAPI();
            return;
        }

        const loaded = playersToMap.map(mapLeaderboardEntry).filter(p => p.id);
        if (loaded.length === 0) {
            await loadPlayersFromClientAPI();
            return;
        }

        store.players = loaded.sort((a, b) => (a.rank || 999999) - (b.rank || 999999));
        store.allPlayers = [...store.players];
        renderPlayers();
        renderHardestLevels();

    } catch (e) {
        if (isAbortError(e)) return;
        try {
            await loadPlayersFromClientAPI();
        } catch (err) {
            if (isAbortError(err)) return;
            clearEl(table);
            table.appendChild(
                h('div', { className: 'empty-state' }, [h('p', {}, ['Не удалось загрузить данные'])])
            );
            showToast('Ошибка загрузки лидерборда', 'error');
        }
    } finally {
        _loadingLeaderboard = false;
    }
}

async function loadPlayersFromClientAPI() {
    const table = document.getElementById('leaderboardTable');
    const names = await getPlayerNames();

    const promises = names.map(async (name) => {
        try {
            const fp = await fetchPlayerData(name);
            if (!fp) return null;
            const recs = await fetchRecords(fp.id);

            let hardest = null;
            const acceptedRecs = recs.filter(r => r.status === 'accepted' && r.level);
            if (acceptedRecs.length > 0) {
                hardest = acceptedRecs.reduce((m, r) => (!m || r.level.placement < m.level.placement) ? r : m);
            }

            return {
                id: fp.id,
                name: fp.username || name,
                rank: fp.placement || 0,
                score: parseFloat(fp.points) || 0,
                nationality: fp.country || null,
                records: recs,
                hardest
            };
        } catch (e) {
            console.error(`Ошибка загрузки игрока ${name}:`, e);
            return null;
        }
    });

    const results = await Promise.all(promises);
    const loaded = results.filter(p => p !== null);

    if (loaded.length === 0) {
        clearEl(table);
        table.appendChild(h('div', { className: 'empty-state' }, [h('p', {}, ['Не удалось загрузить данные игроков'])]));
        return;
    }

    store.players = loaded.sort((a, b) => (a.rank || 999999) - (b.rank || 999999));
    store.allPlayers = [...store.players];
    renderPlayers();
    renderHardestLevels();
}

function filterPlayers(query) {
    if (!query) {
        store.players = [...store.allPlayers];
    } else {
        const q = query.toLowerCase().trim();
        store.players = store.allPlayers.filter(p => p.name.toLowerCase().includes(q));
    }
    renderPlayers();
}

function ensureLeaderboardShell(table) {
    let shell = table.querySelector('.js-leaderboard-shell');
    if (shell) return shell;
    clearEl(table);
    const header = h('div', { className: 'table-header' }, [
        h('div', { className: 'cell cell-position' }, ['#']),
        h('div', { className: 'cell cell-player' }, ['Игрок']),
        h('div', { className: 'cell cell-points' }, ['Очки']),
        h('div', { className: 'cell cell-records' }, ['Hardest']),
    ]);
    const body = h('div', { className: 'js-leaderboard-body' });
    store._leaderboard.body = body;
    shell = h('div', { className: 'js-leaderboard-shell' }, [header, body]);
    table.appendChild(shell);
    return shell;
}

function createPlayerRow(index, p) {
    const rc = index === 0 ? 'rank-1' : index === 1 ? 'rank-2' : index === 2 ? 'rank-3' : 'rank-other';
    const score = p.score ? p.score.toFixed(2) : '—';
    const rank = p.rank || '—';
    const hardest = p.hardest?.level?.name || '—';
    return h('div', { className: 'player-row', dataset: { profileIndex: String(index) } }, [
        h('div', { className: `cell cell-position ${rc}` }, [String(index + 1)]),
        h('div', { className: 'cell cell-player' }, [
            h('span', { className: 'player-flag' }, [
                getFlag(p.nationality),
            ]),
            h('div', { className: 'player-info' }, [
                h('span', { className: 'player-name' }, [p.name]),
                h('span', { className: 'player-score' }, [`${score} pts · #${rank}`]),
            ]),
        ]),
        h('div', { className: 'cell cell-points' }, [score]),
        h('div', { className: 'cell cell-records' }, [hardest]),
    ]);
}

function updatePlayerRow(row, index, p) {
    row.dataset.profileIndex = String(index);
    const rc = index === 0 ? 'rank-1' : index === 1 ? 'rank-2' : index === 2 ? 'rank-3' : 'rank-other';
    const score = p.score ? p.score.toFixed(2) : '—';
    const rank = p.rank || '—';
    const hardest = p.hardest?.level?.name || '—';
    const [cellPos, cellPlayer, cellPoints, cellRec] = row.children;
    cellPos.className = `cell cell-position ${rc}`;
    cellPos.textContent = String(index + 1);
    const flagSpan = cellPlayer.querySelector('.player-flag');
    flagSpan.textContent = '';
    flagSpan.append(getFlag(p.nationality));
    cellPlayer.querySelector('.player-name').textContent = p.name;
    cellPlayer.querySelector('.player-score').textContent = `${score} pts · #${rank}`;
    cellPoints.textContent = score;
    cellRec.textContent = hardest;
}

function renderPlayers() {
    const table = document.getElementById('leaderboardTable');
    const count = document.getElementById('playersCount');
    if (!table) return;

    if (store.players.length === 0) {
        store._leaderboard.body = null;
        clearEl(table);
        table.appendChild(
            h('div', { className: 'empty-state' }, [
                h('div', { className: 'empty-state-icon' }, ['🏆']),
                h('p', {}, ['Игроки не найдены']),
            ])
        );
        if (count) count.textContent = '0 игроков';
        renderStats();
        return;
    }

    ensureLeaderboardShell(table);
    const body = store._leaderboard.body;
    if (!body) return;

    if (count) count.textContent = `${store.players.length} игроков`;

    const n = store.players.length;
    let rows = [...body.children];
    while (rows.length < n) {
        body.appendChild(createPlayerRow(rows.length, store.players[rows.length]));
        rows = [...body.children];
    }
    while (rows.length > n) {
        body.lastElementChild?.remove();
        rows = [...body.children];
    }
    for (let i = 0; i < n; i++) {
        updatePlayerRow(rows[i], i, store.players[i]);
    }
    renderStats();
}

function renderStats() {
    const statPlayers = document.getElementById('statPlayers');
    const statPoints = document.getElementById('statPoints');
    const statHardest = document.getElementById('statHardest');

    if (statPlayers) statPlayers.textContent = store.players.length;

    const totalPoints = store.players.reduce((sum, p) => sum + (p.score || 0), 0);
    if (statPoints) statPoints.textContent = totalPoints.toFixed(2);

    let hardestLevel = null;
    let hardestPlayer = null;
    store.players.forEach(p => {
        if (p.hardest && p.hardest.level) {
            if (!hardestLevel || p.hardest.level.placement < hardestLevel.placement) {
                hardestLevel = p.hardest.level;
                hardestPlayer = p;
            }
        }
    });

    const hardestEl = document.getElementById('statHardest');
    if (hardestLevel) {
        const levelName = hardestLevel.name || '—';
        hardestEl.textContent = levelName;
        hardestEl.title = `${levelName} #${hardestLevel.placement} — ${hardestPlayer ? hardestPlayer.name : '—'}`;
    } else {
        hardestEl.textContent = '—';
    }

    renderCountryStats();
}

function renderCountryStats() {
    const countryList = document.getElementById('countryList');
    if (!countryList) return;

    const countryCounts = {};
    store.players.forEach(p => {
        const country = p.nationality;
        if (country) {
            const key = country.toLowerCase().trim().replace(/\s+/g, '-');
            if (!countryCounts[key]) {
                countryCounts[key] = { name: country, count: 0, members: [] };
            }
            countryCounts[key].count++;
            countryCounts[key].members.push(p);
        }
    });

    const sorted = Object.values(countryCounts).sort((a, b) => b.count - a.count);

    if (sorted.length === 0) {
        clearEl(countryList);
        countryList.appendChild(
            h('div', { style: { color: 'var(--color-text-muted)', fontSize: 'var(--font-size-sm)' } }, ['Нет данных'])
        );
        return;
    }

    clearEl(countryList);
    const frag = document.createDocumentFragment();
    sorted.forEach((c) => {
        const code = resolveCountry(c.name);
        const displayName = code ? (CODE_TO_NAME[code] || code) : 'Unknown';
        const countryName = CODE_TO_NAME[c.name.toUpperCase()] || c.name;
        const token = encodeCountryToken(c.name);
        const row = h(
            'div',
            {
                className: 'country-item',
                style: { cursor: 'pointer' },
                dataset: { countryToken: token },
                title: `Топ игроков ${countryName}`,
            },
            [
                h('div', { className: 'country-info' }, [
                    h('span', { className: 'country-flag' }, [getFlag(c.name)]),
                    h('span', { className: 'country-name' }, [displayName]),
                ]),
                h('span', { className: 'country-count' }, [String(c.count)]),
            ]
        );
        frag.appendChild(row);
    });
    countryList.appendChild(frag);
}

function showCountryTop(raw) {
    const country = resolveCountry(raw);
    if (!country) {
        showToast('Страна не найдена', 'error');
        return;
    }

    const countryPlayers = store.allPlayers.filter(p => {
        const pCountry = resolveCountry(p.nationality);
        return pCountry === country;
    }).sort((a, b) => (a.rank || 999999) - (b.rank || 999999));

    const modal = document.getElementById('countryModal');
    const title = document.getElementById('countryTitle');
    const body = document.getElementById('countryBody');

    if (!modal || !title || !body) return;

    const flagNode = getFlag(country);
    const countryName = CODE_TO_NAME[country] || country;
    
    clearEl(title);
    title.append(flagNode, ` Топ игроков: ${countryName}`);

    if (countryPlayers.length === 0) {
        clearEl(body);
        body.appendChild(
            h('p', { style: { color: 'var(--color-text-muted)' } }, ['Нет данных'])
        );
    } else {
        clearEl(body);
        const list = h('div', { className: 'country-top-list' });
        countryPlayers.forEach((p, idx) => {
            const score = p.score ? p.score.toFixed(2) : '—';
            const rank = p.rank || '—';
            list.appendChild(
                h(
                    'div',
                    {
                        className: 'country-top-item',
                        style: {
                            display: 'flex',
                            justifyContent: 'space-between',
                            padding: 'var(--spacing-sm)',
                            borderBottom: '1px solid var(--color-border)',
                        },
                    },
                    [
                        h('span', {}, [
                            h('strong', {}, [`#${idx + 1} `]),
                            document.createTextNode(p.name),
                        ]),
                        h('span', { style: { color: 'var(--color-text-muted)' } }, [`${score} pts · #${rank}`]),
                    ]
                )
            );
        });
        body.appendChild(list);
    }

    modal.classList.add('active');
}

function closeCountryModal() {
    const modal = document.getElementById('countryModal');
    if (modal) modal.classList.remove('active');
}

function renderHardestLevels() {
    const levelsTable = document.getElementById('levelsTable');
    const levelsCount = document.getElementById('levelsCount');
    const expandContainer = document.getElementById('expandLevelsContainer');

    if (!levelsTable) return;

    const levelMap = new Map();

    store.players.forEach(player => {
        if (player.records) {
            player.records.forEach(record => {
                if (record.status === 'accepted' && record.level) {
                    const levelId = record.level.id;
                    const levelName = record.level.name;
                    const placement = record.level.placement;

                    if (!levelMap.has(levelId)) {
                        levelMap.set(levelId, {
                            id: levelId,
                            name: levelName,
                            placement: placement,
                            victors: []
                        });
                    }

                    const levelData = levelMap.get(levelId);
                    if (!levelData.victors.find(v => v.id === player.id)) {
                        levelData.victors.push({
                            id: player.id,
                            name: player.name,
                            nationality: player.nationality
                        });
                    }
                }
            });
        }
    });

    const sortedLevels = Array.from(levelMap.values())
        .filter(level => level.placement !== undefined && level.placement !== null)
        .sort((a, b) => a.placement - b.placement);

    if (sortedLevels.length === 0) {
        clearEl(levelsTable);
        levelsTable.appendChild(
            h('div', { className: 'empty-state' }, [
                h('div', { className: 'empty-state-icon' }, ['🏆']),
                h('p', {}, ['Нет данных об уровнях']),
            ])
        );
        if (levelsCount) levelsCount.textContent = '0 уровней';
        if (expandContainer) expandContainer.style.display = 'none';
        store.levels.all = null;
        store.levels.levelData = null;
        return;
    }

    if (levelsCount) levelsCount.textContent = `${sortedLevels.length} уровней`;

    store.levels.all = sortedLevels;
    store.levels.expanded = false;
    store.levels.filter = '';
    store.levels.levelData = new Map();
    for (const [k, v] of levelMap) {
        store.levels.levelData.set(String(k), v);
    }

    renderLevelsList(sortedLevels.slice(0, 39));

    if (expandContainer) {
        expandContainer.style.display = sortedLevels.length > 39 ? 'block' : 'none';
    }
}

function ensureLevelsShell(table) {
    let shell = table.querySelector('.js-levels-shell');
    if (shell) return shell;
    clearEl(table);
    const header = h('div', { className: 'table-header' }, [
        h('div', { className: 'cell cell-position' }, ['#']),
        h('div', { className: 'cell cell-player' }, ['Уровень']),
        h('div', { className: 'cell cell-points' }, ['Позиция']),
        h('div', { className: 'cell cell-records' }, ['Викторов']),
    ]);
    const body = h('div', { className: 'js-levels-body' });
    store.levels._body = body;
    shell = h('div', { className: 'js-levels-shell' }, [header, body]);
    table.appendChild(shell);
    return shell;
}

function createLevelRow(index, level) {
    const rc = index === 0 ? 'rank-1' : index === 1 ? 'rank-2' : index === 2 ? 'rank-3' : 'rank-other';
    const lid = String(level.id);
    return h('div', { className: 'player-row', dataset: { levelId: lid } }, [
        h('div', { className: `cell cell-position ${rc}` }, [String(index + 1)]),
        h('div', { className: 'cell cell-player' }, [
            h('div', { className: 'player-info' }, [h('span', { className: 'player-name' }, [level.name])]),
        ]),
        h('div', { className: 'cell cell-points' }, [`#${level.placement}`]),
        h('div', { className: 'cell cell-records' }, [String(level.victors.length)]),
    ]);
}

function updateLevelRow(row, index, level) {
    const lid = String(level.id);
    row.dataset.levelId = lid;
    const rc = index === 0 ? 'rank-1' : index === 1 ? 'rank-2' : index === 2 ? 'rank-3' : 'rank-other';
    const [cellPos, cellPlayer, cellPoints, cellRec] = row.children;
    cellPos.className = `cell cell-position ${rc}`;
    cellPos.textContent = String(index + 1);
    cellPlayer.querySelector('.player-name').textContent = level.name;
    cellPoints.textContent = `#${level.placement}`;
    cellRec.textContent = String(level.victors.length);
}

function renderLevelsList(levels) {
    const levelsTable = document.getElementById('levelsTable');
    if (!levelsTable) return;

    ensureLevelsShell(levelsTable);
    const body = store.levels._body;
    if (!body) return;

    const n = levels.length;
    let rows = [...body.children];
    while (rows.length < n) {
        body.appendChild(createLevelRow(rows.length, levels[rows.length]));
        rows = [...body.children];
    }
    while (rows.length > n) {
        body.lastElementChild?.remove();
        rows = [...body.children];
    }
    for (let i = 0; i < n; i++) {
        updateLevelRow(rows[i], i, levels[i]);
    }
}

function expandLevels() {
    const expandContainer = document.getElementById('expandLevelsContainer');
    const expandButton = expandContainer?.querySelector('button');
    if (!store.levels.all) return;

    if (store.levels.expanded) {
        store.levels.expanded = false;
        renderLevelsList(store.levels.all.slice(0, 39));
        if (expandButton) expandButton.textContent = 'Показать ещё';
    } else {
        store.levels.expanded = true;
        renderLevelsList(store.levels.all);
        if (expandButton) expandButton.textContent = 'Свернуть';
    }
}

function filterLevels(query) {
    if (!store.levels.all) return;

    if (!query) {
        store.levels.expanded = false;
        renderLevelsList(store.levels.all.slice(0, 39));

        const expandContainer = document.getElementById('expandLevelsContainer');
        const expandButton = expandContainer?.querySelector('button');
        if (expandContainer) {
            expandContainer.style.display = store.levels.all.length > 39 ? 'block' : 'none';
        }
        if (expandButton) expandButton.textContent = 'Показать ещё';
        return;
    }

    const q = query.toLowerCase().trim();
    const filtered = store.levels.all.filter((level) => level.name.toLowerCase().includes(q));

    renderLevelsList(filtered);

    const expandContainer = document.getElementById('expandLevelsContainer');
    if (expandContainer) {
        expandContainer.style.display = 'none';
    }
}

function showLevelVictors(levelId) {
    const levelData = store.levels.levelData?.get(String(levelId));
    if (!levelData) return;

    const modal = document.getElementById('levelModal');
    const title = document.getElementById('levelTitle');
    const body = document.getElementById('levelBody');

    if (!modal || !title || !body) return;

    title.textContent = `🏆 ${levelData.name} #${levelData.placement}`;

    clearEl(body);
    if (levelData.victors.length === 0) {
        body.appendChild(
            h('p', { style: { color: 'var(--color-text-muted)' } }, ['Нет викторов'])
        );
    } else {
        const list = h('div', { className: 'level-victors-list' });
        levelData.victors.forEach((victor, idx) => {
            const flagNode = getFlag(victor.nationality);
            const span = h('span', {}, [
                h('strong', {}, [`#${idx + 1} `]),
                flagNode,
                document.createTextNode(` ${victor.name}`),
            ]);
            list.appendChild(
                h(
                    'div',
                    {
                        className: 'level-victor-item',
                        style: {
                            display: 'flex',
                            justifyContent: 'space-between',
                            padding: 'var(--spacing-sm)',
                            borderBottom: '1px solid var(--color-border)',
                        },
                    },
                    [span]
                )
            );
        });
        body.appendChild(list);
    }

    modal.classList.add('active');
}

function closeLevelModal() {
    const modal = document.getElementById('levelModal');
    if (modal) modal.classList.remove('active');
}

function showProfile(idx) {
    const p = store.players[idx];
    if (!p) return;

    const rec = p.records ? p.records.filter((r) => r.status === 'accepted' && r.level) : [];
    const flagNode = getFlag(p.nationality);
    const name = p.name;

    const titleEl = document.getElementById('profileTitle');
    clearEl(titleEl);
    titleEl.append(flagNode, ` ${name}`);

    const score = p.score ? p.score.toFixed(2) : '—';
    const rank = p.rank || '—';

    const body = document.getElementById('profileBody');
    clearEl(body);

    function stat(value, label) {
        return h('div', { className: 'profile-stat' }, [
            h('div', { className: 'profile-stat-value' }, [String(value)]),
            h('div', { className: 'profile-stat-label' }, [label]),
        ]);
    }

    body.appendChild(
        h('div', { className: 'profile-stats' }, [stat(score, 'Очки'), stat(`#${rank}`, 'Глобальный топ'), stat(String(rec.length), 'Уровней')])
    );

    if (p.hardest) {
        const hardestLabel = p.hardest.level?.name != null ? String(p.hardest.level.name) : String(p.hardest);
        body.appendChild(
            h('div', { className: 'profile-info-row' }, [
                h('span', { className: 'profile-info-label' }, ['Hardest:']),
                h('span', { className: 'profile-info-value' }, [hardestLabel]),
            ])
        );
    }

    body.appendChild(
        h('div', { className: 'profile-info-row' }, [
            h('span', { className: 'profile-info-label' }, ['Страна:']),
            h('span', { className: 'profile-info-value' }, [
                getFlag(p.nationality),
                document.createTextNode(` ${p.nationality || 'Не указана'}`)
            ]),
        ])
    );

    const recordsSection = h('div', { className: 'profile-records-section' }, [
        h('h4', {}, [`Пройденные уровни (${rec.length})`]),
        h('div', { className: 'profile-records-list' }, []),
    ]);
    const recordsList = recordsSection.querySelector('.profile-records-list');

    if (rec.length > 0) {
        rec.forEach((r) => {
            const levelName = r.level?.name || 'Unknown';
            const placement = r.level?.placement ?? '?';
            const progress = r.percent ?? r.progress ?? 100;
            recordsList.appendChild(
                h('div', { className: 'record-item' }, [
                    h('span', { className: 'record-demon' }, [
                        document.createTextNode(levelName),
                        h('span', { className: 'record-placement' }, [`#${placement}`]),
                    ]),
                    h('span', { className: `record-progress${progress >= 100 ? ' progress-100' : ''}` }, [`${progress}%`]),
                ])
            );
        });
    } else {
        recordsList.appendChild(h('div', { className: 'no-records' }, ['Нет записей']));
    }
    body.appendChild(recordsSection);

    const link = document.createElement('a');
    link.href = `https://demonlist.org/profile/${encodeURIComponent(String(p.id))}/`;
    link.target = '_blank';
    link.rel = 'noopener noreferrer';
    link.textContent = '🔗 Показать аккаунт в Global Demonlist →';
    body.appendChild(h('div', { className: 'profile-link' }, [link]));

    document.getElementById('profileModal').classList.add('active');
}

function closeProfileModal(e) {
    if (!e || e.target === e.currentTarget) {
        document.getElementById('profileModal').classList.remove('active');
    }
}

// ============================================
// УПРАВЛЕНИЕ ИГРОКАМИ (ХОСТ)
// ============================================

function showAddPlayerModal() {
    if (!store.isHost) {
        showToast('Только хост может добавлять игроков', 'error');
        return;
    }

    const modal = document.getElementById('addPlayerModal');
    if (modal) {
        document.getElementById('newPlayerName').value = '';
        modal.classList.add('active');
    }
}

function closeAddPlayerModal() {
    const modal = document.getElementById('addPlayerModal');
    if (modal) modal.classList.remove('active');
}

async function addPlayer() {
    const nameInput = document.getElementById('newPlayerName');
    const name = nameInput.value.trim();

    if (!name) return;

    if (name.length < 2 || name.length > 32) {
        showToast('Ник должен быть от 2 до 32 символов', 'error');
        return;
    }

    let playerNames = await getPlayerNames();
    if (playerNames.includes(name)) {
        showToast('Такой игрок уже есть', 'error');
        return;
    }

    playerNames.push(name);

    try {
        await savePlayerNames(playerNames);
        closeAddPlayerModal();
        nameInput.value = '';
        await loadAllPlayers();
        showToast('Игрок успешно добавлен', 'success');
    } catch (e) {
        if (isAbortError(e)) return;
        showToast(e.message, 'error');
        if (e.message.includes('сессия истекла') || e.message.includes('401')) {
            logoutHost();
        }
    }
}

async function removePlayer(name) {
    if (!store.isHost) {
        showToast('Только хост может удалять игроков', 'error');
        return;
    }

    if (!confirm(`Удалить игрока "${name}"?`)) return;

    try {
        const res = await fetchWithAbort(`${BACKEND_URL}/players/delete`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'include',
            body: JSON.stringify({ name })
        }, 'players-delete');
        if (!res.ok) {
            const err = await res.json().catch(() => ({}));
            throw new Error(err.error || 'Ошибка удаления игрока');
        }
        await loadAllPlayers();
        showToast(`Игрок "${name}" удалён`, 'success');
    } catch (e) {
        if (isAbortError(e)) return;
        showToast(e.message, 'error');
        if (e.message.includes('сессия') || e.message.includes('401') || e.message.includes('доступ')) {
            logoutHost();
        }
    }
}

// ============================================
// ПРОЕКТЫ
// ============================================

const DEFAULT_PROJECTS = [];

async function getProjects() {
    try {
        const res = await fetchWithAbort(`${BACKEND_URL}/projects`, {}, 'projects-list');
        if (!res.ok) return [];
        const data = await res.json();
        return Array.isArray(data) ? data : [];
    } catch {
        return [];
    }
}

async function saveProjects(data) {
    const res = await fetchWithAbort(`${BACKEND_URL}/projects/save`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify(data)
    }, 'projects-save');
    if (!res.ok) {
        const err = await res.json().catch(() => ({}));
        throw new Error(err.error || 'Ошибка сохранения (возможно, сессия истекла)');
    }
}

async function loadProjects() {
    store.projects = await getProjects();
    renderProjects();
}

function buildParticipantNodes(participants) {
    const frag = document.createDocumentFragment();
    (participants || []).forEach((line) => {
        const div = document.createElement('div');
        div.className = 'participant-tag';

        // New format: "Name - ROLE1 ROLE2"
        const newMatch = line.match(/^(.+?)\s*-\s+(.+)$/);
        // Old format: "Name (ROLE1, ROLE2)"
        const oldMatch = !newMatch ? line.match(/^(.+?)\s*\((.+?)\)$/) : null;

        if (newMatch) {
            const name = newMatch[1].trim();
            const roles = newMatch[2].split(/\s+/).filter(Boolean);
            div.appendChild(document.createTextNode(`${name} - `));
            roles.forEach((role, i) => {
                if (i) div.appendChild(document.createTextNode(' '));
                const roleSpan = createSafeRoleSpan(role);
                div.appendChild(roleSpan);
            });
        } else if (oldMatch) {
            const name = oldMatch[1].trim();
            const roles = oldMatch[2].split(',').map((r) => r.trim());
            div.appendChild(document.createTextNode(`${name} - (`));
            roles.forEach((role, i) => {
                if (i) div.appendChild(document.createTextNode(', '));
                const roleSpan = createSafeRoleSpan(role);
                div.appendChild(roleSpan);
            });
            div.appendChild(document.createTextNode(')'));
        } else {
            div.textContent = line;
        }
        frag.appendChild(div);
    });
    return frag;
}

function toYoutubeId11(raw) {
    const id = extractVideoId(String(raw || ''));
    return id && /^[a-zA-Z0-9_-]{11}$/.test(id) ? id : null;
}

function renderProjects() {
    const grid = document.getElementById('projectsGrid');
    if (!grid) return;

    clearEl(grid);

    if (store.projects.length === 0) {
        grid.appendChild(
            h(
                'div',
                {
                    className: 'empty-state',
                    style: { gridColumn: '1 / -1' },
                },
                [h('div', { className: 'empty-state-icon' }, ['📁']), h('p', {}, ['Проектов пока нет'])]
            )
        );
        return;
    }

    store.projects.forEach((project, idx) => {
        const statusClass = getStatusClass(project.status);
        const vid = toYoutubeId11(String(project.videoId || ''));

        const cardParts = [];
        if (vid) {
            const wrap = h('div', { className: 'project-video' }, []);
            const iframe = document.createElement('iframe');
            iframe.src = `https://www.youtube.com/embed/${vid}?rel=0`;
            iframe.setAttribute('frameborder', '0');
            iframe.setAttribute('allowfullscreen', '');
            iframe.setAttribute(
                'allow',
                'accelerometer; clipboard-write; encrypted-media; gyroscope; picture-in-picture; web-share'
            );
            iframe.setAttribute('referrerpolicy', 'strict-origin-when-cross-origin');
            wrap.appendChild(iframe);
            cardParts.push(wrap);
            const bar = h(
                'div',
                {
                    style: {
                        padding: 'var(--spacing-xs) var(--spacing-md)',
                        background: 'var(--color-surface-2)',
                        textAlign: 'center',
                    },
                },
                []
            );
            const a = document.createElement('a');
            a.href = `https://www.youtube.com/watch?v=${encodeURIComponent(vid)}`;
            a.target = '_blank';
            a.rel = 'noopener noreferrer';
            a.style.fontSize = 'var(--font-size-xs)';
            a.style.color = 'var(--color-secondary)';
            a.textContent = '🔗 Открыть на YouTube';
            bar.appendChild(a);
            cardParts.push(bar);
        } else {
            cardParts.push(
                h('div', { className: 'project-video' }, [h('div', { className: 'project-video-placeholder' }, ['🎬'])])
            );
        }

        const infoItems = [
            h('div', { className: 'project-info-item' }, [
                h('span', { className: 'project-info-label' }, ['ID:']),
                h('span', { className: 'project-info-value' }, [project.id || '—']),
            ]),
            h('div', { className: 'project-info-item' }, [
                h('span', { className: 'project-info-label' }, ['Статус:']),
                h('span', { className: `project-status ${statusClass}` }, [project.status || 'планируется']),
            ]),
            h('div', { className: 'project-info-item' }, [
                h('span', { className: 'project-info-label' }, ['Верифнут:']),
                h('span', { className: 'project-info-value' }, [project.verifier || '—']),
            ]),
        ];
        if (project.comment) {
            infoItems.push(
                h('div', { className: 'project-info-item' }, [
                    h('span', { className: 'project-info-label' }, ['Коммент:']),
                    h('span', { className: 'project-info-value' }, [project.comment]),
                ])
            );
        }

        const participantsList = h('div', { className: 'project-participants-list' }, []);
        participantsList.appendChild(buildParticipantNodes(project.participants));

        const contentChildren = [
            h('h3', { className: 'project-title' }, [project.name || `Проект #${idx + 1}`]),
            h('div', { className: 'project-info' }, infoItems),
            h('div', { className: 'project-participants' }, [
                h('div', { className: 'project-participants-title' }, ['Участники:']),
                participantsList,
            ]),
        ];

        if (store.isHost) {
            contentChildren.push(
                h('div', { className: 'project-actions' }, [
                    h('button', { className: 'btn btn-secondary btn-sm', attrs: { type: 'button' }, dataset: { action: 'move-project', index: String(idx), direction: 'up' } }, ['↑']),
                    h('button', { className: 'btn btn-secondary btn-sm', attrs: { type: 'button' }, dataset: { action: 'move-project', index: String(idx), direction: 'down' } }, ['↓']),
                    h('button', { className: 'btn btn-secondary btn-sm', attrs: { type: 'button' }, dataset: { action: 'edit-project', projectIndex: String(idx) } }, [
                        '✏️ Редактировать',
                    ]),
                    h('button', { className: 'btn btn-danger btn-sm', attrs: { type: 'button' }, dataset: { action: 'delete-project', projectIndex: String(idx) } }, [
                        '🗑️ Удалить',
                    ]),
                ])
            );
        }

        cardParts.push(h('div', { className: 'project-content' }, contentChildren));
        grid.appendChild(h('div', { className: 'project-card' }, cardParts));
    });
}

function getStatusClass(status) {
    const classes = {
        'готов': 'status-ready',
        'в процессе верифа': 'status-verifying',
        'в процессе постройки': 'status-building',
        'планируется': 'status-planned',
        'заморожен': 'status-frozen',
        'мёртв': 'status-dead'
    };
    return classes[status?.toLowerCase()] || 'status-planned';
}

function showAddProjectModal() {
    if (!store.isHost) {
        showToast('Только хост может добавлять проекты', 'error');
        return;
    }

    const modal = document.getElementById('projectModal');
    const form = document.getElementById('projectForm');

    if (modal && form) {
        form.reset();
        document.getElementById('projectIndex').value = '-1';
        document.getElementById('projectModalTitle').textContent = 'Добавить проект';
        resetParticipantBuilder();
        modal.classList.add('active');
        setTimeout(initParticipantBuilder, 50);
    }
}

function closeProjectModal() {
    const modal = document.getElementById('projectModal');
    if (modal) modal.classList.remove('active');
}

function resetParticipantBuilder() {
    store.pendingProjectParticipants = [];
    store._selectedParticipant = '';
    const preview = document.getElementById('participantsPreview');
    if (preview) preview.innerHTML = '';
    const searchInput = document.getElementById('participantSearchInput');
    if (searchInput) searchInput.value = '';
    const results = document.getElementById('participantSearchResults');
    if (results) results.innerHTML = '';
    document.querySelectorAll('#participantRoleTags .role-tag-btn').forEach(b => b.classList.remove('active'));
}

async function initParticipantBuilder() {
    if (!store.staffRoles || store.staffRoles.length === 0) {
        try {
            const res = await fetchWithAbort(`${BACKEND_URL}/staff`, {}, 'staff-list-part');
            if (res.ok) {
                const data = await res.json();
                store.staffRoles = Array.isArray(data) ? data : [];
            }
        } catch {}
    }

    const tagsContainer = document.getElementById('participantRoleTags');
    if (tagsContainer) {
        clearEl(tagsContainer);
        (store.staffRoles || []).forEach(role => {
            const btn = document.createElement('button');
            btn.type = 'button';
            btn.className = 'role-tag-btn';
            btn.dataset.action = 'toggle-role-tag';
            btn.dataset.role = role.name;
            if (role.color) {
                btn.dataset.color = role.color;
                btn.style.borderColor = role.color;
                btn.style.color = role.color;
            }
            btn.textContent = role.name;
            tagsContainer.appendChild(btn);
        });
    }

    updateParticipantsPreview();

    const searchInput = document.getElementById('participantSearchInput');
    const resultsContainer = document.getElementById('participantSearchResults');
    if (searchInput && resultsContainer) {
        searchInput.value = '';
        resultsContainer.innerHTML = '';
        searchInput.oninput = null;
        searchInput.oninput = function() {
            const q = this.value.toLowerCase().trim();
            resultsContainer.innerHTML = '';
            if (!q) { resultsContainer.style.display = 'none'; return; }

            const matches = [];
            (store.staffRoles || []).forEach(role => {
                (role.players || []).forEach(player => {
                    if (player.nickname.toLowerCase().includes(q) && !matches.some(m => m.nickname === player.nickname)) {
                        matches.push({ nickname: player.nickname, role: role.name, color: role.color });
                    }
                });
            });
            matches.sort((a, b) => a.nickname.localeCompare(b.nickname));

            if (matches.length === 0) {
                resultsContainer.style.display = 'none';
                return;
            }

            resultsContainer.style.display = 'block';
            matches.forEach(m => {
                const item = document.createElement('div');
                item.className = 'participant-search-result-item';
                if (m.color) item.style.borderLeftColor = m.color;
                item.innerHTML = `<span class="psr-name">${escapeHtml(m.nickname)}</span> <span class="psr-role">${escapeHtml(m.role)}</span>`;
                item.addEventListener('click', () => {
                    document.querySelectorAll('.participant-search-result-item').forEach(el => el.classList.remove('selected'));
                    item.classList.add('selected');
                    store._selectedParticipant = m.nickname;
                    resultsContainer.style.display = 'none';
                    searchInput.value = m.nickname;
                });
                resultsContainer.appendChild(item);
            });
        };

        searchInput.addEventListener('blur', () => {
            setTimeout(() => { resultsContainer.style.display = 'none'; }, 150);
        });
        searchInput.addEventListener('focus', () => {
            if (searchInput.value.trim()) {
                searchInput.oninput();
            }
        });
    }

    store._selectedParticipant = '';
}

function addProjectParticipant() {
    const name = store._selectedParticipant || document.getElementById('participantSearchInput')?.value?.trim();
    if (!name) {
        showToast('Введите или выберите игрока', 'error');
        return;
    }
    const activeRoles = [];
    document.querySelectorAll('#participantRoleTags .role-tag-btn.active').forEach(b => {
        activeRoles.push(b.dataset.role);
    });
    let entry = name;
    if (activeRoles.length > 0) {
        entry = name + ' - ' + activeRoles.join(' ');
    }
    store.pendingProjectParticipants.push(entry);
    store._selectedParticipant = '';
    const searchInput = document.getElementById('participantSearchInput');
    if (searchInput) searchInput.value = '';
    const results = document.getElementById('participantSearchResults');
    if (results) results.innerHTML = '';
    document.querySelectorAll('#participantRoleTags .role-tag-btn').forEach(b => b.classList.remove('active'));
    updateParticipantsPreview();
}

function removeProjectParticipant(index) {
    if (index >= 0 && index < store.pendingProjectParticipants.length) {
        store.pendingProjectParticipants.splice(index, 1);
        updateParticipantsPreview();
    }
}

function updateParticipantsPreview() {
    const preview = document.getElementById('participantsPreview');
    if (!preview) return;
    clearEl(preview);
    if (!store.pendingProjectParticipants || store.pendingProjectParticipants.length === 0) {
        preview.appendChild(
            h('span', { style: { color: 'var(--color-text-muted)', fontSize: 'var(--font-size-xs)' } }, ['Участники не добавлены'])
        );
        return;
    }
    store.pendingProjectParticipants.forEach((entry, i) => {
        const tag = document.createElement('span');
        tag.className = 'participant-tag participant-preview-tag';
        const newMatch = entry.match(/^(.+?)\s*-\s+(.+)$/);
        if (newMatch) {
            const name = newMatch[1].trim();
            const roles = newMatch[2].split(/\s+/).filter(Boolean);
            tag.appendChild(document.createTextNode(`${name} - `));
            roles.forEach((role, ri) => {
                if (ri) tag.appendChild(document.createTextNode(' '));
                tag.appendChild(createSafeRoleSpan(role));
            });
        } else {
            tag.appendChild(h('span', {}, [escapeHtml(entry)]));
        }
        const removeBtn = document.createElement('button');
        removeBtn.className = 'staff-player-remove-tag';
        removeBtn.dataset.action = 'remove-project-participant';
        removeBtn.dataset.index = String(i);
        removeBtn.title = 'Удалить';
        removeBtn.textContent = '✕';
        tag.appendChild(removeBtn);
        preview.appendChild(tag);
    });
}

function editProject(idx) {
    if (!store.isHost) {
        showToast('Только хост может редактировать проекты', 'error');
        return;
    }

    const project = store.projects[idx];
    if (!project) return;

    document.getElementById('projectIndex').value = idx;
    document.getElementById('projectModalTitle').textContent = 'Редактировать проект';
    document.getElementById('projectName').value = project.name || '';
    document.getElementById('projectVideo').value = project.videoId || '';
    document.getElementById('projectId').value = project.id || '';
    document.getElementById('projectComment').value = project.comment || '';
    document.getElementById('projectStatus').value = project.status || 'планируется';
    document.getElementById('projectVerifier').value = project.verifier || '';

    store.pendingProjectParticipants = [...(project.participants || [])];
    updateParticipantsPreview();

    document.getElementById('projectModal').classList.add('active');
    setTimeout(initParticipantBuilder, 50);
}

async function saveProject() {
    const idx = parseInt(document.getElementById('projectIndex').value);
    const project = {
        name: document.getElementById('projectName').value.trim(),
        videoId: extractVideoId(document.getElementById('projectVideo').value.trim()),
        id: document.getElementById('projectId').value.trim(),
        comment: document.getElementById('projectComment').value.trim(),
        status: document.getElementById('projectStatus').value,
        verifier: document.getElementById('projectVerifier').value.trim(),
        participants: Array.isArray(store.pendingProjectParticipants) ? store.pendingProjectParticipants.filter(Boolean) : []
    };

    const oldProject = idx === -1 ? null : { ...store.projects[idx] };
    if (idx === -1) {
        store.projects.push(project);
    } else {
        store.projects[idx] = project;
    }

    try {
        await saveProjects(store.projects);
        showToast(idx === -1 ? 'Проект добавлен!' : 'Проект обновлён!', 'success');
        closeProjectModal();
        renderProjects();
    } catch (e) {
        if (isAbortError(e)) return;
        if (idx === -1) {
            store.projects.pop();
        } else {
            store.projects[idx] = oldProject;
        }
        showToast(e.message, 'error');
    }
}

async function deleteProject(idx) {
    if (!store.isHost) {
        showToast('Только хост может удалять проекты', 'error');
        return;
    }

    if (!confirm('Удалить этот проект?')) return;

    const removed = store.projects.splice(idx, 1);
    try {
        await saveProjects(store.projects);
        renderProjects();
        showToast('Проект удалён', 'success');
    } catch (e) {
        if (isAbortError(e)) return;
        store.projects.splice(idx, 0, removed[0]);
        showToast(e.message, 'error');
    }
}

function extractVideoId(url) {
    if (!url) return '';

    const patterns = [
        /(?:youtube\.com\/(?:watch\?v=|embed\/|shorts\/)|youtu\.be\/)([a-zA-Z0-9_-]{11})/,
        /^([a-zA-Z0-9_-]{11})$/
    ];

    for (const pattern of patterns) {
        const match = url.match(pattern);
        if (match) return match[1];
    }

    return '';
}

// ============================================
// ИНФОРМАЦИЯ И УТИЛИТЫ
// ============================================

function showInfoModal() {
    const modal = document.getElementById('infoModal');
    if (modal) modal.classList.add('active');
}

function closeInfoModal(e) {
    if (!e || e.target === e.currentTarget) {
        const modal = document.getElementById('infoModal');
        if (modal) modal.classList.remove('active');
    }
}

// ============================================
// СТАФФ (УПРАВЛЕНИЕ РОЛЯМИ)
// ============================================

async function initStaffPage() {
    await Promise.all([
        loadStaffRoles(),
        loadStaffTiers()
    ]);
    initStaffEventListeners();
}

function initStaffEventListeners() {
    const colorInput = document.getElementById('roleColor');
    if (colorInput) {
        colorInput.addEventListener('input', () => {
            store.selectedRoleColor = colorInput.value;
            const hexInput = document.getElementById('roleColorHex');
            if (hexInput) hexInput.value = colorInput.value.replace('#', '');
        });
    }

    const colorHexInput = document.getElementById('roleColorHex');
    if (colorHexInput) {
        colorHexInput.addEventListener('input', () => {
            let val = colorHexInput.value.replace(/[^0-9a-fA-F]/g, '').slice(0, 6);
            colorHexInput.value = val;
            if (val.length === 6) {
                const fullColor = '#' + val.toLowerCase();
                store.selectedRoleColor = fullColor;
                document.getElementById('roleColor').value = fullColor;
            }
        });
    }

    const roleNameInput = document.getElementById('roleName');
    if (roleNameInput) {
        roleNameInput.addEventListener('keypress', (e) => {
            if (e.key === 'Enter') {
                e.preventDefault();
                createRole();
            }
        });
    }

    const playerNickname = document.getElementById('playerNickname');
    if (playerNickname) {
        playerNickname.addEventListener('keypress', (e) => {
            if (e.key === 'Enter') {
                e.preventDefault();
                addPlayerToRole();
            }
        });
    }

    const editPlayerNickname = document.getElementById('editPlayerNickname');
    if (editPlayerNickname) {
        editPlayerNickname.addEventListener('keypress', (e) => {
            if (e.key === 'Enter') {
                e.preventDefault();
                editAddPlayer();
            }
        });
    }

    const roleAddPlayerNickname = document.getElementById('roleAddPlayerNickname');
    if (roleAddPlayerNickname) {
        roleAddPlayerNickname.addEventListener('keypress', (e) => {
            if (e.key === 'Enter') {
                e.preventDefault();
                const btn = document.getElementById('roleAddPlayerBtn');
                if (btn.dataset.action === 'role-modal-save-player') {
                    roleModalSavePlayer();
                } else {
                    addPlayerFromRoleModal();
                }
            }
        });
    }

    const editPlayerSearch = document.getElementById('editPlayerSearch');
    if (editPlayerSearch) {
        editPlayerSearch.addEventListener('input', () => renderEditPlayerList());
    }

    const rolePlayerSearch = document.getElementById('rolePlayerSearch');
    if (rolePlayerSearch) {
        rolePlayerSearch.addEventListener('input', () => {
            const roleIndex = parseInt(document.getElementById('editRoleIndex')?.value || '-1');
            if (roleIndex >= 0) renderRoleModalPlayerList(roleIndex);
        });
    }
}

async function loadStaffRoles() {
    const loadingState = document.getElementById('staffLoadingState');
    if (loadingState) loadingState.style.display = 'flex';

    try {
        const res = await fetchWithAbort(`${BACKEND_URL}/staff`, {}, 'staff-list');
        if (!res.ok) {
            store.staffRoles = [];
        } else {
            const data = await res.json();
            store.staffRoles = Array.isArray(data) ? data : [];
        }
    } catch (e) {
        if (!isAbortError(e)) {
            console.error('Ошибка загрузки staff ролей:', e);
            store.staffRoles = [];
        }
    } finally {
        if (loadingState) loadingState.style.display = 'none';
        renderStaffRoles();
    }
}

async function saveStaffRoles() {
    try {
        const res = await fetchWithAbort(`${BACKEND_URL}/staff`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'include',
            body: JSON.stringify(store.staffRoles)
        }, 'staff-save');

        if (!res.ok) {
            const err = await res.json().catch(() => ({}));
            throw new Error(err.error || 'Ошибка сохранения ролей');
        }
    } catch (e) {
        if (!isAbortError(e)) {
            console.error('Ошибка сохранения staff ролей:', e);
            showToast(e.message, 'error');
            if (e.message.includes('401')) {
                logoutHost();
            }
        }
    }
}

function renderStaffRoles() {
    const container = document.getElementById('staffRolesContainer');
    const emptyState = document.getElementById('staffEmptyState');
    if (!container) return;

    clearEl(container);

    if (store.staffRoles.length === 0) {
        if (emptyState) emptyState.style.display = 'flex';
        return;
    }

    if (emptyState) emptyState.style.display = 'none';

    store.staffRoles.forEach((role, roleIndex) => {
        const roleColor = role.color || '#3b82f6';
        const players = role.players || [];

        const cardParts = [];

        const visual = h('div', { className: 'staff-role-visual', style: { background: roleColor } }, [
            h('span', { className: 'staff-role-visual-name' }, [escapeHtml(role.name)])
        ]);
        cardParts.push(visual);

        const infoItems = [
            h('div', { className: 'project-info-item' }, [
                h('span', { className: 'project-info-label' }, ['Роль:']),
                h('span', { className: 'project-info-value', style: { color: roleColor, fontWeight: 700 } }, [escapeHtml(role.name)]),
            ]),
            h('div', { className: 'project-info-item' }, [
                h('span', { className: 'project-info-label' }, ['Участников:']),
                h('span', { className: 'project-info-value' }, [String(players.length)]),
            ]),
        ];

        const participantsList = h('div', { className: 'project-participants-list' }, []);

        if (players.length === 0) {
            participantsList.appendChild(
                h('span', { style: { color: 'var(--color-text-muted)', fontSize: 'var(--font-size-xs)' } }, ['Нет игроков'])
            );
        } else {
            players.forEach((player, pIdx) => {
                const tagParts = [
                    h('span', { className: 'nickname-glow' }, [escapeHtml(player.nickname)]),
                    ...(player.discord ? [h('span', { className: 'staff-player-discord-inline' }, [escapeHtml(player.discord)])] : []),
                ];

                if (role.tiersEnabled !== false) {
                    const tier = getPlayerTier(player.nickname);
                    const cfg = TIER_CONFIG[tier];
                    tagParts.push(
                        h('span', {
                            className: 'staff-tier-dot',
                            style: { background: cfg.color },
                            title: cfg.label
                        }, [])
                    );
                }

                if (store.isHost) {
                    tagParts.push(
                        h('button', {
                            className: 'staff-player-remove-tag',
                            attrs: {
                                'data-action': 'remove-staff-player',
                                'data-role-index': String(roleIndex),
                                'data-player-index': String(pIdx),
                                'title': 'Удалить игрока'
                            }
                        }, ['✕'])
                    );
                }

                participantsList.appendChild(
                    h('div', { className: 'staff-player-row' }, [
                        h('span', { className: 'participant-tag staff-player-tag' }, tagParts)
                    ])
                );
            });
        }

        const contentChildren = [
            h('div', { className: 'project-info' }, infoItems),
            h('div', { className: 'project-participants' }, [
                h('div', { className: 'project-participants-title' }, ['Участники:']),
                participantsList,
            ]),
        ];

        if (store.isHost) {
            contentChildren.push(
                h('div', { className: 'project-actions' }, [
                    h('button', {
                        className: 'btn btn-secondary btn-sm',
                        attrs: { type: 'button' },
                        dataset: { action: 'move-role', index: String(roleIndex), direction: 'up' }
                    }, ['↑']),
                    h('button', {
                        className: 'btn btn-secondary btn-sm',
                        attrs: { type: 'button' },
                        dataset: { action: 'move-role', index: String(roleIndex), direction: 'down' }
                    }, ['↓']),
                    h('button', {
                        className: 'btn btn-primary btn-sm',
                        attrs: { type: 'button' },
                        dataset: { action: 'show-edit-role-modal', roleIndex: String(roleIndex) }
                    }, ['✏️ Редактировать']),
                    h('button', {
                        className: 'btn btn-danger btn-sm',
                        attrs: { type: 'button' },
                        dataset: { action: 'delete-role', roleIndex: String(roleIndex) }
                    }, ['🗑️ Удалить роль']),
                ])
            );
        }

        cardParts.push(h('div', { className: 'project-content' }, contentChildren));
        container.appendChild(h('div', { className: 'project-card' }, cardParts));
    });
}

function moveRole(index, direction) {
    const target = direction === 'down' ? index + 1 : index - 1;
    if (target < 0 || target >= store.staffRoles.length) return;
    const prev = [...store.staffRoles];
    [store.staffRoles[index], store.staffRoles[target]] = [store.staffRoles[target], store.staffRoles[index]];
    renderStaffRoles();
    fetchWithAbort(`${BACKEND_URL}/staff/reorder`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({ roleIndex: index, direction })
    }, 'staff-reorder').catch(() => {
        store.staffRoles = prev;
        renderStaffRoles();
    });
}

function moveProject(index, direction) {
    const target = direction === 'down' ? index + 1 : index - 1;
    if (target < 0 || target >= store.projects.length) return;
    [store.projects[index], store.projects[target]] = [store.projects[target], store.projects[index]];
    renderProjects();
    saveProjects(store.projects).catch(e => {
        [store.projects[index], store.projects[target]] = [store.projects[target], store.projects[index]];
        renderProjects();
        showToast(e.message, 'error');
    });
}

function showAddRoleModal() {
    const modal = document.getElementById('addRoleModal');
    if (modal) {
        document.getElementById('editRoleIndex').value = '-1';
        document.getElementById('roleName').value = '';
        document.getElementById('roleColor').value = store.selectedRoleColor || '#3b82f6';
        const hexInput = document.getElementById('roleColorHex');
        if (hexInput) hexInput.value = (store.selectedRoleColor || '#3b82f6').replace('#', '');
        document.getElementById('addRoleModalTitle').textContent = '🆕 Новая роль';
        document.getElementById('createRoleBtn').textContent = 'Создать';
        const playerSection = document.getElementById('rolePlayerSection');
        if (playerSection) playerSection.style.display = 'none';
        modal.classList.add('active');
        setTimeout(() => document.getElementById('roleName').focus(), 100);
    }
}

function showEditRoleModal(roleIndex) {
    const role = store.staffRoles[roleIndex];
    if (!role) return;
    const modal = document.getElementById('addRoleModal');
    if (modal) {
        document.getElementById('editRoleIndex').value = roleIndex;
        document.getElementById('editRolePlayerIdx').value = '-1';
        document.getElementById('roleName').value = role.name;
        const color = role.color || '#3b82f6';
        document.getElementById('roleColor').value = color;
        const hexInput = document.getElementById('roleColorHex');
        if (hexInput) hexInput.value = color.replace('#', '');
        store.selectedRoleColor = color;
        document.getElementById('addRoleModalTitle').textContent = '✏️ Редактировать роль';
        document.getElementById('createRoleBtn').textContent = 'Сохранить';
        const playerSection = document.getElementById('rolePlayerSection');
        if (playerSection) playerSection.style.display = 'block';
        document.getElementById('roleAddPlayerNickname').value = '';
        document.getElementById('roleAddPlayerDiscord').value = '';
        const searchInput = document.getElementById('rolePlayerSearch');
        if (searchInput) searchInput.value = '';
        const addBtn = document.getElementById('roleAddPlayerBtn');
        if (addBtn) { addBtn.textContent = '➕ Добавить'; addBtn.dataset.action = 'role-add-player'; }
        const toggleBtn = document.getElementById('roleToggleTiersBtn');
        if (toggleBtn) toggleBtn.textContent = role.tiersEnabled !== false ? '🎯 Тир: вкл' : '🎯 Тир: выкл';
        renderRoleModalPlayerList(roleIndex);
        modal.classList.add('active');
        setTimeout(() => document.getElementById('roleName').focus(), 100);
    }
}

function closeAddRoleModal() {
    const modal = document.getElementById('addRoleModal');
    if (modal) modal.classList.remove('active');
}

async function createRole() {
    const editIndexInput = document.getElementById('editRoleIndex');
    const editIndex = parseInt(editIndexInput?.value || '-1');
    if (editIndex >= 0) {
        await updateRole(editIndex);
        return;
    }

    const nameInput = document.getElementById('roleName');
    const name = nameInput.value.trim();
    if (!name) {
        showToast('Введите название роли', 'error');
        return;
    }

    const color = document.getElementById('roleColor').value || '#3b82f6';

    try {
        const res = await fetchWithAbort(`${BACKEND_URL}/staff/role`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'include',
            body: JSON.stringify({ name, color })
        }, 'create-role');

        if (!res.ok) {
            const err = await parseJsonResponse(res);
            throw new Error(err.error || 'Ошибка создания роли');
        }

        await loadStaffRoles();
        closeAddRoleModal();
        nameInput.value = '';
        showToast(`Роль «${name}» создана`, 'success');
    } catch (e) {
        if (!isAbortError(e)) {
            console.error('Ошибка создания роли:', e);
            showToast(e.message, 'error');
            if (e.message.includes('401')) {
                logoutHost();
            }
        }
    }
}

async function updateRole(roleIndex) {
    const role = store.staffRoles[roleIndex];
    if (!role) return;

    const nameInput = document.getElementById('roleName');
    const name = nameInput.value.trim();
    if (!name) {
        showToast('Введите название роли', 'error');
        return;
    }

    const color = document.getElementById('roleColor').value || '#3b82f6';

    try {
        const res = await fetchWithAbort(`${BACKEND_URL}/staff/role`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'include',
            body: JSON.stringify({ roleIndex, name, color })
        }, 'update-role');

        if (!res.ok) {
            const err = await parseJsonResponse(res);
            throw new Error(err.error || 'Ошибка обновления роли');
        }

        await loadStaffRoles();
        closeAddRoleModal();
        showToast(`Роль «${name}» обновлена`, 'success');
    } catch (e) {
        if (!isAbortError(e)) {
            console.error('Ошибка обновления роли:', e);
            showToast(e.message, 'error');
            if (e.message.includes('401')) {
                logoutHost();
            }
        }
    }
}

async function deleteRole(index) {
    const role = store.staffRoles[index];
    if (!role) return;

    if (!confirm(`Удалить роль «${escapeHtml(role.name)}»?`)) return;

    try {
        const res = await fetchWithAbort(`${BACKEND_URL}/staff/role`, {
            method: 'DELETE',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'include',
            body: JSON.stringify({ roleIndex: index })
        }, 'delete-role');

        if (!res.ok) {
            const err = await parseJsonResponse(res);
            throw new Error(err.error || 'Ошибка удаления роли');
        }

        await loadStaffRoles();
        showToast('Роль удалена', 'success');
    } catch (e) {
        if (!isAbortError(e)) {
            console.error('Ошибка удаления роли:', e);
            showToast(e.message, 'error');
            if (e.message.includes('401')) {
                logoutHost();
            }
        }
    }
}

function showAddStaffPlayerModal(roleIndex) {
    const modal = document.getElementById('addPlayerModal');
    const title = document.getElementById('addPlayerModalTitle');
    const roleIndexInput = document.getElementById('addPlayerRoleIndex');
    const nicknameInput = document.getElementById('playerNickname');
    const discordInput = document.getElementById('playerDiscord');

    if (modal && title && roleIndexInput) {
        const role = store.staffRoles[roleIndex];
        if (!role) return;
        title.textContent = `➕ Добавить игрока в «${escapeHtml(role.name)}»`;
        roleIndexInput.value = roleIndex;
        if (nicknameInput) nicknameInput.value = '';
        if (discordInput) discordInput.value = '';
        modal.classList.add('active');
        setTimeout(() => { if (nicknameInput) nicknameInput.focus(); }, 100);
    }
}

function closeAddStaffPlayerModal() {
    const modal = document.getElementById('addPlayerModal');
    if (modal) modal.classList.remove('active');
}

async function addPlayerToRole() {
    const roleIndexInput = document.getElementById('addPlayerRoleIndex');
    const nicknameInput = document.getElementById('playerNickname');
    const discordInput = document.getElementById('playerDiscord');

    const roleIndex = parseInt(roleIndexInput?.value || '-1');
    if (roleIndex < 0 || roleIndex >= store.staffRoles.length) {
        showToast('Ошибка: роль не найдена', 'error');
        return;
    }

    const nickname = nicknameInput?.value?.trim();
    if (!nickname) {
        showToast('Введите ник игрока', 'error');
        return;
    }

    const discord = discordInput?.value?.trim() || '';
    const role = store.staffRoles[roleIndex];

    try {
        const res = await fetchWithAbort(`${BACKEND_URL}/staff/add`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'include',
            body: JSON.stringify({ roleIndex, nickname, discord })
        }, 'add-player');

        if (!res.ok) {
            const err = await parseJsonResponse(res);
            throw new Error(err.error || 'Ошибка добавления игрока');
        }

        await Promise.all([
            loadStaffRoles(),
            loadStaffTiers()
        ]);
        closeAddStaffPlayerModal();
        renderEditPlayerList();
        showToast(`Игрок «${escapeHtml(nickname)}» добавлен в роль «${escapeHtml(role.name)}»`, 'success');
    } catch (e) {
        if (!isAbortError(e)) {
            console.error('Ошибка добавления игрока:', e);
            showToast(e.message, 'error');
            if (e.message.includes('401')) {
                logoutHost();
            }
        }
    }
}

async function removeStaffPlayer(roleIndex, playerIndex) {
    const role = store.staffRoles[roleIndex];
    if (!role) return;

    const player = role.players[playerIndex];
    if (!player) return;

    if (!confirm(`Удалить игрока «${escapeHtml(player.nickname)}» из роли «${escapeHtml(role.name)}»?`)) return;

    try {
        const res = await fetchWithAbort(`${BACKEND_URL}/staff/remove`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'include',
            body: JSON.stringify({ roleIndex, nickname: player.nickname })
        }, 'remove-player');

        if (!res.ok) {
            const err = await parseJsonResponse(res);
            throw new Error(err.error || 'Ошибка удаления игрока');
        }

        await Promise.all([
            loadStaffRoles(),
            loadStaffTiers()
        ]);
        renderEditPlayerList();
        const roleIdx = parseInt(document.getElementById('editRoleIndex')?.value || '-1');
        if (roleIdx >= 0) renderRoleModalPlayerList(roleIdx);
        showToast(`Игрок «${escapeHtml(player.nickname)}» удалён из роли`, 'success');
    } catch (e) {
        if (!isAbortError(e)) {
            console.error('Ошибка удаления игрока:', e);
            showToast(e.message, 'error');
            if (e.message.includes('401')) {
                logoutHost();
            }
        }
    }

    return true;
}

// ============================================
// ТИРЫ
// ============================================

const TIER_CONFIG = {
    priority: { label: 'Приоритет', color: '#00ffff' },
    base: { label: 'Основа', color: '#540b6d' },
    reserve: { label: 'Резерв', color: '#6d0b0d' },
    na: { label: 'N/A', color: '#888888' },
};

const TIER_CYCLE = ['na', 'priority', 'base', 'reserve'];

async function loadStaffTiers() {
    try {
        const res = await fetchWithAbort(`${BACKEND_URL}/staff/tiers`, {}, 'staff-tiers');
        if (res.ok) {
            const data = await res.json();
            store.staffTiers = Array.isArray(data.gp) ? data.gp : [];
        } else {
            store.staffTiers = [];
        }
    } catch (e) {
        if (!isAbortError(e)) {
            console.error('Ошибка загрузки тиров:', e);
            store.staffTiers = [];
        }
    }
}

function getNextTier(current) {
    const idx = TIER_CYCLE.indexOf(current);
    if (idx === -1 || idx >= TIER_CYCLE.length - 1) return TIER_CYCLE[0];
    return TIER_CYCLE[idx + 1];
}

async function setPlayerTier(nickname) {
    if (!store.isHost) return;

    const current = store.staffTiers.find(t => t.nickname === nickname);
    const currentTier = current ? current.tier : 'na';
    const nextTier = getNextTier(currentTier);

    try {
        const res = await fetchWithAbort(`${BACKEND_URL}/staff/tier`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'include',
            body: JSON.stringify({ category: 'gp', nickname, tier: nextTier })
        }, 'set-tier');

        if (!res.ok) {
            const err = await parseJsonResponse(res);
            throw new Error(err.error || 'Ошибка установки тира');
        }

        await loadStaffTiers();
        renderEditPlayerList();
        const roleIdx = parseInt(document.getElementById('editRoleIndex')?.value || '-1');
        if (roleIdx >= 0) renderRoleModalPlayerList(roleIdx);
        showToast(`${escapeHtml(nickname)} → ${TIER_CONFIG[nextTier].label}`, 'success');
    } catch (e) {
        if (!isAbortError(e)) {
            console.error('Ошибка установки тира:', e);
            showToast(e.message, 'error');
            if (e.message.includes('401')) {
                logoutHost();
            }
        }
    }
}

function getPlayerTier(nickname) {
    const entry = store.staffTiers.find(t => t.nickname === nickname);
    return entry ? entry.tier : 'na';
}

async function setPlayerTierSilent(nickname, tier) {
    if (!store.isHost) return;
    try {
        await fetchWithAbort(`${BACKEND_URL}/staff/tier`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'include',
            body: JSON.stringify({ category: 'gp', nickname, tier })
        }, 'set-tier-silent');
    } catch (e) {
        if (!isAbortError(e)) {
            console.error('Ошибка установки тира:', e);
            if (e.message.includes('401')) logoutHost();
        }
    }
}

async function setPlayerTierDirect(nickname, tier) {
    if (!store.isHost) return;
    await setPlayerTierSilent(nickname, tier);
    await loadStaffTiers();
    renderStaffRoles();
    const roleIdx = parseInt(document.getElementById('editRoleIndex')?.value || '-1');
    if (roleIdx >= 0) renderRoleModalPlayerList(roleIdx);
}

async function roleModalSortByTiers(roleIndex) {
    const role = store.staffRoles[roleIndex];
    if (!role || !role.players) return;

    const tierOrder = { priority: 0, base: 1, reserve: 2, na: 3 };
    role.players.sort((a, b) => {
        const ta = tierOrder[getPlayerTier(a.nickname)] ?? 3;
        const tb = tierOrder[getPlayerTier(b.nickname)] ?? 3;
        if (ta !== tb) return ta - tb;
        return a.nickname.localeCompare(b.nickname);
    });

    await roleModalSavePlayers(roleIndex);
    renderRoleModalPlayerList(roleIndex);
    showToast('Участники отсортированы по тирам', 'success');
}

async function roleModalToggleTiers(roleIndex) {
    const role = store.staffRoles[roleIndex];
    if (!role) return;

    role.tiersEnabled = role.tiersEnabled === false ? true : false;

    try {
        const res = await fetchWithAbort(`${BACKEND_URL}/staff/role`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'include',
            body: JSON.stringify({ roleIndex, name: role.name, color: role.color, tiersEnabled: role.tiersEnabled })
        }, 'role-toggle-tiers');
        if (!res.ok) { const err = await parseJsonResponse(res); throw new Error(err.error || 'Ошибка'); }
        await loadStaffRoles();
        const btn = document.getElementById('roleToggleTiersBtn');
        if (btn) btn.textContent = role.tiersEnabled ? '🎯 Тир: вкл' : '🎯 Тир: выкл';
        renderRoleModalPlayerList(roleIndex);
        showToast(`Тиры для роли «${escapeHtml(role.name)}» ${role.tiersEnabled ? 'включены' : 'выключены'}`, 'success');
    } catch (e) {
        role.tiersEnabled = !role.tiersEnabled;
        showToast(e.message, 'error');
    }
}

// ============================================
// ПАНЕЛЬ РЕДАКТИРОВАНИЯ
// ============================================

function openEditPanel() {
    if (!store.isHost) return;
    document.getElementById('editPanelOverlay').classList.add('active');
    document.getElementById('editPanel').classList.add('open');
    document.body.style.overflow = 'hidden';
    populateEditRoleSelect();
    document.getElementById('editPlayerKey').value = '';
    const searchInput = document.getElementById('editPlayerSearch');
    if (searchInput) searchInput.value = '';
    renderEditPlayerList();
    document.getElementById('editPlayerNickname').value = '';
    document.getElementById('editPlayerDiscord').value = '';
    const btn = document.getElementById('editPanelSubmitBtn');
    if (btn) { btn.textContent = '➕ Добавить игрока'; btn.dataset.action = 'edit-add-player'; }
    setTimeout(() => document.getElementById('editPlayerNickname').focus(), 100);
}

function closeEditPanel() {
    const overlay = document.getElementById('editPanelOverlay');
    const panel = document.getElementById('editPanel');
    if (overlay) overlay.classList.remove('active');
    if (panel) panel.classList.remove('open');
    document.body.style.overflow = '';
}

function populateEditRoleSelect() {
    const select = document.getElementById('editPlayerRole');
    if (!select) return;
    clearEl(select);

    const placeholder = document.createElement('option');
    placeholder.value = '';
    placeholder.textContent = 'Выберите роль...';
    placeholder.disabled = true;
    placeholder.selected = true;
    select.appendChild(placeholder);

    store.staffRoles.forEach((role, idx) => {
        const opt = document.createElement('option');
        opt.value = String(idx);
        opt.textContent = role.name;
        select.appendChild(opt);
    });
}

function renderEditPlayerList() {
    const container = document.getElementById('editPlayerList');
    if (!container) return;
    clearEl(container);

    if (!store.isHost) return;

    const searchInput = document.getElementById('editPlayerSearch');
    const query = searchInput ? searchInput.value.toLowerCase().trim() : '';

    let totalPlayers = 0;
    for (const role of store.staffRoles) {
        const rolePlayers = role.players || [];
        for (const p of rolePlayers) {
            if (query && !p.nickname.toLowerCase().includes(query)) continue;
            totalPlayers++;
            const tier = getPlayerTier(p.nickname);
            const cfg = TIER_CONFIG[tier];
            const roleIndex = store.staffRoles.indexOf(role);
            const playerIndex = role.players.indexOf(p);
            const item = h('div', { className: 'edit-player-list-item' }, [
                h('div', { className: 'player-info' }, [
                    h('span', { className: 'player-nickname' }, [escapeHtml(p.nickname)]),
                    h('span', { className: 'player-role-name' }, [escapeHtml(role.name)]),
                ]),
                ...Object.entries(TIER_CONFIG).map(([key, cfg]) => {
                    const isActive = getPlayerTier(p.nickname) === key;
                    return h('span', {
                        className: 'role-tier-square',
                        style: {
                            background: cfg.color,
                            opacity: isActive ? '1' : '0.25',
                            outline: isActive ? '2px solid var(--color-text-primary)' : 'none',
                            outlineOffset: '1px'
                        },
                        dataset: { action: 'role-modal-set-tier-direct', nickname: p.nickname, tier: key },
                        title: cfg.label
                    }, []);
                }),
                h('button', {
                    className: 'player-edit-btn',
                    attrs: {
                        'data-action': 'edit-player-from-list',
                        'data-role-index': String(roleIndex),
                        'data-player-index': String(playerIndex),
                        'title': 'Редактировать игрока'
                    }
                }, ['✏️']),
                h('button', {
                    className: 'player-remove-btn',
                    attrs: {
                        'data-action': 'edit-remove-player',
                        'data-role-index': String(roleIndex),
                        'data-nickname': p.nickname,
                        'title': 'Удалить игрока'
                    }
                }, ['✕']),
            ]);
            container.appendChild(item);
        }
    }

    if (totalPlayers === 0) {
        container.appendChild(
            h('span', { style: { color: 'var(--color-text-muted)', fontSize: 'var(--font-size-xs)' } },
                [query ? 'Ничего не найдено' : 'Нет игроков']
            )
        );
    }
}

function editPlayerFromList(roleIndex, playerIndex) {
    const role = store.staffRoles[roleIndex];
    if (!role) return;
    const player = role.players[playerIndex];
    if (!player) return;

    document.getElementById('editPlayerKey').value = roleIndex + ':' + playerIndex;
    document.getElementById('editPlayerNickname').value = player.nickname;
    document.getElementById('editPlayerDiscord').value = player.discord || '';
    document.getElementById('editPlayerRole').value = String(roleIndex);

    const btn = document.getElementById('editPanelSubmitBtn');
    btn.textContent = '💾 Сохранить изменения';
    btn.dataset.action = 'edit-save-player';
}

function cancelEditPlayer() {
    document.getElementById('editPlayerKey').value = '';
    document.getElementById('editPlayerNickname').value = '';
    document.getElementById('editPlayerDiscord').value = '';
    document.getElementById('editPlayerRole').value = '';
    const btn = document.getElementById('editPanelSubmitBtn');
    btn.textContent = '➕ Добавить игрока';
    btn.dataset.action = 'edit-add-player';
    renderEditPlayerList();
}

function renderRoleModalPlayerList(roleIndex) {
    const container = document.getElementById('rolePlayerList');
    if (!container) return;
    clearEl(container);

    const role = store.staffRoles[roleIndex];
    if (!role) return;

    const searchInput = document.getElementById('rolePlayerSearch');
    const query = searchInput ? searchInput.value.toLowerCase().trim() : '';

    const tiersEnabled = role.tiersEnabled !== false;

    const players = role.players || [];
    if (players.length === 0) {
        container.appendChild(
            h('span', { style: { color: 'var(--color-text-muted)', fontSize: 'var(--font-size-xs)' } }, ['Нет игроков'])
        );
        return;
    }

    let count = 0;
    for (let pIdx = 0; pIdx < players.length; pIdx++) {
        const p = players[pIdx];
        if (query && !p.nickname.toLowerCase().includes(query)) continue;
        count++;

        const actions = [];

        if (tiersEnabled) {
            Object.entries(TIER_CONFIG).forEach(([key, cfg]) => {
                const isActive = getPlayerTier(p.nickname) === key;
                actions.push(
                    h('span', {
                        className: 'role-tier-square',
                        style: {
                            background: cfg.color,
                            opacity: isActive ? '1' : '0.25',
                            outline: isActive ? '2px solid var(--color-text-primary)' : 'none',
                            outlineOffset: '1px'
                        },
                        dataset: { action: 'role-modal-set-tier-direct', nickname: p.nickname, tier: key },
                        title: cfg.label
                    }, [])
                );
            });
        }

        actions.push(
            h('button', {
                className: 'player-edit-btn',
                attrs: {
                    'data-action': 'role-modal-edit-player',
                    'data-role-index': String(roleIndex),
                    'data-player-index': String(pIdx),
                    'title': 'Редактировать'
                }
            }, ['✏️'])
        );

        if (pIdx > 0) {
            actions.push(
                h('button', {
                    className: 'player-edit-btn',
                    attrs: {
                        'data-action': 'role-modal-move-player',
                        'data-role-index': String(roleIndex),
                        'data-player-index': String(pIdx),
                        'data-direction': 'up',
                        'title': 'Переместить вверх'
                    }
                }, ['↑'])
            );
        }
        if (pIdx < players.length - 1) {
            actions.push(
                h('button', {
                    className: 'player-edit-btn',
                    attrs: {
                        'data-action': 'role-modal-move-player',
                        'data-role-index': String(roleIndex),
                        'data-player-index': String(pIdx),
                        'data-direction': 'down',
                        'title': 'Переместить вниз'
                    }
                }, ['↓'])
            );
        }

        actions.push(
            h('button', {
                className: 'player-remove-btn',
                attrs: {
                    'data-action': 'role-modal-remove-player',
                    'data-role-index': String(roleIndex),
                    'data-nickname': p.nickname,
                    'title': 'Удалить игрока'
                }
            }, ['✕'])
        );

        const item = h('div', { className: 'edit-player-list-item' }, [
            h('div', { className: 'player-info' }, [
                h('span', { className: 'player-nickname' }, [escapeHtml(p.nickname)]),
                ...(p.discord ? [h('span', { className: 'player-role-name' }, [escapeHtml(p.discord)])] : []),
            ]),
            ...actions,
        ]);
        container.appendChild(item);
    }

    if (count === 0) {
        container.appendChild(
            h('span', { style: { color: 'var(--color-text-muted)', fontSize: 'var(--font-size-xs)' } },
                [query ? 'Ничего не найдено' : 'Нет игроков']
            )
        );
    }
}

function roleModalEditPlayer(roleIndex, playerIndex) {
    const player = store.staffRoles[roleIndex]?.players?.[playerIndex];
    if (!player) return;

    document.getElementById('editRolePlayerIdx').value = String(playerIndex);
    document.getElementById('roleAddPlayerNickname').value = player.nickname;
    document.getElementById('roleAddPlayerDiscord').value = player.discord || '';

    const btn = document.getElementById('roleAddPlayerBtn');
    btn.textContent = '💾 Сохранить';
    btn.dataset.action = 'role-modal-save-player';
}

function roleModalCancelEdit() {
    document.getElementById('editRolePlayerIdx').value = '-1';
    document.getElementById('roleAddPlayerNickname').value = '';
    document.getElementById('roleAddPlayerDiscord').value = '';
    const btn = document.getElementById('roleAddPlayerBtn');
    btn.textContent = '➕ Добавить';
    btn.dataset.action = 'role-add-player';
}

async function roleModalSavePlayers(roleIndex) {
    const role = store.staffRoles[roleIndex];
    if (!role) return;
    try {
        const res = await fetchWithAbort(`${BACKEND_URL}/staff/role`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'include',
            body: JSON.stringify({ roleIndex, name: role.name, color: role.color, players: role.players })
        }, 'role-save-players');
        if (!res.ok) { const err = await parseJsonResponse(res); throw new Error(err.error || 'Ошибка сохранения'); }
        await loadStaffRoles();
    } catch (e) {
        showToast(e.message, 'error');
    }
}

async function roleModalMovePlayer(roleIndex, playerIndex, direction) {
    const role = store.staffRoles[roleIndex];
    if (!role || !role.players) return;

    const targetIdx = direction === 'up' ? playerIndex - 1 : playerIndex + 1;
    if (targetIdx < 0 || targetIdx >= role.players.length) return;

    [role.players[playerIndex], role.players[targetIdx]] = [role.players[targetIdx], role.players[playerIndex]];

    await roleModalSavePlayers(roleIndex);
    renderRoleModalPlayerList(roleIndex);
}

async function roleModalSavePlayer() {
    const roleIndex = parseInt(document.getElementById('editRoleIndex')?.value || '-1');
    const playerIdx = parseInt(document.getElementById('editRolePlayerIdx')?.value || '-1');
    if (roleIndex < 0 || playerIdx < 0) return;

    const role = store.staffRoles[roleIndex];
    if (!role || !role.players || !role.players[playerIdx]) return;

    const nicknameInput = document.getElementById('roleAddPlayerNickname');
    const discordInput = document.getElementById('roleAddPlayerDiscord');
    const nickname = nicknameInput.value.trim();
    if (!nickname) { showToast('Введите ник игрока', 'error'); return; }
    const discord = discordInput.value.trim() || '';

    const oldNickname = role.players[playerIdx].nickname;
    role.players[playerIdx].nickname = nickname;
    role.players[playerIdx].discord = discord;

    try {
        const res = await fetchWithAbort(`${BACKEND_URL}/staff/role`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'include',
            body: JSON.stringify({ roleIndex, name: role.name, color: role.color, players: role.players })
        }, 'role-save-player-edit');
        if (!res.ok) { const err = await parseJsonResponse(res); throw new Error(err.error || 'Ошибка сохранения'); }
        if (oldNickname !== nickname) {
            await setPlayerTierSilent(nickname, getPlayerTier(oldNickname));
        }
        await loadStaffRoles();
        roleModalCancelEdit();
        renderRoleModalPlayerList(roleIndex);
        showToast(`Игрок «${escapeHtml(nickname)}» обновлён`, 'success');
    } catch (e) {
        role.players[playerIdx].nickname = oldNickname;
        role.players[playerIdx].discord = discord;
        showToast(e.message, 'error');
    }
}

async function addPlayerFromRoleModal() {
    const roleIndex = parseInt(document.getElementById('editRoleIndex')?.value || '-1');
    if (roleIndex < 0 || roleIndex >= store.staffRoles.length) {
        showToast('Ошибка: роль не найдена', 'error');
        return;
    }

    const nicknameInput = document.getElementById('roleAddPlayerNickname');
    const discordInput = document.getElementById('roleAddPlayerDiscord');
    const nickname = nicknameInput?.value?.trim();
    if (!nickname) {
        showToast('Введите ник игрока', 'error');
        return;
    }

    const discord = discordInput?.value?.trim() || '';
    const role = store.staffRoles[roleIndex];

    roleModalCancelEdit();

    try {
        const res = await fetchWithAbort(`${BACKEND_URL}/staff/add`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'include',
            body: JSON.stringify({ roleIndex, nickname, discord })
        }, 'role-add-player');

        if (!res.ok) {
            const err = await parseJsonResponse(res);
            throw new Error(err.error || 'Ошибка добавления игрока');
        }

        await Promise.all([ loadStaffRoles(), loadStaffTiers() ]);
        nicknameInput.value = '';
        discordInput.value = '';
        renderRoleModalPlayerList(roleIndex);
        renderEditPlayerList();
        showToast(`Игрок «${escapeHtml(nickname)}» добавлен в роль «${escapeHtml(role.name)}»`, 'success');
    } catch (e) {
        if (!isAbortError(e)) {
            console.error('Ошибка добавления игрока:', e);
            showToast(e.message, 'error');
            if (e.message.includes('401')) logoutHost();
        }
    }
}

async function editAddPlayer() {
    const nicknameInput = document.getElementById('editPlayerNickname');
    const discordInput = document.getElementById('editPlayerDiscord');
    const roleSelect = document.getElementById('editPlayerRole');
    const key = document.getElementById('editPlayerKey').value;

    const nickname = nicknameInput.value.trim();
    if (!nickname) {
        showToast('Введите ник игрока', 'error');
        return;
    }

    const roleIndex = parseInt(roleSelect.value);
    if (isNaN(roleIndex) || roleIndex < 0 || roleIndex >= store.staffRoles.length) {
        showToast('Выберите роль', 'error');
        return;
    }

    const discord = discordInput.value.trim() || '';

    if (key) {
        const [oldRoleIdx, playerIdx] = key.split(':').map(Number);
        const oldRole = store.staffRoles[oldRoleIdx];
        const role = store.staffRoles[roleIndex];

        if (oldRoleIdx !== roleIndex) {
            if (oldRole && oldRole.players) {
                oldRole.players.splice(playerIdx, 1);
            }
            if (!role.players) role.players = [];
            role.players.push({ nickname, discord });
        } else {
            if (role.players && role.players[playerIdx]) {
                role.players[playerIdx].nickname = nickname;
                role.players[playerIdx].discord = discord;
            }
        }

        try {
            await saveStaffRoles();
            await loadStaffTiers();
            cancelEditPlayer();
            renderEditPlayerList();
            showToast(`Игрок «${escapeHtml(nickname)}» обновлён`, 'success');
        } catch (e) {
            showToast(e.message, 'error');
        }
        return;
    }

    const role = store.staffRoles[roleIndex];

    try {
        const res = await fetchWithAbort(`${BACKEND_URL}/staff/add`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'include',
            body: JSON.stringify({ roleIndex, nickname, discord })
        }, 'edit-add-player');

        if (!res.ok) {
            const err = await parseJsonResponse(res);
            throw new Error(err.error || 'Ошибка добавления игрока');
        }

        await Promise.all([
            loadStaffRoles(),
            loadStaffTiers()
        ]);
        nicknameInput.value = '';
        discordInput.value = '';
        renderEditPlayerList();
        showToast(`Игрок «${escapeHtml(nickname)}» добавлен в роль «${escapeHtml(role.name)}»`, 'success');
    } catch (e) {
        if (!isAbortError(e)) {
            console.error('Ошибка добавления игрока:', e);
            showToast(e.message, 'error');
            if (e.message.includes('401')) {
                logoutHost();
            }
        }
    }
}
