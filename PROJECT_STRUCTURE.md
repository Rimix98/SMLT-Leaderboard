# 📁 Структура проекта SMLT Demonlist

## Обзор

```
SMLT-Demonlist/
├── api/                    # Go backend (Vercel serverless)
│   └── index.go           # Точка входа, все handlers
│
├── Frontend/              # Веб-интерфейс (Vanilla JS)
│   ├── index.html         # Главная страница
│   ├── demonlist.html     # Лидерборд с игроками
│   ├── projects.html      # Коллабы и проекты
│   ├── app.js             # Основная логика приложения
│   └── styles.css         # Стили
│
├── Secret/                # 🔐 Секреты (не в git)
│   └── serviceAccountKey.json  # Firebase credentials
│
├── tools/                 # Утилиты
│   └── hash.go           # Генератор hash для пароля
│
├── docs/                  # Документация
│   └── screenshots/       # Скриншоты приложения
│
├── coverage/              # Тесты (пусто)
│
├── go.mod                 # Go модули
├── go.sum                 # Go зависимости
├── vercel.json            # Конфиг Vercel
├── README.md              # Основная документация
└── PROJECT_STRUCTURE.md   # Этот файл
```

---

## 🔧 Backend (api/index.go)

### Архитектура

```
Go Handler (Vercel)
│
├── Rate Limiting (Upstash Redis)
│
├── Auth (JWT + HttpOnly cookies)
│
├── Firestore (Google Firebase)
│
└── External API (demonlist.org)
```

### API Routes

| Метод | Endpoint | Аутентификация | Назначение |
|-------|----------|---|---|
| `POST` | `/api/login` | ❌ | Выдача JWT токена |
| `POST` | `/api/logout` | ✅ | Удаление токена |
| `GET` | `/api/auth/verify` | ✅ | Проверка аутентификации |
| `GET` | `/api/leaderboard` | ❌ | Получить топ игроков + рекорды |
| `GET` | `/api/players` | ❌ | Список имён игроков |
| `POST` | `/api/players` | ✅ | Обновить список игроков |
| `DELETE` | `/api/players` | ✅ | Удалить игрока (транзакция) |
| `GET` | `/api/projects` | ❌ | Получить все коллабы |
| `POST` | `/api/projects` | ✅ | Сохранить коллабы |

### Главные компоненты

#### 1. **Аутентификация**
- JWT токены с сроком на 24 часа
- HttpOnly cookies (защита от XSS)
- Переменные окружения:
  - `JWT_SECRET` — ключ подписи
  - `ADMIN_HASH` — bcrypt хеш пароля

#### 2. **Rate Limiting** (Upstash Redis)
- Лимиты:
  - **60 запросов/мин** — обычные эндпоинты
  - **10 запросов/мин** — `/api/login` (защита от brute-force)
- Распределённый (работает между инстансами Vercel)

#### 3. **Firestore Database**
Коллекции:
```
firestore/
├── config/
│   └── players (array)  # Список имён на демонлисте
│
└── projects (documents)
    ├── id
    ├── name
    ├── videoId
    ├── comment
    ├── status (активен/завершён)
    ├── verifier
    └── participants (array)
```

#### 4. **Внешнее API (demonlist.org)**
```
GET /leaderboard/user/list?search={name}  → User info + ID
GET /user/record/list?user_id={id}        → Records stats
```

**Улучшения:**
- ✅ Retry логика (3 попытки)
- ✅ Проверка HTTP статуса
- ✅ Валидация JSON
- ✅ Логирование ошибок
- ✅ Таймауты запросов (10 сек)

#### 5. **Конкурентность**
- ✅ Firestore транзакции (ACID гарантии)
- ✅ Semaphore для параллельных API запросов (8 горутин одновременно)
- ✅ Защита от race condition в `handleDeletePlayer`

---

## 🎨 Frontend (HTML/JS/CSS)

### Страницы

#### **index.html** — Главная
- О сообществе SMLT
- Ссылка на Discord
- Форма входа для админа

#### **demonlist.html** — Лидерборд
- Таблица топ игроков
- Рекорды на демонах
- Поиск по нику
- Фильтры (страна, сложность)

#### **projects.html** — Коллабы
- Список текущих проектов
- Роли: автор, верификатор, участники
- Статус: идеи → WIP → завершён
- Редактирование (для админа)

### Логика (app.js)

```javascript
API calls:
├── fetchLeaderboard()     // Получить игроков
├── fetchProjects()        // Получить коллабы
├── login(password)        // Аутентификация
├── saveProjects(data)     // Сохранить коллабы (админ)
└── deletePlayer(name)     // Удалить игрока (админ)

UI Updates:
├── renderLeaderboard()    // Отрисовать таблицу
├── renderProjects()       // Отрисовать коллабы
└── showAdminPanel()       // Показать управление (админ)
```

---

## 📊 Типы данных

### Firestore Documents

```go
// Player (в массиве config/players)
type Player struct {
    Name string `json:"name"`
}

// Project (коллекция projects)
type Project struct {
    Name         string   `json:"name"`
    VideoID      string   `json:"videoId"`
    ID           string   `json:"id"`
    Comment      string   `json:"comment"`
    Status       string   `json:"status"`        // "active"|"completed"|"idea"
    Verifier     string   `json:"verifier"`
    Participants []string `json:"participants"` // Ники игроков
}

// API Response (demonlist.org)
type FullPlayerData struct {
    Name    string      `json:"name"`
    Data    interface{} `json:"data"`           // User info
    Records interface{} `json:"records"`        // Recs stats
}
```

---

## 🚀 Деплой

### Vercel Configuration (vercel.json)

```json
{
  "rewrites": [
    { "source": "/api/:path*", "destination": "/api" },
    { "source": "/((?!api).*)", "destination": "/Frontend/$1" }
  ]
}
```

**Маршруты:**
- `/api/*` → Go обработчик (`api/index.go`)
- Все остальное → Фронтенд (`Frontend/`)

### Переменные окружения

| Переменная | Назначение |
|-----------|-----------|
| `JWT_SECRET` | Подпись JWT токенов |
| `ADMIN_HASH` | bcrypt хеш пароля админа |
| `FIREBASE_CREDENTIALS` | JSON ключ Firebase (переменная окружения) |
| `UPSTASH_REDIS_REST_URL` | Redis для rate limiting |
| `UPSTASH_REDIS_REST_TOKEN` | Токен Redis |
| `TRUST_PROXY` | `true` — для Vercel (IP за прокси) |

---

## 🔒 Безопасность

### Реализовано

✅ **Authentication**
- JWT токены (24h TTL)
- HttpOnly cookies (защита от JavaScript доступа)
- bcrypt пароль хеш

✅ **Rate Limiting**
- Distributed (Upstash Redis)
- Per-IP в окне 1 минута
- Более строгие лимиты для логина

✅ **Валидация**
- Max body size: 1MB
- Reject unknown JSON fields
- Type-safe Firestore structs

✅ **Concurrency**
- Firestore транзакции (ACID)
- Защита от race conditions

✅ **HTTP Headers**
- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: DENY`
- `SameSite=Strict` cookies

✅ **API Resilience**
- Retry логика для внешних API
- Timeout на все запросы
- Graceful degradation при ошибках

---

## 📈 Производительность

### Оптимизации

1. **Параллельные запросы**
   - 8 одновременных горутин для fetch'а игроков
   - Semaphore для управления нагрузкой

2. **Кэширование**
   - Default players list (если Firestore недоступна)
   - В браузере: localStorage для токенов

3. **Коннекшены**
   - Reuse HTTP client
   - TCP connection pooling

4. **Таймауты**
   - 10 сек на API запросы
   - 1 мин на rate limit окно

---

## 🛠️ Для разработки

### Установка

```bash
# Clone
git clone https://github.com/smlt-demonlist/backend.git
cd SMLT-Demonlist

# Go dependencies
go mod download

# Local dev
vercel dev
```

### Генерация пароля

```bash
go run tools/hash.go -password "your_password"
# Скопировать bcrypt hash в ADMIN_HASH
```

### Тестирование API

```bash
# Логин
curl -X POST http://localhost:3000/api/login \
  -H "Content-Type: application/json" \
  -d '{"password":"test"}'

# Получить игроков
curl http://localhost:3000/api/players

# Получить демонлист (может быть медленно)
curl http://localhost:3000/api/leaderboard
```

---

## 📝 Изменения последнего деплоя

✅ **Исправлены критические баги:**
1. Race condition в `handleDeletePlayer` → Firestore транзакции
2. Слабая обработка API ошибок → Retry + валидация
3. Unsafe `users[0]` fallback → Strict matching или пусто

---

## 📚 Дополнительно

- **Firebase Console**: https://console.firebase.google.com
- **Demonlist API**: https://api.demonlist.org
- **Vercel Docs**: https://vercel.com/docs
- **Discord**: https://discord.gg/VK56W7ZzdA
