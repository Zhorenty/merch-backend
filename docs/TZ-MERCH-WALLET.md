# ТЗ: карта лояльности MERCH

Apple Wallet + Google Wallet + касса на Flutter (APK для сотрудников)

| | |
|---|---|
| Версия | 1.0 |
| Дата | 27.08.2026 |
| Заказчик | магазин одежды **MERCH** |
| Статус | к согласованию бизнес-правил в §3 |

---

## 1. Цель и границы

### 1.1. Цель

Клиент носит карту MERCH в системном кошельке телефона (Apple Wallet / Google Wallet). На кассе сотрудник сканирует QR с карты в **служебном Flutter-приложении**, начисляет или списывает баллы. Баланс на карте клиента обновляется без установки клиентского приложения.

### 1.2. Входит в MVP

- Выпуск персональной карты лояльности в Apple Wallet и Google Wallet
- QR на карте = идентификатор клиента (не баланс)
- Начисление и списание баллов из приложения кассира
- История операций, роли сотрудников, несколько точек
- Обновление баланса на уже установленной карте
- Страница выдачи карты клиенту (ссылка / QR на стойке, в чеке, в переписке)

### 1.3. Не входит в MVP

- Клиентское приложение в App Store / Google Play / RuStore
- NFC / Apple VAS / Google Smart Tap («приложил как пластик»)
- Оплата через Apple Pay / Google Pay
- Интернет-магазин, доставка, размерная сетка, каталог
- Продажа баллов, лотерея, случайный бонус
- Интеграция с 1С / Эвотор / iiko (закладывается API, коннектор — этап 2)
- iOS-сборка кассы (сотрудникам раздаётся **только APK**, то есть Android)

### 1.4. Принцип

**Источник правды — сервер MERCH, не Wallet.**

Wallet только показывает баланс и QR. Условия списания проверяет бэкенд по запросу кассы.

---

## 2. Роли и устройства

| Роль | Где работает | Что делает |
|---|---|---|
| Клиент | iPhone Wallet / Google Wallet / страница выдачи | Получает карту, показывает QR на кассе |
| Кассир | Flutter APK на Android | Сканирует QR, начисляет, списывает, видит баланс |
| Старший смены | то же + расширенные операции | Отмена ошибочной операции в рамках смены |
| Админ | веб-админка (минимальная) | Сотрудники, точки, правила баллов, просмотр клиентов |
| Система | API + Apple PassKit + Google Wallet API | Хранение баллов, подпись карт, push-обновления |

Касса **не публикуется в сторах**. Сборка: `flutter build apk --release`, раздача APK сотрудникам (ссылка / USB / MDM). На телефон кассира включить установку из неизвестных источников.

Ограничение: кассир на iPhone это APK не поставит. Если появятся iPhone на кассе — отдельная задача (TestFlight / IPA), не MVP.

---

## 3. Программа лояльности MERCH (бизнес-правила)

Значения в `{фигурных}` и колонка «Дефолт» — параметры админки. До запуска заполнить. Ниже — разумный старт для одежды, не догма.

### 3.1. Единица

Внутренний счёт в **баллах** (целое число ≥ 0). Клиенту на карте: «Баллы».

### 3.2. Начисление (`earn`)

| Параметр | Код | Дефолт | Комментарий |
|---|---|---|---|
| Курс | `earn_percent` | **5%** | 5 баллов за каждые 100 ₽ оплаченного чека |
| Округление | `earn_round` | вниз до целого | 1990 ₽ → 99 баллов |
| База | `earn_base` | сумма к оплате **после** скидок магазина, **до** списания баллов | не начислять с «оплаченной баллами» части |
| Минимум чека | `earn_min_receipt` | 0 | можно поставить порог |
| Возврат товара | `earn_on_refund` | сторно пропорционально | см. §3.5 |

Формула MVP:

```
points_earn = floor( (receipt_payable_rub - points_redeemed_as_rub) * earn_percent / 100 )
```

### 3.3. Списание (`redeem`)

| Параметр | Код | Дефолт | Комментарий |
|---|---|---|---|
| Курс | `redeem_rate` | **1 балл = 1 ₽** | |
| Минимум к списанию | `redeem_min` | **100** баллов | меньше — кнопка неактивна |
| Максимум от чека | `redeem_max_share` | **50%** суммы чека | нельзя закрыть весь чек баллами |
| Нельзя в минус | — | да | `points >= requested` |
| Категории-исключения | `redeem_exclusions` | пусто | этап 2: sale / бельё / сертификаты |

Кассир вводит сумму скидки в рублях **или** число баллов. Сервер приводит к баллам, проверяет все условия одним ответом: `allowed` / `denied` + причина на русском.

### 3.4. Что нельзя в MVP

- Списание без открытого чека (просто «подарить скидку с улицы»)
- Ручное начисление кассиром без суммы чека — только роль админа, с обязательным комментарием
- Передача баллов между клиентами
- Сгорание баллов — параметр `expire_days` = `null` (можно включить позже)

### 3.5. Возврат

Если товар вернули: админ/старший вызывает `POST /cashier/refund`. Сервер:

1. сторнирует начисленные за этот чек баллы (не ниже 0);
2. если в чеке было списание — **возвращает** списанные баллы на карту.

Идемпотентность по `receipt_id`.

### 3.6. Тексты для клиента (на карте и на обороте)

Лицевая сторона: **MERCH** · **Баллы** · QR.

Оборот (коротко, «ты»):

> Баллы начисляются с покупок в MERCH и списываются на кассе. Карта — не платёжное средство. Правила может изменить магазин. Вопросы: {телефон или Telegram}.

Полные правила — ссылка на сайте, на обороте карты поле `terms_url`.

---

## 4. Архитектура

```
Клиент                    Кассир (Flutter APK)              Админ
  │                              │                            │
  │  страница /add               │  HTTPS + Bearer            │
  ▼                              ▼                            ▼
                    ┌─────────────────────────────────┐
                    │         API MERCH               │
                    │  клиенты, баллы, чеки, staff    │
                    └───────────┬─────────────────────┘
              ┌─────────────────┼─────────────────┐
              ▼                 ▼                 ▼
        Apple .pkpass     Google Wallet      SQLite/Postgres
        + APNs push       REST PATCH         ledger баллов
```

Один бэкенд. Касса не ходит в Apple/Google напрямую — только в API MERCH.

Рекомендуемый стек (не догма):

- API: Dart/Shelf, Node или Go — любой, главное HTTPS и транзакции
- БД: PostgreSQL (лучше) или SQLite на старте одной точки
- Касса: Flutter, Android API 26+
- Выдача карты: простой веб (тот же домен)

---

## 5. Идентификаторы и QR

| Сущность | Формат | Пример |
|---|---|---|
| Клиент `customer_id` | UUID | `3f2a…` |
| Штрихкод / QR `barcode` | тот же UUID **или** `MCH-` + 8 символов Crockford | `MCH-7K2P9Q4R` |
| Apple `serialNumber` | = `customer_id` | совпадает навсегда |
| Google object id | `{issuerId}.{customer_id}` | |
| Чек `receipt_id` | UUID, генерит касса или кассовое ПО | ключ идемпотентности |

**В QR только id клиента.** Баланс в QR не класть.

Полезная нагрузка QR: UTF-8 строка `MCH-7K2P9Q4R` (или UUID). Формат **QR**. На кассе сканер/камера читает ту же строку.

---

## 6. Apple Wallet — что создать и что куда писать

### 6.1. Кабинет

1. [developer.apple.com/programs/enroll](https://developer.apple.com/programs/enroll) — Organization **MERCH**, **$99/год**.
2. **Membership → Team ID** → в конфиг `APPLE_TEAM_ID`.
3. **Identifiers → + → Pass Type IDs**
   - Description: `MERCH Loyalty`
   - Identifier: `pass.com.merch.loyalty` (зафиксировать, не менять)
4. **Certificates → + → Pass Type ID Certificate** → CSR с Mac → `.cer` → `.p12` на сервер.
5. Промежуточный **WWDR G4** рядом с сертификатом.
6. **Keys → + → APNs** → файл `.p8`, сохранить Key ID.

Topic push = `pass.com.merch.loyalty` (не bundle id приложения).

### 6.2. Поля `pass.json` MERCH

Тип: **`storeCard`**.

| Ключ | Значение MERCH |
|---|---|
| `formatVersion` | `1` |
| `passTypeIdentifier` | `pass.com.merch.loyalty` |
| `teamIdentifier` | Team ID |
| `serialNumber` | `customer_id` |
| `organizationName` | `MERCH` |
| `description` | `Карта лояльности MERCH` |
| `logoText` | `MERCH` |
| `foregroundColor` | контраст к фону, задать с дизайном |
| `backgroundColor` | фирменный цвет MERCH |
| `webServiceURL` | `https://api.{домен}/passes/` |
| `authenticationToken` | уникальный ≥ 32 символов на клиента |
| `storeCard.headerFields` | опционально номер карты |
| `storeCard.primaryFields[0]` | key `points`, label `Баллы`, value `"120"`, `changeMessage`: `Баланс: %@` |
| `storeCard.secondaryFields` | имя, если есть |
| `storeCard.backFields` | правила, телефон, URL |
| `barcodes[0].format` | `PKBarcodeFormatQR` |
| `barcodes[0].message` | `barcode` клиента |
| `barcodes[0].altText` | тот же код для ручного ввода |

Картинки (имена фиксированы): `icon.png` (+@2x @3x), `logo.png`, `strip.png` (баннер одежды/лого, **без текста на картинке**).

Сборка: манифест SHA-1 → CMS-подпись сертификатом Pass Type ID + WWDR → zip → `.pkpass`.

`Content-Type: application/vnd.apple.pkpass`.

### 6.3. Web Service (обязательные URL)

База: `https://api.{домен}/passes`

| Метод | Путь |
|---|---|
| POST | `/v1/devices/{deviceLibraryId}/registrations/pass.com.merch.loyalty/{serial}` |
| DELETE | тот же |
| GET | `/v1/devices/{deviceLibraryId}/registrations/pass.com.merch.loyalty?passesUpdatedSince=` |
| GET | `/v1/passes/pass.com.merch.loyalty/{serial}` |
| POST | `/v1/log` |

Заголовок: `Authorization: ApplePass {authenticationToken}`.

После смены баллов: тихий APNs (пустое тело) на все `push_token` этой карты → телефон сам скачает новый `.pkpass`.

---

## 7. Google Wallet — что создать и что куда писать

### 7.1. Кабинет

1. [pay.google.com/business/console](https://pay.google.com/business/console) — имя **MERCH**, сохранить **Issuer ID**.
2. Google Cloud: проект, включить **Google Wallet API**, service account, JSON-ключ.
3. В Pay Console пригласить email service account как Developer/Admin.
4. Создать **Loyalty Class** `id = {issuerId}.merch_loyalty`.
5. Тестовые Gmail в Test accounts → заявка **publishing access**.

Для аудитории в РФ Google в бренд-гайдлайне просит кнопку **«Save to phone»**, не «Add to Google Wallet» (продукт Wallet в RU ограничен). API пропусков при этом использовать можно. На странице выдачи: определение устройства + актуальная кнопка из гайдлайна; заложить проверку на пилоте (открывается ли сохранение на типовых Android в РФ, в т.ч. без GMS — Huawei: **fallback на веб-карту**, §10.4).

### 7.2. Class (шаблон всех карт)

| Поле | Значение |
|---|---|
| `id` | `{issuerId}.merch_loyalty` |
| `issuerName` | `MERCH` |
| `programName` | `Карта лояльности` |
| `programLogo` | квадрат ≥ 660×660 PNG, не обрезать заранее под круг |
| `hexBackgroundColor` | фирменный, не неон |
| `reviewStatus` | сначала `UNDER_REVIEW` |
| `textModulesData` | краткие правила |
| `linksModuleData` | сайт, `tel:`, Telegram |

### 7.3. Object (карта человека)

| Поле | Значение |
|---|---|
| `id` | `{issuerId}.{customer_id}` |
| `classId` | `{issuerId}.merch_loyalty` |
| `state` | `ACTIVE` |
| `accountId` | `barcode` |
| `accountName` | имя или пусто |
| `loyaltyPoints.label` | `Баллы` |
| `loyaltyPoints.balance.int` | текущие баллы |
| `barcode.type` | `QR_CODE` |
| `barcode.value` | тот же `barcode` |

Выдача: JWT service account → `https://pay.google.com/gp/v/save/{jwt}`

Обновление: `PATCH .../loyaltyobject/{issuerId}.{customer_id}` только `loyaltyPoints`.

---

## 8. Модель данных

```
stores            id, name, address
staff             id, store_id, name, pin_hash or password_hash, role, active
customers         id, barcode, display_name, phone?,
                  apple_auth_token, google_object_id,
                  points, created_at, blocked
apple_devices     customer_id, device_library_id, push_token
receipts          id, store_id, staff_id, customer_id,
                  amount_rub, redeem_points, earn_points, status, created_at
ledger            id, customer_id, receipt_id?, delta, reason, actor_staff_id, created_at
job_dedupe        key, created_at          -- идемпотентность кассы
```

`customers.points` меняется **только** в транзакции вместе с `ledger`.

Повтор `receipt_id` + операция → тот же результат, второе начисление запрещено.

---

## 9. API бэкенда

База: `https://api.{домен}`

- Касса: `Authorization: Bearer {staff_jwt}`, TTL смены 12 ч.
- Админ: отдельный JWT.
- Страница `/add` — без staff-токена, rate limit.

Ошибки:

```json
{ "error": { "code": "INSUFFICIENT_POINTS", "message": "…" } }
```

Коды: `INSUFFICIENT_POINTS`, `BELOW_MIN_REDEEM`, `EXCEEDS_RECEIPT_SHARE`, `CUSTOMER_NOT_FOUND`, `CUSTOMER_BLOCKED`, `DUPLICATE_RECEIPT`, `STAFF_FORBIDDEN`.

### 9.1. Выдача карты клиенту

`POST /public/enroll`

```json
{ "name": "Анна", "phone": "+7…" }
```

Телефон опционален в MVP (можно только QR на стойке без ПДн). Если телефон есть — 152-ФЗ: согласие на странице.

Ответ:

```json
{
  "customer_id": "…",
  "barcode": "MCH-7K2P9Q4R",
  "apple_url": "https://api…/public/passes/apple/{id}.pkpass",
  "google_save_url": "https://pay.google.com/gp/v/save/…",
  "add_page": "https://merch.ru/card/add/{id}"
}
```

`GET /card/add/{id}` — User-Agent: iOS → скачать pkpass / кнопка Apple; Android с GMS → кнопка Google/Save to phone; иначе веб-карточка с QR.

Повторный заход того же клиента (cookie / тот же phone) **не** создаёт вторую карту.

### 9.2. Касса

`POST /cashier/lookup`

```json
{ "barcode": "MCH-7K2P9Q4R" }
```

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

`POST /cashier/quote-redeem` — посчитать, сколько можно списать с этого чека, без записи.

```json
{ "barcode": "…", "receipt_amount_rub": 4500, "requested_points": 500 }
```

`POST /cashier/commit` — **одна** атомарная операция на чек: списание (0+) и начисление.

```json
{
  "receipt_id": "uuid-с-кассы",
  "barcode": "MCH-7K2P9Q4R",
  "receipt_amount_rub": 4500,
  "redeem_points": 500,
  "store_id": "…"
}
```

Сервер:

1. блокирует клиента (`SELECT … FOR UPDATE`);
2. проверяет лимиты списания;
3. `points -= redeem`; `earn = floor((amount - redeem_as_rub) * percent / 100)`; `points += earn`;
4. пишет `receipts` + две строки ledger (если redeem/earn ≠ 0);
5. ставит в очередь обновление Wallet (не блокировать кассира больше ~300 мс);
6. возвращает новые баллы, earn, redeem.

Повтор с тем же `receipt_id` — 200 и исходный результат, без двойного начисления.

`POST /cashier/refund` — старший смены, тело `{ "receipt_id": "…" }`.

`POST /admin/adjust` — ручная правка баллов, обязательный `reason`, роль admin.

---

## 10. Flutter-приложение кассира (APK)

### 10.1. Сборка и раздача

- Приложение **employee-only**, не для клиентов.
- `flutter build apk --release --split-per-abi` (или один fat APK).
- Версия в UI: `1.0.0+1`, проверка `GET /cashier/app-version` → если ниже `min_supported` — экран «обнови APK» со ссылкой.
- Раздача: закрытая ссылка на ваш сервер / Google Drive / MDM. Не Play Store в MVP.
- Иконка и имя: **MERCH Касса** — чтобы не путать с клиентским.

### 10.2. Экраны MVP

1. **Вход** — логин + пароль или PIN точки. Без входа сканер закрыт.
2. **Смена** — точка, имя кассира, кнопка «закрыть смену».
3. **Скан** — камера (`mobile_scanner` / `qr_code_scanner`), ручной ввод кода, вспышка.
4. **Карточка клиента** — имя, баллы крупно, кнопки «Чек».
5. **Чек** — сумма ₽ (обязательно) → сервер считает доступный redeem → кассир ставит «списать N» в пределах подсказки → подтверждение:
   - списать X баллов (= Y ₽)
   - к оплате Z ₽
   - будет начислено P баллов
6. **Успех** — новый баланс. Если Wallet-push задержится, на телефоне клиента цифра обновится через секунды; кассиру это не блокирует.
7. **История смены** — список чеков, для старшего — «отменить/возврат».
8. **Офлайн** — без сети **не** проводить commit (риск рассинхрона). Только сообщение «нет связи». Lookup тоже online.

### 10.3. Нефункциональное

- Камера: разрешение только на экране скана, текст зачем.
- Не логировать полный barcode в crash-репорты без нужды.
- JWT в `flutter_secure_storage`.
- Таймаут запроса 8 с, повтор commit безопасен за счёт `receipt_id`.
- Запрет скриншотов на экране клиента — по желанию (`FLAG_SECURE`), не обязательно.

### 10.4. Скан в зале и fallback без GMS

- Яркость экрана клиента: Wallet сам подсвечивает QR — не просить «открыть другое приложение».
- Если QR не читается: ручной ввод `altText` с карты.
- Лазерный 1D-сканер кассового аппарата **не обязателен**: в MVP камера телефона сотрудника.
- Huawei / Android без Google Play Services: карта Google Wallet недоступна. Страница `/card/add` показывает **веб-карточку** с тем же QR. Касса сканирует её так же, как Wallet.

---

## 11. Потоки

### 11.1. Выдача карты

1. Клиент сканирует на стойке QR «Получить карту MERCH» → `/card/add`.
2. По желанию имя (и телефон).
3. iPhone: добавить в Apple Wallet. Android: Save to phone / Google Wallet. Иначе: сохранить страницу / скрин QR.
4. Баллы = 0. Карта сразу валидна для скана.

Первая покупка в тот же визит: кассир может `enroll` с кассы («выдать карту») → показать QR на своём экране клиенту для добавления в Wallet **или** сразу привязать и сканировать, если enroll вернул barcode.

### 11.2. Покупка со списанием

1. Кассир бьёт товары в своей кассе (вне скоупа) / вводит сумму в MERCH Касса.
2. Скан QR Wallet.
3. Ввод суммы чека → quote → выбор списания → commit.
4. Клиент платит остаток как обычно (карта, нал, SBP — **не** Wallet).
5. На карте в Wallet через обновление — новый баланс.

### 11.3. Покупка без списания

Тот же commit с `redeem_points: 0`.

---

## 12. Безопасность и право

- Касса только у сотрудников, APK не выкладывать публично (известная ссылка = любой начислит баллы, если утечёт логин — поэтому PIN + флаг `active` + отзыв JWT).
- Rate limit на `/public/enroll` и lookup.
- HTTPS, HSTS.
- Секреты Apple/Google не в репозиторий и не в APK.
- 152-ФЗ, если собираете телефон/имя: политика, согласие, возможность удаления карты (админ `blocked` + Google `EXPIRED`; Apple-карту не перевыпускать под новым serial без нужды — инвалидировать текущий).
- Правила лояльности публично (страница сайта) — тот же текст, что в админке.
- Wallet **не является** электронной предоплатой / подарочным счётом ЦБ: баллы только скидка MERCH, не выводятся в деньги на карту банка. Юристу магазина — одна страница оферты до запуска.

---

## 13. Дизайн карты (контент для дизайнера)

Слоты Wallet нельзя сверстать как лендинг. Отдать:

- лого MERCH (PNG, прозрачность по гайду: Apple — прямоугольник ~160×50 pt; Google — квадрат с полями 15% под круглую маску)
- strip для Apple 375×144 pt @1x и @2x, фото/паттерн **без** надписи «MERCH» на картинке (название уже в `logoText`)
- два цвета: фон и текст
- не обещать на карте % и даты, которых нет в админке

---

## 14. Этапы, сроки, деньги

| Этап | Срок | Результат |
|---|---|---|
| 0. Аккаунты Apple / Google | 3–15 дней (юрлицо Apple дольше) | Team ID, Pass Type ID, Issuer ID |
| 1. Бэкенд: клиенты, ledger, commit | 1.5–2.5 нед | API + тесты идемпотентности |
| 2. Apple pkpass + web service + APNs | 1–1.5 нед | карта ставится и обновляет баллы |
| 3. Google class/object + JWT | ~1 нед | параллельно с п.2 |
| 4. Flutter касса APK | 1.5–2 нед | скан, чек, commit |
| 5. Страница /add + админка min | 3–5 дн | выдача без кассира |
| 6. Пилот 1 точка | 3–7 дн | 10–20 реальных карт |
| 7. Google publishing access | 1–2 раб. дня | снять TEST ONLY |

**Календарно MVP: 5–8 недель** при одном разработчике full-time, не считая ожидания Apple Organization.

Ориентир стоимости разработки (внешняя команда): **350–600 тыс. ₽** за этот объём.

Годовая эксплуатация: Apple **$99**, хостинг **3–10 тыс. ₽/мес**, Google API **0 ₽**.

Касса: существующие Android-телефоны сотрудников.

---

## 15. Критерии приёмки

1. С iPhone карта добавляется, QR читается камерой APK, в QR — barcode MERCH.
2. С Android (GMS) карта добавляется, тот же barcode, тот же клиент.
3. Commit 4500 ₽ / списать 500 / 5% → баллы на сервере и **после обновления** на обеих картах совпадают с формулой §3.
4. Повтор commit с тем же `receipt_id` не меняет баллы.
5. Списание 50 баллов при `redeem_min=100` → отказ, баланс не меняется.
6. Списание больше 50% чека → отказ.
7. APK без логина не сканирует. Уволенный `active=false` не входит.
8. Нет сети → commit не проходит, кассир видит ошибку.
9. Huawei без GMS → веб-карточка с QR, касса сканирует её так же.
10. Сертификат Apple не зашит в APK.

---

## 16. Открытые решения (нужны от MERCH до начала кода)

1. Домен API и страницы выдачи (`merch.ru` / поддомен).
2. Юрлицо для Apple Developer (как в вывеске).
3. Финальные `earn_percent`, `redeem_min`, `redeem_max_share`.
4. Собирать ли телефон при выдаче карты.
5. Число точек и кто админ.
6. Нужна ли выдача карты с экрана кассы в первый визит.
7. Цвета/лого в PNG.
