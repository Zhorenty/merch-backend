# API MERCH

Справочник HTTP-ручек бэкенда карты лояльности. Источник правды — этот сервер, не Wallet.

База локально: `http://localhost:8080`. В проде — HTTPS за прокси (`API_BASE_URL`).

Машинный контракт: [`openapi.yaml`](../openapi.yaml). Бизнес-правила: [`TZ-MERCH-WALLET.md`](./TZ-MERCH-WALLET.md).

- [Общие правила](#общие-правила)
- [Ошибки](#ошибки)
- [Служебные](#служебные)
- [Выдача карты](#выдача-карты)
- [Касса](#касса)
- [Админ](#админ)
- [Apple PassKit](#apple-passkit)

---

## Общие правила

| | |
|---|---|
| Тело | JSON, UTF-8, лимит **1 МБ** |
| Касса | `Authorization: Bearer {staff_jwt}`, секрет `CASHIER_JWT_SECRET`, TTL **12 ч** |
| Админ | отдельный JWT, секрет `ADMIN_JWT_SECRET`, только роль `admin` |
| Публичные | `/public/*`, `/card/add` — без staff-токена |
| Cookie выдачи | `merch_cid` = `customer_id` (HttpOnly, SameSite=Lax) |
| CORS | только `PUBLIC_BASE_URL` / `API_BASE_URL` или список `CORS_ORIGINS` |
| HSTS | если TLS или `X-Forwarded-Proto: https` |
| Rate limit | `/public/enroll` ~10/мин, `/cashier/lookup` ~60/мин (по IP) |

Роли staff: `cashier`, `shift_lead`, `admin`.

Идентификаторы:

- `customer_id` — UUID, он же Apple `serialNumber`
- `barcode` — `MCH-` + 8 символов Crockford (без I, L, O, U), это содержимое QR
- `receipt_id` — UUID с кассы, ключ идемпотентности commit/refund
- Google object id — `{issuerId}.{customer_id}`

Формула начисления (целые баллы ≥ 0):

```
points_earn = floor( (receipt_amount_rub - redeem_points * redeem_rate) * earn_percent / 100 )
```

Дефолты: `earn_percent=5`, `redeem_rate=1`, `redeem_min=100`, `redeem_max_share=50`. Параметры правятся через админку.

---

## Ошибки

```json
{ "error": { "code": "INSUFFICIENT_POINTS", "message": "Недостаточно баллов" } }
```

| code | HTTP | Когда |
|---|---|---|
| `UNAUTHORIZED` | 401 | нет/битый JWT, неверный логин, `active=false`, сессия отозвана |
| `STAFF_FORBIDDEN` | 403 | роль не позволяет операцию; чужая точка на commit |
| `CUSTOMER_BLOCKED` | 403 | карта заблокирована |
| `CUSTOMER_NOT_FOUND` | 404 | нет клиента по barcode/id |
| `DUPLICATE_RECEIPT` | 409 | тот же `receipt_id`, но другие сумма/barcode/redeem |
| `INSUFFICIENT_POINTS` | 422 | не хватает баллов (redeem или adjust в минус) |
| `BELOW_MIN_REDEEM` | 422 | списание меньше `redeem_min` (при `requested > 0`) |
| `EXCEEDS_RECEIPT_SHARE` | 422 | списание больше доли чека |
| `INVALID_REQUEST` | 422 | JSON, отрицательные числа, пустой `reason`, не-UUID `receipt_id` |
| `INTERNAL` | 500 | внутренняя ошибка |

Повтор **того же** `receipt_id` с теми же полями — **200** и исходный результат, не ошибка.

---

## Служебные

### `GET /healthz`

Без авторизации.

```json
{ "status": "ok" }
```

---

## Выдача карты

### `POST /public/enroll`

Rate limit. Cookie `merch_cid` и/или тот же телефон **не создают вторую карту**.

Имя и телефон опциональны. Телефон нормализуется (`8…` / `9…` → `+7…`).

```json
{ "name": "Анна", "phone": "+79001112233" }
```

```json
{
  "customer_id": "3f2a…",
  "barcode": "MCH-7K2P9Q4R",
  "apple_url": "http://localhost:8080/public/passes/apple/{id}.pkpass",
  "google_save_url": "",
  "add_page": "http://localhost:8080/card/add/{id}",
  "created": true
}
```

`google_save_url` пустой, пока нет `GOOGLE_ISSUER_ID` + `GOOGLE_SA_JSON`. Повтор: `created: false`, те же id/barcode.

### `GET /card/add`

HTML-форма выдачи. Если cookie `merch_cid` валиден — редирект на `/card/add/{id}`.

### `GET /card/add/{id}`

HTML-карточка. `{id}` — UUID клиента (или резолв через `loadCustomer`).

User-Agent:

| Устройство | Поведение |
|---|---|
| iPhone / iPad / iPod | кнопка «Добавить в Apple Wallet» → `.pkpass` |
| Android без Huawei/Honor/Harmony | кнопка **Save to phone** (если есть Google JWT) |
| иначе (Huawei, десктоп) | веб-карточка с QR = `barcode` |

Форс платформы: `?platform=apple|google|web`.

### `GET /public/passes/apple/{id}.pkpass`

`Content-Type: application/vnd.apple.pkpass`. `{id}` — `customer_id` или barcode. Есть вариант без суффикса `.pkpass`.

Без Apple-сертификатов zip всё равно отдаётся (stub-подпись). `If-Modified-Since` → 304.

---

## Касса

Префикс `/cashier`. JWT кассы: любой **active** staff (`cashier` / `shift_lead` / `admin`).

### `POST /cashier/login`

Без Bearer. Только `active=true`. Пароль или PIN.

```json
{ "login": "anna", "password": "secret" }
```

```json
{
  "token": "eyJ…",
  "expires_at": "2026-08-28T19:00:00Z",
  "staff": { "id": "…", "name": "Анна", "role": "cashier", "store_id": "…" }
}
```

Неактивный сотрудник → `401 UNAUTHORIZED`.

### `GET /cashier/app-version`

Без JWT.

```json
{ "min_supported": "1.0.0", "download_url": "https://…" }
```

### `POST /cashier/lookup`

Rate limit. JWT.

```json
{ "barcode": "MCH-7K2P9Q4R" }
```

Допускается UUID клиента, если barcode не найден.

```json
{
  "customer_id": "…",
  "name": "Анна",
  "points": 120,
  "redeem_min": 100,
  "redeem_rate": 1,
  "can_redeem": true
}
```

`can_redeem` = `points >= redeem_min`. Заблокированный → `CUSTOMER_BLOCKED`.

### `POST /cashier/quote-redeem`

Без записи в БД. JWT.

```json
{
  "barcode": "MCH-7K2P9Q4R",
  "receipt_amount_rub": 4500,
  "requested_points": 500
}
```

Успех:

```json
{
  "allowed": true,
  "requested_points": 500,
  "max_points": 500,
  "redeem_points": 500,
  "redeem_rub": 500,
  "earn_points": 200,
  "payable_rub": 4000,
  "current_points": 500
}
```

Отказ (всё равно 200): `allowed: false`, `code`, `reason` на русском (`BELOW_MIN_REDEEM` / `INSUFFICIENT_POINTS` / `EXCEEDS_RECEIPT_SHARE`). `requested_points: 0` всегда allowed.

Пример ТЗ: 4500 ₽, redeem 500, 5% → `earn_points = 200`.

### `POST /cashier/commit`

Атомарно: lock клиента `FOR UPDATE` → проверки redeem → `points -= redeem; points += earn` → `receipts` + ledger (строки только если redeem/earn ≠ 0) → очередь Wallet (не блокирует кассу). JWT.

```json
{
  "receipt_id": "11111111-1111-4111-8111-111111111111",
  "barcode": "MCH-7K2P9Q4R",
  "receipt_amount_rub": 4500,
  "redeem_points": 500,
  "store_id": "00000000-0000-4000-8000-000000000001"
}
```

`receipt_id` обязателен UUID. `store_id` можно опустить — берётся из JWT. Чужая точка разрешена только `admin`.

```json
{
  "receipt_id": "…",
  "customer_id": "…",
  "barcode": "MCH-…",
  "points": 200,
  "earn_points": 200,
  "redeem_points": 500,
  "idempotent_replay": false
}
```

Повтор того же чека: `200`, `idempotent_replay: true`, баллы не меняются. Покупка без списания: `redeem_points: 0`.

Без сети касса **не** должна слать commit — сервер stateless, «запоминать» чек сам не будет.

### `POST /cashier/refund`

JWT, роли **`shift_lead` или `admin`**. Иначе `STAFF_FORBIDDEN`.

```json
{ "receipt_id": "11111111-1111-4111-8111-111111111111" }
```

1. Сторно начисленных (не ниже 0).  
2. Возврат списанных на карту.  
Повтор — 200 и тот же результат.

```json
{
  "receipt_id": "…",
  "customer_id": "…",
  "points": 500,
  "idempotent_replay": false
}
```

### `POST /cashier/enroll`

JWT. То же тело/ответ, что `/public/enroll`, **без** cookie `merch_cid`. Для выдачи с экрана кассы.

---

## Админ

Префикс `/admin`. JWT админки: только `role=admin`, секрет `ADMIN_JWT_SECRET`.

### `POST /admin/login`

Тело как у кассы. Не-admin → `403 STAFF_FORBIDDEN`.

### `POST /admin/adjust`

Ручная правка баллов. `reason` обязателен. Нельзя увести баланс ниже 0.

```json
{ "barcode": "MCH-7K2P9Q4R", "delta": 50, "reason": "компенсация" }
```

Можно `customer_id` вместо `barcode`.

```json
{ "customer_id": "…", "barcode": "MCH-…", "points": 250 }
```

### Staff

`GET /admin/staff` → `{ "staff": [ { "id", "store_id", "login", "name", "role", "active" } ] }`

`POST /admin/staff` — обязательны `login`, `name`, `password`, `role`. `store_id` по умолчанию флагман `00000000-0000-4000-8000-000000000001`. `pin` опционален.

`PATCH /admin/staff/{id}` — частичное обновление. `active: false` отзывает все JWT сотрудника.

### Точки

`GET /admin/stores` → `{ "stores": [ { "ID", "Name", "Address", "CreatedAt" } ] }`  
(поля JSON — как у Go-структуры `StoreRow`.)

`POST /admin/stores` `{ "name": "ТЦ", "address": "…" }` — `name` обязателен.

`PATCH /admin/stores/{id}` `{ "name", "address" }`

`DELETE /admin/stores/{id}` → **204** без тела.

### Правила лояльности

`GET /admin/loyalty-settings` — карта ключ/значение:

| ключ | дефолт |
|---|---|
| `earn_percent` | `5` |
| `earn_round` | `down` |
| `earn_base` | `after_store_discount_before_points` |
| `earn_min_receipt` | `0` |
| `redeem_rate` | `1` |
| `redeem_min` | `100` |
| `redeem_max_share` | `50` |
| `expire_days` | пусто (= выкл., MVP) |

`PUT /admin/loyalty-settings` — JSON-объект строк, upsert, ответ как GET.

### Клиенты

`GET /admin/customers?q=` — поиск по barcode, phone, id, имени. Без `q` — последние 100.

```json
{ "customers": [ { "id", "barcode", "name", "phone", "points", "blocked" } ] }
```

`POST /admin/customers/{id}/block` → `{ "id", "blocked": true }`  
`POST /admin/customers/{id}/unblock` → `{ "id", "blocked": false }`

`{id}` — UUID клиента.

---

## Apple PassKit

База `webServiceURL`: `{API_BASE_URL}/passes/`  
Заголовок (кроме `POST /v1/log`): `Authorization: ApplePass {authenticationToken}`  
`passType` по умолчанию `pass.com.merch.loyalty`, `serial` = `customer_id`.

| Метод | Путь | Ответ |
|---|---|---|
| `POST` | `/passes/v1/devices/{deviceLibraryId}/registrations/{passType}/{serial}` | **201**, тело `{ "pushToken": "…" }` |
| `DELETE` | тот же | **200** |
| `GET` | `/passes/v1/devices/{deviceLibraryId}/registrations/{passType}?passesUpdatedSince=` | **204** или `{ "lastUpdated", "serialNumbers" }` |
| `GET` | `/passes/v1/passes/{passType}/{serial}` | `.pkpass` |
| `POST` | `/passes/v1/log` | **200**, логи без токенов |

После commit/refund/adjust сервер ставит job `wallet_update` (outbox). APNs — тихое пустое тело; без `.p8` в логе `wallet update skipped`. Google PATCH только `loyaltyPoints`; без ключей — no-op.

---

## Коды ответа (кратко)

| Ситуация | HTTP |
|---|---|
| Успех JSON | 200 |
| Регистрация устройства Apple | 201 |
| Delete store | 204 |
| Нет обновлённых passes | 204 |
| pkpass не менялся | 304 |
| Rate limit | 429 |
| OPTIONS CORS | 204 |
