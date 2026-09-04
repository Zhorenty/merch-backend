# Как задеплоить MERCH backend на SmartApe — шпаргалка по шагам

Простыми словами, без теории. Полное обоснование решений — в `docs/TZ-SMARTAPE-DEPLOY.md`, здесь только «что нажать» и «что ввести в терминал».

Итог: один VPS на SmartApe, на нём Docker с тремя контейнерами — `postgres`, `api` (наш Go-бэкенд) и `caddy` (сам получает HTTPS-сертификат и проксирует запросы на `api`).

---

## 0. Что нужно иметь перед началом

- [ ] Домен (см. шаг 1)
- [ ] Аккаунт на SmartApe с деньгами на балансе
- [ ] Свой компьютер с терминалом и SSH (macOS — есть из коробки)

---

## 1. Домен: нужно ли регать на reg.ru?

**Домен нужен обязательно** — без него не будет HTTPS, а без HTTPS Apple Wallet не работает вообще. Регистрировать именно на **reg.ru** не обязательно — можно на любом регистраторе (включая сам SmartApe). Но reg.ru — самый простой и привычный вариант для РФ, поэтому шаги ниже — под него. Если домен уже есть — пропусти покупку и иди сразу к «DNS-запись».

### 1.1. Купить домен на reg.ru (если его ещё нет)

1. Зайти на [reg.ru](https://www.reg.ru), в поиске ввести желаемое имя (например `merch-loyalty.ru`).
2. Нажать **Добавить в корзину** у свободного домена с зоной `.ru`.
3. Перейти в корзину → оформить заказ → оплатить.
4. Домен появится в личном кабинете, раздел **Домены**.

Ничего больше на этом шаге настраивать не нужно — хостинг у reg.ru покупать не надо, сайт будет жить на VPS SmartApe.

### 1.2. DNS-запись (когда уже будет IP-адрес VPS из шага 2)

1. Личный кабинет reg.ru → **Домены** → кликнуть по своему домену.
2. Блок **«DNS-серверы и управление зоной»** → **Изменить**.
3. Нажать **Добавить запись** → в шторке выбрать тип **A**.
4. Заполнить:
   - **Subdomain**: `api` (получится `api.твой-домен.ru` — именно этот адрес пропишем в `.env` как `API_BASE_URL`)
   - **IP Address**: IP-адрес VPS от SmartApe
5. Нажать **Готово**.
6. Повторить то же самое ещё раз, но с **Subdomain**: `@` (это будет `твой-домен.ru` без поддомена) — на него можно повесить страницу выдачи карты `/card/add`, если решите не использовать общий `api.` хост для всего (по умолчанию в проекте один хост на всё, тогда шаг можно пропустить).

DNS применяется не мгновенно — обычно 5–30 минут, иногда до пары часов. Проверить, что запись «доехала», можно с любого компьютера:

```bash
dig +short api.твой-домен.ru
# должен вернуть тот же IP, что у VPS
```

---

## 2. SmartApe: что тыкать, чтобы получить сервер

1. Зарегистрироваться / войти на [smartape.ru](https://www.smartape.ru).
2. В личном кабинете открыть раздел **VPS** (или «Товары/Услуги» → VPS).
3. Нажать **Заказать**.
4. В конфигурации выбрать:
   - Тариф: **NVMe X4** (4 ГБ RAM, диск от 40 ГБ) — младше не бери, Postgres+Go в Docker упрутся в память.
   - ОС: **Ubuntu 24.04 LTS**.
   - Панель управления (Hestia/ISPmanager): **не ставить** — она будет конфликтовать с Docker.
   - Локация: Москва (по умолчанию).
5. Указать SSH-ключ, если панель заказа это предлагает (иначе получишь пароль root на почту/в кабинете и подключишь ключ после первого входа — см. шаг 3.1).
6. Добавить в корзину → оплатить.
7. Через 1–2 минуты сервер готов. В кабинете, в карточке услуги, будет **IP-адрес** сервера — он нужен для DNS-записи (шаг 1.2) и для SSH.

Больше в SmartApe тыкать ничего не нужно — весь остальной деплой делается через терминал по SSH.

---

## 3. Терминал: пошагово от чистого сервера до работающего API

Дальше всё выполняется на своём компьютере (подключение по SSH) либо уже на сервере — помечено в каждом блоке.

### 3.1. Подключиться и завести отдельного пользователя

На своём компе:

```bash
ssh root@IP_СЕРВЕРА
```

На сервере (первый вход под root):

```bash
adduser deploy
usermod -aG sudo deploy

# перенести свой SSH-ключ новому пользователю, если заказывали без ключа
rsync --archive --chown=deploy:deploy ~/.ssh /home/deploy
```

Дальше работаем под `deploy`, root по SSH лучше выключить:

```bash
exit
ssh deploy@IP_СЕРВЕРА
```

### 3.2. Поставить Docker

На сервере:

```bash
curl -fsSL https://get.docker.com | sudo sh
sudo usermod -aG docker deploy
newgrp docker   # чтобы не перелогиниваться, применить группу сразу

docker --version
docker compose version
```

### 3.3. Файрвол (базовая защита)

```bash
sudo apt update && sudo apt install -y ufw fail2ban
sudo ufw allow 22
sudo ufw allow 80
sudo ufw allow 443
sudo ufw enable
sudo systemctl enable --now fail2ban
```

### 3.4. Занести код на сервер

Вариант А — если код лежит в git-репозитории (GitHub/GitLab/Cursor):

```bash
sudo mkdir -p /opt/merch
sudo chown deploy:deploy /opt/merch
cd /opt/merch
git clone ССЫЛКА_НА_РЕПО .
```

Вариант Б — просто скопировать локальную папку проекта с компа (если git-репо ещё нет):

```bash
# выполнить на своём компе, не на сервере
rsync -avz --exclude .git --exclude data \
  /Users/voloshin.georgiy3/Developer/PET/MERCH/merch-backend/ \
  deploy@IP_СЕРВЕРА:/opt/merch/
```

### 3.5. Собрать `.env` для продакшена

На сервере:

```bash
cd /opt/merch
mkdir -p deploy certs
touch deploy/.env
chmod 600 deploy/.env
```

Сгенерировать секреты (выполнить трижды, каждый раз получишь новую случайную строку):

```bash
openssl rand -hex 32
```

Открыть `deploy/.env` (`nano deploy/.env`) и заполнить своими значениями (домен и три сгенерированные строки выше):

```env
# Postgres
POSTGRES_USER=merch
POSTGRES_PASSWORD=ВСТАВЬ_ПЕРВУЮ_СЛУЧАЙНУЮ_СТРОКУ
POSTGRES_DB=merch

# API
HTTP_ADDR=:8080
DATABASE_URL=postgres://merch:ВСТАВЬ_ПЕРВУЮ_СЛУЧАЙНУЮ_СТРОКУ@postgres:5432/merch?sslmode=disable
API_BASE_URL=https://api.твой-домен.ru
PUBLIC_BASE_URL=https://api.твой-домен.ru
CORS_ORIGINS=https://api.твой-домен.ru

CASHIER_JWT_SECRET=ВСТАВЬ_ВТОРУЮ_СЛУЧАЙНУЮ_СТРОКУ
ADMIN_JWT_SECRET=ВСТАВЬ_ТРЕТЬЮ_СЛУЧАЙНУЮ_СТРОКУ

ADMIN_BOOTSTRAP_LOGIN=admin
ADMIN_BOOTSTRAP_PASSWORD=придумай-сильный-пароль
ADMIN_BOOTSTRAP_NAME=Админ

TERMS_URL=https://example.com/loyalty-terms
SUPPORT_CONTACT=Telegram @merch
APP_MIN_SUPPORTED=1.0.0
APP_DOWNLOAD_URL=https://api.твой-домен.ru/static/merch-kassa.apk

# Apple/Google — пусто пока нет сертификатов, бэк работает и без них (wallet-заглушка)
APPLE_TEAM_ID=
APPLE_PASS_TYPE_ID=pass.com.merch.loyalty
APPLE_PASS_CERT_P12=
APPLE_PASS_CERT_PASSWORD=
APPLE_WWDR_CERT=
APPLE_APNS_P8=
APPLE_APNS_KEY_ID=
APPLE_APNS_PRODUCTION=false
GOOGLE_ISSUER_ID=
GOOGLE_SA_JSON=
GOOGLE_CLASS_SUFFIX=merch_loyalty
```

Замени `api.твой-домен.ru` на реальный домен из шага 1.

### 3.6. Файлы деплоя: compose и Caddy

Создать `deploy/docker-compose.prod.yml`:

```bash
cat > deploy/docker-compose.prod.yml <<'EOF'
services:
  postgres:
    image: postgres:16-alpine
    restart: unless-stopped
    env_file: .env
    volumes:
      - merch_pg:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${POSTGRES_USER} -d ${POSTGRES_DB}"]
      interval: 5s
      timeout: 5s
      retries: 20

  api:
    build:
      context: ..
    restart: unless-stopped
    depends_on:
      postgres:
        condition: service_healthy
    env_file: .env
    volumes:
      - ./certs:/certs:ro

  caddy:
    image: caddy:2-alpine
    restart: unless-stopped
    depends_on:
      - api
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile:ro
      - caddy_data:/data
      - caddy_config:/config

volumes:
  merch_pg:
  caddy_data:
  caddy_config:
EOF
```

Создать `deploy/Caddyfile` (замени домен на свой):

```bash
cat > deploy/Caddyfile <<'EOF'
api.твой-домен.ru {
    reverse_proxy api:8080
}
EOF
```

Caddy сам получит и продлит сертификат Let's Encrypt — никакого certbot не нужно.

### 3.7. Запуск

```bash
cd /opt/merch/deploy
docker compose -f docker-compose.prod.yml up -d --build
docker compose -f docker-compose.prod.yml ps
```

Смотрим логи, если что-то не поднялось:

```bash
docker compose -f docker-compose.prod.yml logs -f api
```

### 3.8. Проверка (смоук-тест)

```bash
curl -fsS https://api.твой-домен.ru/healthz
# ожидаем: {"status":"ok"}

curl -fsS -X POST https://api.твой-домен.ru/admin/login \
  -H 'Content-Type: application/json' \
  -d '{"login":"admin","password":"придумай-сильный-пароль"}'
# ожидаем JSON с токеном, без ошибки
```

Если `healthz` не отвечает — проверь `dig +short api.твой-домен.ru` (DNS доехал?) и `docker compose logs caddy` (сертификат выдался?).

---

## 4. Как обновлять код (все следующие релизы)

```bash
cd /opt/merch
git pull                     # если через git; при копировании — повторить rsync с шага 3.4
cd deploy
docker compose -f docker-compose.prod.yml up -d --build
```

Откат к прошлой версии — то же самое, но после `git checkout <прошлый_коммит>` (или `git reset --hard <коммит>`).

---

## 5. Бэкап базы (сделать сразу после первого запуска)

```bash
mkdir -p /var/backups/merch
docker compose -f /opt/merch/deploy/docker-compose.prod.yml exec -T postgres \
  pg_dump -U merch -d merch -Fc > /var/backups/merch/merch-$(date +%F).dump
```

Добавить в cron (`crontab -e`) на каждый день:

```
0 3 * * * docker compose -f /opt/merch/deploy/docker-compose.prod.yml exec -T postgres pg_dump -U merch -d merch -Fc > /var/backups/merch/merch-$(date +\%F).dump
```

Копию `/var/backups/merch/` не забыть выгружать за пределы VPS (S3 / другой сервер) — если сервер умрёт, локальная копия умрёт с ним.

---

## 6. Коротко: минимальный набор команд от начала до конца

```bash
# на компе
ssh root@IP_СЕРВЕРА

# на сервере, под root
adduser deploy && usermod -aG sudo deploy
rsync --archive --chown=deploy:deploy ~/.ssh /home/deploy
exit

# на компе снова
ssh deploy@IP_СЕРВЕРА

# на сервере, под deploy
curl -fsSL https://get.docker.com | sudo sh
sudo usermod -aG docker deploy && newgrp docker
sudo apt update && sudo apt install -y ufw fail2ban
sudo ufw allow 22 && sudo ufw allow 80 && sudo ufw allow 443 && sudo ufw enable

sudo mkdir -p /opt/merch && sudo chown deploy:deploy /opt/merch
cd /opt/merch && git clone ССЫЛКА_НА_РЕПО .
mkdir -p deploy certs && chmod 700 certs
nano deploy/.env            # заполнить по шагу 3.5

# создать deploy/docker-compose.prod.yml и deploy/Caddyfile по шагу 3.6

cd deploy
docker compose -f docker-compose.prod.yml up -d --build
curl -fsS https://api.твой-домен.ru/healthz
```

Готово — API живёт по HTTPS, база в Postgres в отдельном контейнере, сертификат продлевается сам.
