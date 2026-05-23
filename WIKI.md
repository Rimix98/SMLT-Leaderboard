# SMLT Demonlist Wiki

Вся документация проекта SMLT Demonlist — лидерборда, системы Staff и каталога коллабов для Geometry Dash сообщества SMLT.

---

## 📋 Содержание

1. [О проекте](#-о-проекте)
2. [Архитектура](#-архитектура)
3. [Страницы (Frontend)](#-страницы-frontend)
4. [JavaScript (app.js)](#-javascript-appjs)
5. [Go Backend (api/index.go)](#-go-backend-apiindexgo)
6. [API Endpoints](#-api-endpoints)
7. [Firestore Database](#-firestore-database)
8. [Безопасность](#-безопасность)
9. [Развёртывание](#-развёртывание)
10. [Переменные окружения](#-переменные-окружения)
11. [Файловая структура](#-файловая-структура)
12. [CSS и темизация](#-css-и-темизация)

---

## 🎯 О проекте

SMLT Demonlist — это веб-платформа для сообщества SMLT (Geometry Dash), объединяющая:

- **Демонлист** — таблица лидеров игроков с рейтингом, рекордами и уровнями
- **Staff система** — иерархия ролей команды с цветовой кодировкой и тирами (Priority/Base/Reserve/NA)
- **Проекты** — каталог коллаборационных уровней с отслеживанием статуса и участников

**Стек:** Vanilla JS, Go (Vercel Serverless), Firestore (Google Cloud), JWT, Upstash Redis.

---

## 🏗 Архитектура

```
Browser (index.html / demonlist.html / projects.html / staff.html + app.js)
       │
       │ REST /api/* (JSON, credentials: 'include')
       ▼
Vercel Serverless Function (api/index.go)
       │
       ├──► Firestore (Google Cloud)
       │     ├── config/players        — список отслеживаемых игроков
       │     ├── config/staff          — роли, игроки, тиры
       │     ├── config/auth           — tokenVersion
       │     ├── projects/{id}         — коллабы
       │     ├── captcha/{id}          — капчи (10 мин TTL)
       │     ├── token_blacklist/{jti} — отозванные JWT (24ч TTL)
       │     └── audit_log/{id}        — логи действий админа
       │
       ├──► Upstash Redis              — распределённый rate limiting (опционально)
       │
       └──► api.demonlist.org          — внешнее API для данных игроков
```

**Принципы:**
- Backend — единая serverless функция Go, работающая как BFF (Backend For Frontend)
- Все запросы к внешнему API проксируются через сервер (защита от CORS, агрегация данных)
- Работа с Firestore через транзакции для атомарных read-modify-write
- Оптимистичные обновления на фронтенде с откатом при ошибке

---

## 📄 Страницы (Frontend)

### `index.html` — Главная

Лендинг с навигационными карточками и фоновым изображением.

**Структура:**
- Hero-секция с названием и описанием
- Карточки навигации: Рейтинг игроков, Проекты, Персонал
- Футер с копирайтом

### `demonlist.html` — Демонлист

Основная страница с табами «Топ игроков» и «Топ уровней».

**Секции:**
- **Топ игроков:** поиск по нику, таблица игроков с флагами, рейтингом, очками и рекордами
- **Топ уровней:** поиск по уровню, таблица сложнейших уровней с викторами
- **Статистика:** кол-во игроков, сумма очков, хардлейший уровень
- **По странам:** список стран с флагами и количеством игроков (клик — топ игроков страны)
- **Профиль игрока:** модалка с полной статистикой, рекордами, ссылкой на глобальный демонлист
- **Панель хоста:** управление списком игроков (добавить/удалить)

### `projects.html` — Проекты

Каталог коллаборационных уровней.

**Возможности:**
- Карточки проектов с YouTube-плеером, статусом, участниками
- Поиск по названию проекта
- Управление: создание, редактирование, удаление, перетасовка порядка
- **Построитель участников:** выбор из стафф-игроков, назначение ролей через теги
- Легенда статусов: Готов, В процессе верифа, В процессе постройки, Планируется, Заморожен, Мёртв

### `staff.html` — Персонал

Страница команды с ролями и тирами.

**Возможности:**
- Карточки ролей с цветовым баннером, списком участников (никнейм + Discord)
- Система тиров: Priority, Base, Reserve, N/A (с цветовой кодировкой)
- **Панель управления (хост):** добавление/удаление ролей, изменение цвета, смена порядка
- **Edit Panel:** слайд-панель с полным списком всех игроков по ролям, кликабельные тиры
- **Color Picker:** нативный выбор цвета + hex-ввод + 7 пресетов

### Общие элементы (все страницы)

- Кнопка смены темы (🌙/☀️) — фиксированная позиция
- Кнопка входа хоста — фиксированная позиция
- Модалка информации (Discord, контакты)
- Модалка логина хоста (пароль + капча)
- Toast-уведомления (5 сек, auto-dismiss)

---

## ⚙️ JavaScript (app.js)

Единый файл `app.js` (3084 строки) — вся клиентская логика.

### Ключевые паттерны

**`h(tag, opts, children)`** — фабрика DOM-элементов. Замена `innerHTML` для XSS-безопасности.

```js
h('div', { className: 'foo', dataset: { id: 1 } }, [
    h('span', { style: { color: 'red' } }, ['текст'])
])
```

**`store`** — центральное состояние:

```js
const store = {
    isHost: false,
    players: [],           // имена игроков для отслеживания
    allPlayers: [],        // полные данные с API
    projects: [],
    levels: { all: null, levelData: null, expanded: false, filter: '', _body: null },
    _leaderboard: { body: null, lastSig: '' },
    staffRoles: [],
    staffTiers: [],
    selectedRoleColor: '#3b82f6',
    pendingProjectParticipants: [],
};
```

**Делегирование событий** — глобальный обработчик `document` на `[data-action]` (~55 экшенов).

**Эффективный re-render** — таблицы игроков и уровней обновляют существующие DOM-узлы вместо полной перерисовки (паттерн reconcile).

**Оптимистичные обновления** — CRUD проектов и ролей меняет локальное состояние, синхронизирует с сервером, откатывает при ошибке.

### Основные функции

| Функция | Описание |
|---------|----------|
| `escapeHtml(text)` | Санитизация текста для DOM |
| `resolveCountry(input)` | Определение кода страны |
| `getFlag(c)` | Флаг (img с flagcdn.com или эмодзи 🌍) |
| `getPlayerNames()` | GET /api/players (или дефолтные имена) |
| `loadAllPlayers()` | Загрузка лидерборда (backend или прямой запрос) |
| `fetchPlayerData(name)` | Поиск игрока на api.demonlist.org |
| `fetchRecords(id)` | Рекорды игрока с api.demonlist.org |
| `renderPlayers()` | Рендер таблицы игроков (reconcile) |
| `renderStats()` | Обновление статистики |
| `renderHardestLevels()` | Рендер таблицы уровней |
| `renderCountryStats()` | Рендер списка стран |
| `showProfile(idx)` | Модалка профиля игрока |
| `showCountryTop(raw)` | Модалка топа игроков страны |
| `showLevelVictors(levelId)` | Модалка викторов уровня |
| `loadProjects()` / `renderProjects()` | Загрузка/рендер проектов |
| `saveProject()` / `deleteProject()` | CRUD проектов |
| `initParticipantBuilder()` | Построитель участников проекта |
| `loadStaffRoles()` / `renderStaffRoles()` | Загрузка/рендер Staff ролей |
| `loadStaffTiers()` / `setPlayerTier()` | Система тиров |
| `initDemonlistTabs()` | Переключение табов (игроки/уровни) |
| `verifyHost(password)` | Логин хоста (пароль + капча) |
| `logoutHost()` | Выход хоста |
| `initTheme()` / `toggleTheme()` | Тёмная/светлая тема |
| `showToast(msg, type)` | Toast-уведомление |
| `openEditPanel()` / `closeEditPanel()` | Slide-in панель управления |

### Внешние API

- `https://api.demonlist.org/leaderboard/user/list?search=...` — данные игрока
- `https://api.demonlist.org/user/record/list?user_id=...` — рекорды игрока
- `https://flagcdn.com/w20/{code}.png` — изображения флагов

---

## 🔧 Go Backend (api/index.go)

Единый файл `api/index.go` (2188 строк) — serverless-функция для Vercel.

### Пакеты и зависимости

| Пакет | Назначение |
|-------|-----------|
| `cloud.google.com/go/firestore` | Firestore клиент |
| `firebase.google.com/go` | Firebase Admin SDK |
| `github.com/golang-jwt/jwt/v5` | JWT (HS256) |
| `golang.org/x/crypto` | bcrypt |
| `github.com/microcosm-cc/bluemonday` | HTML санитизация |
| `github.com/mojocn/base64Captcha` | Генерация капчи |

### Middleware цепочка

```
Request → gzip → Security Headers → CORS → Rate Limit → Auth (JWT) → CSRF → Handler
```

### Инициализация (init)

- Firestore клиент (через `sync.Once`)
- JWT секреты (основной + ключи ротации: `JWT_SECRET`, `JWT_SECRET_2`, ...)
- Rate limiter (Upstash Redis или in-memory)
- Капча сторадж (Firestore или in-memory)

### Rate Limiting

**Интерфейс:**
```go
type rateLimiter interface {
    Allow(ip string) (bool, error)
}
```

**Реализации:**
- `memoryLimiter` — in-process с горутиной очистки
- `upstashLimiter` — Redis REST API (SET NX / INCR / GET)

**Лимиты:**
- Общие эндпоинты: 30–60 запросов/мин/IP
- Логин: 5 запросов/мин/IP (дополнительно Firestore-бэкап)
- Капча генерируется при каждом запросе

### JWT Аутентификация

**Структура токена:**
```json
{
    "admin": true,
    "exp": "24h",
    "iat": "now",
    "ver": "tokenVersion",
    "jti": "uuid"
}
```

**Защита:**
- Cookie `auth_token`: HttpOnly, Secure, SameSite=Strict, Path=/
- Подпись HMAC-SHA256 с поддержкой ротации ключей (header `kid`)
- Проверка `ver` против Firestore `config/auth.tokenVersion` (кэш 60s)
- Проверка `jti` по blacklist в Firestore
- CSRF double-submit cookie: `csrf_token` cookie + `X-CSRF-Token` header

### Основные хендлеры

| Функция | Path | Описание |
|---------|------|----------|
| `handleCaptcha` | GET `/api/captcha` | Генерация капчи |
| `handleGetCSRFToken` | GET `/api/csrf-token` | CSRF токен |
| `handleVerify` | GET `/api/verify` | Проверка JWT |
| `handleLogin` | POST `/api/login` | Вход хоста |
| `handleLogout` | POST `/api/logout` | Выход хоста |
| `handleLeaderboard` | GET `/api/leaderboard` | Данные лидерборда |
| `handleGetPlayers` | GET `/api/players` | Список игроков |
| `handleSavePlayers` | POST `/api/players/save` | Сохранение игроков |
| `handleDeletePlayer` | POST `/api/players/delete` | Удаление игрока |
| `handleGetProjects` | GET `/api/projects` | Список проектов |
| `handleSaveProjects` | POST `/api/projects/save` | Сохранение проектов |
| `handleGetStaff` | GET `/api/staff` | Staff роли |
| `handleStaffAdd` | POST `/api/staff/add` | Добавить в роль |
| `handleCreateStaffRole` | POST `/api/staff/role` | Создать роль |
| `handleUpdateStaffRole` | PUT `/api/staff/role` | Обновить роль |
| `handleDeleteStaffRole` | DELETE `/api/staff/role` | Удалить роль |
| `handleStaffRemove` | POST `/api/staff/remove` | Удалить из роли |
| `handleReorderStaffRoles` | POST `/api/staff/reorder` | Сменить порядок |
| `handleGetStaffTiers` | GET `/api/staff/tiers` | Тиры игроков |
| `handleSetStaffTier` | POST `/api/staff/tier` | Установить тир |

### Валидация

- `validateProjectID` — латиница, цифры, дефис, слэш (1-64 символа)
- `validateNickname` — латиница, цифры, подчёркивание, дефис (1-32)
- `validateDiscord` — username#0000 формат
- `validateRoleName` — любые символы (1-32)
- `validateDemonlistURL` — только `https://api.demonlist.org/*`

### Аудит

Все изменения состояния записываются в Firestore `audit_log`:
```go
type AuditEntry struct {
    Action    string    `firestore:"action"`
    AdminIp   string    `firestore:"adminIp"`
    Details   string    `firestore:"details"`
    CreatedAt time.Time `firestore:"createdAt"`
}
```

---

## 🌐 API Endpoints

| Метод | Путь | Auth | CSRF | Rate Limit | Описание |
|-------|------|:----:|:----:|:----------:|----------|
| `GET` | `/api/captcha` | - | - | 30/мин | Капча |
| `GET` | `/api/csrf-token` | - | - | 30/мин | CSRF токен |
| `GET` | `/api/verify` | - | - | 60/мин | Проверка JWT |
| `POST` | `/api/login` | - | - | 5/мин | Логин |
| `POST` | `/api/logout` | - | - | 5/мин | Логаут |
| `GET` | `/api/leaderboard` | - | - | 30/мин | Лидерборд |
| `GET` | `/api/players` | - | - | 60/мин | Список имён |
| `POST` | `/api/players/save` | ✅ | ✅ | 30/мин | Сохр. имён |
| `POST` | `/api/players/delete` | ✅ | ✅ | 30/мин | Удал. имя |
| `GET` | `/api/projects` | - | - | 60/мин | Проекты |
| `POST` | `/api/projects/save` | ✅ | ✅ | 30/мин | Сохр. проекты |
| `GET` | `/api/staff` | - | - | 60/мин | Staff |
| `POST` | `/api/staff/add` | ✅ | ✅ | 30/мин | Добавить в роль |
| `POST` | `/api/staff/remove` | ✅ | ✅ | 30/мин | Удалить из роли |
| `POST` | `/api/staff/reorder` | ✅ | ✅ | 30/мин | Порядок ролей |
| `POST` | `/api/staff/role` | ✅ | ✅ | 30/мин | Создать роль |
| `PUT` | `/api/staff/role` | ✅ | ✅ | 30/мин | Обновить роль |
| `DELETE` | `/api/staff/role` | ✅ | ✅ | 30/мин | Удалить роль |
| `GET` | `/api/staff/tiers` | - | - | 60/мин | Тиры |
| `POST` | `/api/staff/tier` | ✅ | ✅ | 30/мин | Установить тир |

---

## 🔥 Firestore Database

### Коллекции

```
config/
├── players              { players: [{ name: "..." }] }
├── staff                { roles: [...], gp_tiers: [...], deco_tiers: [...] }
└── auth                 { tokenVersion: int64 }

projects/{projectId}
    name, videoId, id, comment, status, verifier, participants[]

captcha/{captchaId}
    value, expiresAt                          (TTL: 10 мин)

token_blacklist/{jti}
    blacklistedAt, expiresAt                  (TTL: 24 ч)

rate_limits/login:{ipHash}
    count, resetAt

audit_log/{autoId}
    action, adminIp, details, createdAt
```

### Cтруктура staff документа

```json
{
  "roles": [
    {
      "name": "Основа",
      "color": "#6d0b0d",
      "players": [
        { "nickname": "Player1", "discord": "user#0000" }
      ]
    }
  ],
  "gp_tiers": [
    { "nickname": "Player1", "tier": "priority" }
  ],
  "deco_tiers": [
    { "nickname": "Player2", "tier": "base" }
  ]
}
```

**Тиры (цвета):**
- `priority` — #00ffff (Cyan)
- `base` — #6d0b0d (Тёмно-красный)
- `reserve` — #540b6d (Фиолетовый)
- `na` — #888888 (Серый)

---

## 🔒 Безопасность

### Аутентификация
- Пароль хранится в bcrypt хеше (`ADMIN_HASH`)
- JWT HS256 с 24h expiry
- HttpOnly, Secure, SameSite=Strict cookies
- Поддержка ротации JWT секретов (header `kid`)
- Версионирование токенов (`tokenVersion` для массовой инвалидации)
- Blacklist отдельных JWT по `jti`

### CSRF
- Double-submit cookie паттерн
- Cookie `csrf_token` + header `X-CSRF-Token`

### Rate Limiting
- 30–60 запросов/мин на эндпоинт
- 5 запросов/мин на логин (с капчей)
- Upstash Redis в production, in-memory fallback
- Хеширование IP (SHA-256 + соль)

### Input Validation
- `DisallowUnknownFields` при парсинге JSON
- 1 MB лимит тела запроса
- bluemonday HTML санитизация
- Regex валидация ID, ников, Discord, URL
- Проверка URL внешнего API (защита от SSRF)

### Фронтенд
- Полный отказ от `innerHTML` — используется `h()` фабрика
- `escapeHtml()` для пользовательского текста
- `rel="noopener noreferrer"` на внешних ссылках
- Content Security Policy в заголовках

### Security Headers (Go)
```
Strict-Transport-Security: max-age=31536000; includeSubDomains
X-Content-Type-Options: nosniff
X-Frame-Options: DENY
Content-Security-Policy: default-src 'self';...
Referrer-Policy: strict-origin-when-cross-origin
```

---

## 🚀 Развёртывание

### Требования
- Go 1.26+
- Аккаунт Vercel + Vercel CLI
- Firebase проект с Firestore
- (Рекомендуется) Upstash Redis

### Локальная разработка

```bash
# 1. Клонировать
git clone https://github.com/Rimix98/SMLT-Leaderboard.git
cd SMLT-Leaderboard

# 2. Установить переменные окружения
cp .env.example .env.local
# Заполнить ADMIN_HASH, JWT_SECRET, FIREBASE_CREDENTIALS

# 3. Запустить локально
# Фронтенд: просто открыть Frontend/*.html через любой HTTP сервер
# Бэкенд:
cd api
go run .
```

### Деплой на Vercel

```bash
vercel --prod
```

Vercel автоматически:
- Компилирует Go в serverless функцию
- Прокси `/api/*` → Go функция
- Сервит статику из `Frontend/`

### Переменные окружения (Vercel)

Установить в Vercel Dashboard → Project Settings → Environment Variables:
- `ADMIN_HASH` — bcrypt хеш пароля
- `JWT_SECRET` — строка ≥32 символов
- `FIREBASE_CREDENTIALS` — JSON сервисного аккаунта Firebase
- `UPSTASH_REDIS_REST_URL` — (рекомендуется) URL Redis REST
- `UPSTASH_REDIS_REST_TOKEN` — (рекомендуется) токен Redis

---

## 🔐 Переменные окружения

| Переменная | Обязательна | Описание |
|-----------|:----------:|----------|
| `ADMIN_HASH` | ✅ | bcrypt хеш пароля хоста |
| `JWT_SECRET` | ✅ | Ключ подписи JWT (≥32 символа) |
| `FIREBASE_CREDENTIALS` | ✅ | JSON сервисного аккаунта Firebase |
| `UPSTASH_REDIS_REST_URL` | ❌ | URL Upstash Redis для rate limiting |
| `UPSTASH_REDIS_REST_TOKEN` | ❌ | Токен Upstash Redis |
| `TRUST_PROXY` | ❌ | Доверять X-Forwarded-For (авто на Vercel) |

---

## 📁 Файловая структура

```
SMLT-Demonlist/
├── api/
│   └── index.go              # Go serverless backend (2188 строк)
├── Frontend/
│   ├── index.html            # Главная страница
│   ├── demonlist.html        # Демонлист (лидерборд + уровни)
│   ├── projects.html         # Каталог проектов
│   ├── staff.html            # Staff страница
│   ├── app.js                # Вся фронтенд логика (3084 строки)
│   ├── styles.css            # Все стили (2119 строк)
│   ├── Background_Image_DarkMode.png
│   ├── Background_Image_WhiteMode.png
│   └── js/                   # (пусто)
├── docs/
│   └── screenshots/          # Скриншоты (содержит .gitkeep)
├── Secret/                   # Исключён из git
│   ├── .env.local
│   └── serviceAccountKey.json
├── coverage/                 # (пусто)
├── .vscode/
│   └── settings.json
├── .vercel/
│   └── project.json
├── .env.example              # Шаблон переменных окружения
├── .gitignore
├── go.mod                    # Go модуль + зависимости
├── go.sum
├── vercel.json               # Vercel конфиг (rewrites)
├── README.md
├── SECURITY.md
├── WIKI.md                   # Этот файл
├── favicon.ico
└── ...
```

---

## 🎨 CSS и темизация

### Дизайн-система

Вся стилизация через CSS custom properties с переключением темы через атрибут `data-theme` на `<html>`.

**Тёмная тема (по умолчанию):**
- Фон: `#0a0a0a`
- Поверхности: `#141414` / `#1a1a1a` / `#242424`
- Границы: `#2a2a2a`
- Текст: white / `#a0a0a0` / `#666`
- Акцент: `#3b82f6` (синий)

**Светлая тема:**
- Фон: `#f5f5f5`
- Поверхности: white / `#f0f0f0` / `#e5e5e5`
- Границы: `#d0d0d0`
- Текст: `#1a1a1a` / `#4a4a4a` / `#7a7a7a`

### Брейкпоинты

| breakpoint | изменения |
|-----------|-----------|
| ≤768px | Шрифт 14px, стековый header, скрытые заголовки таблиц, одинарная колонка |
| ≤480px | Шрифт 13px, полная ширина модалок, toast на всю ширину |

### Ключевые CSS классы

| Класс | Назначение |
|-------|-----------|
| `.btn-primary` / `.btn-secondary` / `.btn-danger` / `.btn-success` | Кнопки |
| `.modal-overlay` / `.modal` / `.modal-lg` | Модальные окна |
| `.player-row` / `.rank-1` / `.rank-2` / `.rank-3` | Строки лидерборда |
| `.status-ready` / `.status-verifying` / `.status-building` / `.status-planned` / `.status-frozen` / `.status-dead` | Статусы проектов |
| `.project-card` / `.project-video` | Карточки проектов |
| `.staff-role-card` / `.staff-role-header` / `.staff-role-visual` | Карточки ролей |
| `.tier-player-badge` / `.edit-player-tier-badge` | Бейджи тиров |
| `.edit-panel` / `.edit-panel-overlay` | Slide-in панель |
| `.toast` / `.toast-error` / `.toast-success` / `.toast-info` | Уведомления |
| `.country-item` / `.country-list` | Список стран |
| `.demonlist-tab-header` / `.demonlist-tab-btn` / `.demonlist-tab-content` | Табы |
| `.leaderboard-section` / `.leaderboard-table` / `.table-header` | Таблицы |

---

## 📦 Зависимости (Go)

```
cloud.google.com/go/firestore v1.22.0
firebase.google.com/go v3.13.0+incompatible
github.com/golang-jwt/jwt/v5 v5.3.1
golang.org/x/crypto v0.52.0
google.golang.org/api v0.279.0
```

Транзитивные (всего ~71 зависимость): gRPC, protobuf, OpenTelemetry, Google Cloud SDK и др.

---

## 👥 Контакты

- **Discord сервер:** discord.gg/VK56W7ZzdA
- **GitHub:** https://github.com/Rimix98/SMLT-Leaderboard
