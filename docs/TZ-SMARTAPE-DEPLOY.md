# ТЗ: деплой бэкенда MERCH на SmartApe

| | |
|---|---|
| Версия | 1.0 |
| Дата | 28.08.2026 |
| Площадка | [SmartApe](https://www.smartape.ru/) (VPS KVM, РФ) |
| Объект | HTTPS API карты лояльности MERCH (`merch-backend`) |
| Статус | к согласованию домена (§16 основного ТЗ) |

Связанные документы: `docs/TZ-MERCH-WALLET.md`, `docs/api.md`, `.env.example`, `Dockerfile`.

---

## 1. Цель и границы

### 1.1. Цель

Выкатить **один** публичный HTTPS-сервис MERCH на VPS SmartApe так, чтобы:

- касса (Flutter APK) ходила в API по Bearer;
- страница выдачи `/card/add` открывалась с телефона клиента;
- Apple PassKit web service (`/passes/…`) был доступен **только по HTTPS** с валидным сертификатом;
- баллы жили в PostgreSQL с бэкапами;
- секреты и сертификаты Apple/Google не попадали в git.

Источник правды по-прежнему сервер, не Wallet.

### 1.2. Входит

- Заказ и первичная настройка VPS SmartApe
- DNS, TLS (Let's Encrypt), reverse-proxy
- Docker Compose: `api` + `postgres` (+ proxy)
- Прод-конфиг env, миграции при старте
- Бэкапы БД, файрвол, базовый hardening
- Проверка `GET /healthz` и смоук enroll / login
- Runbook: выкатка, откат, смена секретов

### 1.3. Не входит

- Shared-хостинг / ISPmanager / Hestia как runtime для Go (панель можно не ставить)
- Деплой Flutter-кассы и кабинетов Apple/Google Developer
- CDN, Kubernetes, отдельный staging-кластер (staging — опция, см. §9)
- Мониторинг уровня Datadog (достаточно health + диск + логи)
- 1С, Эвотор, отдельный файловый хостинг APK в MVP можно положить на тот же nginx

---

## 2. Площадка SmartApe: что заказывать

SmartApe даёт **KVM VPS с root**, автоустановку Ubuntu/Debian, ЦОД уровня TIER-III/IV, заявленный uptime **99.982%**. Для этого бэкенда нужен **VPS**, не «безлимитный сайт-хостинг».

### 2.1. Рекомендуемый тариф (пилот 1 точка)

| Параметр | Значение | Зачем |
|---|---|---|
| Линейка | **NVMe** (не HDD) | Postgres + образы Docker |
| Минимум | **NVMe X4**: 4 ГБ RAM, 2+ vCPU, диск от **40 ГБ** | api + postgres + логи + 1–2 релиза образов |
| Комфорт | NVMe X8, если на той же машине APK + бэкапы 7–14 дней | запас под рост точек |
| ОС | **Ubuntu 24.04 LTS** (альтернатива Debian 12) | Docker CE, certbot/caddy |
| Панель | **не ставить** Hestia/ISPmanager | конфликт с Docker/nginx |
| Сеть | 1 публичный IPv4, трафик безлимит (как в тарифе) | касса + Wallet push исходящий |
| Локация | Москва (дефолт SmartApe) | касса в РФ, низкий RTT |

Ориентир цены (сайт SmartApe, 2026): NVMe X4 ≈ **995 ₽/мес**, X2 (2 ГБ) ≈ 585 ₽ — **X2 не брать**: postgres + Go в Docker на 2 ГБ будут упираться в OOM при сборке/миграциях.

Выделенный («дедик») для MVP **не нужен**. Переезд на dedicated — если появятся десятки точек и жёсткий SLA.

### 2.2. Домен и DNS (открыто в §16 основного ТЗ)

До согласования домена заложить схему:

| Имя | Назначение |
|---|---|
| `api.{домен}` | API + PassKit `webServiceURL` = `https://api.{домен}/passes/` |
| `{домен}` или `card.{домен}` | страница `/card/add` (может быть тот же хост, что API) |

В коде сейчас один процесс отдаёт и API, и `/card/add`. **Дефолт деплоя: один хост** `https://api.{домен}` для всего. Отдельный `merch.ru` — reverse-proxy на тот же контейнер, если маркетинг хочет короткий URL выдачи.

A-запись → IPv4 VPS. Без HTTPS Apple Wallet **не** регистрирует web service.

### 2.3. Что купить у SmartApe кроме VPS

- VPS NVMe X4+
- Опционально: услуга бэкапа ВМ у провайдера (если есть в биллинге) — **дополнение**, не замена `pg_dump`
- Домен `.ru` можно у них же или оставить у текущего регистратора

---

## 3. Целевая схема на сервере

```
Интернет
   │  :443 TLS
   ▼
nginx или Caddy          ← Let's Encrypt, HSTS, прокси
   │  127.0.0.1:8080
   ▼
Docker: merch-api        ← бинарь Go, user nonroot, HTTP_ADDR=:8080
   │
   ▼
Docker: postgres:16      ← volume, не публиковать 5432 наружу
```

Исходящие с VPS (не блокировать):

- `api.push.apple.com` / `api.sandbox.push.apple.com` (APNs)
- `walletobjects.googleapis.com`, `oauth2.googleapis.com` (Google Wallet)
- `pay.google.com` не обязателен с сервера (JWT save URL открывает клиент)

SQLite на проде **запрещён**. Только Postgres.

Миграции: процесс API сам гоняет goose при старте. Отдельный job `migrate-only` — для ручного прогона.

---

## 4. Состав поставки (артефакты)

Уже есть в репозитории:

- `Dockerfile` (multi-stage, `CGO_ENABLED=0`, distroless)
- `docker-compose.yml` — **только для локалки** (пароли `merch/merch`, порт 5432 наружу)

Нужно **добавить при реализации деплоя** (не часть этого ТЗ как код, но обязательный результат работ):

| Файл | Назначение |
|---|---|
| `deploy/docker-compose.prod.yml` | api + postgres, без публикации 5432 |
| `deploy/nginx.conf` или Caddyfile | TLS, `client_max_body_size 1m`, прокси `/` |
| `deploy/.env.prod.example` | список ключей без значений |
| Каталог на сервере `/opt/merch/` | compose, `.env`, `certs/` (mode 700) |

Исправление перед первым `docker build` на сервере: в текущем `Dockerfile` есть `COPY … /src/assets`, каталога `assets/` в репо нет — сборка упадёт. Либо завести пустой `assets/.gitkeep`, либо убрать строку (шаблоны уже в бинаре через `embed`).

---

## 5. Конфигурация продакшена

Все секреты — **только** файл `/opt/merch/.env` на VPS (права `600`, владелец деплой-пользователь). Не в git, не в образ.

### 5.1. Обязательно сменить

| Переменная | Прод |
|---|---|
| `DATABASE_URL` | `postgres://merch:<случайный>@postgres:5432/merch?sslmode=disable` (TLS внутри docker-сети не обязателен; снаружи Postgres нет) |
| `CASHIER_JWT_SECRET` | ≥32 случайных байт |
| `ADMIN_JWT_SECRET` | **другой** секрет, ≥32 байт |
| `ADMIN_BOOTSTRAP_PASSWORD` | сильный пароль; после первого входа сменить через API / не светить в чатах |
| `API_BASE_URL` | `https://api.{домен}` без trailing slash |
| `PUBLIC_BASE_URL` | тот же или публичный URL страницы выдачи |
| `CORS_ORIGINS` | только реальные origin страницы выдачи (если отдельный домен) |
| `TERMS_URL` | публичная оферта |
| `SUPPORT_CONTACT` | телефон / Telegram магазина |
| `APP_DOWNLOAD_URL` | закрытая ссылка на APK (тот же nginx `/apk/…` с basic auth или одноразовый URL) |

### 5.2. Apple / Google (можно выкатить stub, потом докинуть файлы)

Пока кабинетов нет: API живёт, в логах `wallet update skipped`. Когда появятся ключи:

```
APPLE_TEAM_ID=
APPLE_PASS_TYPE_ID=pass.com.merch.loyalty
APPLE_PASS_CERT_P12=/certs/pass.p12
APPLE_PASS_CERT_PASSWORD=
APPLE_WWDR_CERT=/certs/wwdr.pem
APPLE_APNS_P8=/certs/AuthKey_XXXX.p8
APPLE_APNS_KEY_ID=
APPLE_APNS_PRODUCTION=true          # после выпуска карт на проде
GOOGLE_ISSUER_ID=
GOOGLE_SA_JSON=/certs/google-sa.json
```

Файлы монтировать read-only: `./certs:/certs:ro`. Не класть `.p12`/`.p8`/JSON в репозиторий.

`webServiceURL` в pass.json собирается из `API_BASE_URL` + `/passes/`. После смены домена старые карты в Apple **не** переедут сами — закладывать финальный домен до массовой выдачи.

### 5.3. Процесс

```
HTTP_ADDR=:8080
```

Снаружи только 80/443. Postgres, 8080, 5432 — localhost/docker.

---

## 6. Безопасность на VPS

Соответствие §12 основного ТЗ.

1. Пользователь `deploy` с docker (без ежедневного root). SSH **только ключ**, `PermitRootLogin no`, нестандартный порт — по желанию.
2. UFW/nftables: вход `22` (или свой SSH), `80`, `443`. Исход — любой. **5432 закрыт.**
3. fail2ban на sshd.
4. Автообновления безопасности Ubuntu (`unattended-upgrades`).
5. Reverse-proxy: HSTS (`max-age=31536000`), редирект HTTP→HTTPS. Приложение уже ставит HSTS, если видит `X-Forwarded-Proto: https` — прокси **обязан** передавать этот заголовок и `X-Real-IP` / `X-Forwarded-For` (rate limit по IP).
6. `client_max_body_size 1m` — как лимит тела API.
7. После пилота: сменить bootstrap-пароль админа, отозвать дефолтный если светился.
8. APK кассы не класть в открытый `/` без защиты ссылки.

---

## 7. Бэкапы и диски

| Что | Как | Частота | Хранение |
|---|---|---|---|
| Postgres | `pg_dump -Fc` в `/var/backups/merch/` + копирование **off-box** (S3/другой VPS/диск) | ежедневно | ≥ 14 дней |
| Сертификаты Apple/Google | копия каталога `certs/` в том же off-box бэкапе | при изменении | бессрочно |
| Volume Docker | не считать единственной копией | — | — |

Восстановление: поднять пустой postgres → `pg_restore` → запустить api (goose no-op, если схема уже в дампе). Прогон восстановления — **один раз до пилота**.

Шифрование диска у SmartApe на VPS обычно нет из коробки; PII (имя/телефон) — минимизация по 152-ФЗ, доступ к SSH ограничен.

---

## 8. Выкатка и откат

### 8.1. Первый деплой (ручной, приемлемо для MVP)

1. Заказать VPS, поставить Ubuntu 24.04, ключ SSH.
2. Установить Docker Engine + Compose plugin.
3. Создать `/opt/merch`, положить compose.prod, `.env`, пустой `certs/`.
4. DNS A → IP, дождаться пропагации.
5. Поднять compose, получить сертификат (Caddy сам или certbot + nginx).
6. `curl -fsS https://api.{домен}/healthz` → `{"status":"ok"}`.
7. Смоук: `POST /public/enroll`, `POST /admin/login` (учётка bootstrap).

Сборку образа предпочтительно на сервере (`docker compose build`) — раннер GitHub не обязателен в MVP.

### 8.2. Последующие релизы

```
cd /opt/merch && git pull   # или копирование нового образа
docker compose -f docker-compose.prod.yml up -d --build
```

Healthcheck контейнера: `GET /healthz` каждые 10 с. Откат: предыдущий тег образа / предыдущий git commit + `up -d`.

Миграции только вперёд (goose Up). Обратные — отдельное решение, в MVP не накатывать Down на проде.

### 8.3. CI (этап 2, не блокер пилота)

GitHub Actions: `go test ./...` на push; деплой по SSH на `main` — опционально. Секреты SSH и `.env` не хранить в репозитории.

---

## 9. Staging

Не обязателен для пилота 1 точки. Если появится:

- второй маленький VPS или тот же хост, порты/поддомен `staging-api.{домен}`;
- отдельные JWT-секреты и БД;
- `APPLE_APNS_PRODUCTION=false`.

Не смешивать продовые карты Apple с staging `webServiceURL`.

---

## 10. Наблюдаемость

- `docker compose logs -f api` (JSON slog). Не писать полный barcode и токены (уже в коде).
- Диск: алерт если `/` > 80% (логи + WAL postgres).
- По желанию: Uptime Kuma / тот же SmartApe-мониторинг на `https://api.{домен}/healthz` раз в 1 мин.
- После включения APNs: смотреть ошибки push в логе, не падать кассу.

---

## 11. Оценка ресурсов и денег

Нагрузка пилота (10–20 карт, 1 касса): десятки RPS пик, commit < 50 мс CPU. NVMe X4 с запасом.

| Статья | Ориентир / мес |
|---|---|
| SmartApe NVMe X4 | ~1 000 ₽ |
| Домен .ru | ~20 ₽ (годовая / 12) |
| Off-box бэкап (мелкий object storage) | 0–300 ₽ |
| Apple Developer | $99 / год |
| Google Wallet API | 0 ₽ |
| **Итого хостинг** | **~1–2 тыс. ₽** (в коридоре 3–10 тыс. ₽ из §14 основного ТЗ остаётся запас на рост и бэкап ВМ) |

Работы по этому ТЗ: **1–2 рабочих дня** при готовом домене и доступе в биллинг SmartApe. Ожидание DNS/Apple — вне этой оценки.

---

## 12. Критерии приёмки деплоя

1. С телефона открывается `https://api.{домен}/healthz` без сертификатных предупреждений.
2. `https://api.{домен}/card/add` отдаёт HTML выдачи.
3. `POST /public/enroll` создаёт клиента; повтор по телефону не плодит карту.
4. `POST /admin/login` и `POST /cashier/login` работают с продовыми секретами; дефолт `changeme` на проде отсутствует или сменён.
5. Порт **5432 не открыт** в интернет (`nmap` с внешней сети).
6. Ребут VPS: compose `restart: unless-stopped` поднимает api и postgres сами, данные volume на месте.
7. Сделан и проверен restore из `pg_dump` на копии (можно на той же машине в другой контейнер).
8. В `.env` нет плейсхолдеров `change-me`; `API_BASE_URL` начинается с `https://`.
9. Касса с 4G (не Wi‑Fi магазина) логинится и делает lookup — проверка NAT/файрвола.
10. (Когда будут сертификаты) скачивание `.pkpass` и регистрация устройства Apple идут на `https://api.{домен}/passes/`.

Пункты 1–9 — блокер пилота. Пункт 10 — блокер выдачи в Apple Wallet, не блокер кассы по веб-QR.

---

## 13. Открытые решения (нужны от MERCH)

1. Финальный домен API и страницы выдачи (см. §16 основного ТЗ).
2. Кто платит SmartApe (юрлицо магазина) и на чей email аккаунт.
3. Нужен ли отдельный хост только для `/card/add`.
4. Где хранить off-box бэкапы (второй VPS / объектное хранилище / диск бухгалтера — не git).
5. Ставить ли CI сразу или ручной `compose up` на пилот.

---

## 14. Порядок работ

| # | Шаг | Результат |
|---|---|---|
| 1 | Согласовать домен и тариф NVMe X4 | биллинг SmartApe, DNS |
| 2 | Починить Docker-сборку (`assets` / COPY) | `docker build` зелёный |
| 3 | compose.prod + nginx/Caddy + `.env` на VPS | контейнеры up |
| 4 | TLS + файрвол + бэкап dump | приёмка §12.1–12.8 |
| 5 | Смоук API по `docs/api.md` | enroll / login / health |
| 6 | По мере появления — смонтировать Apple/Google certs | Wallet без stub |

---

## 15. Риски

| Риск | Митигация |
|---|---|
| Смена домена после выдачи Apple-карт | не раздавать pkpass, пока DNS финальный |
| Сборка образа на 2 ГБ RAM | не брать X2; сборка с `--memory` или собирать CI и грузить image |
| Утечка `.env` / bootstrap-пароля | 600 на файл, смена JWT-секретов = разлогин всех касс |
| SmartApe shared hosting «для сайта» | не использовать; только VPS с root |
| Postgres на том же диске без dump | ежедневный dump + копия вне VPS |
