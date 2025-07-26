# OpenStack Resources Reporter

Веб-приложение для создания отчетов по ресурсам в облаке OpenStack с возможностью группировки, сортировки и экспорта в PDF.

## Возможности

- 📊 **Сбор данных**: Автоматический сбор информации о ресурсах через OpenStack API
- 🗂️ **Группировка**: По проектам, типу ресурсов или статусу
- 🔄 **Сортировка**: По имени, дате создания, статусу или типу (с обратной сортировкой)
- 📄 **PDF экспорт**: Генерация профессиональных PDF отчетов с детализацией по проектам
- 💾 **Кэширование**: Сохранение данных в JSON файлы для быстрого доступа
- 🎨 **Современный UI**: Адаптивный интерфейс с Bootstrap и информативными подписями ресурсов
- 📖 **API документация**: Встроенная документация всех API эндпоинтов
- 🔍 **Информативные подписи**: Отображение полезной информации вместо UUID (Flavor, IP, размеры дисков)

## Поддерживаемые ресурсы

- ✅ Проекты (Projects)
- ✅ Виртуальные машины (Servers) - с информацией о Flavor и сетях
- ✅ Диски (Volumes) - с данными о подключении, типе и размере
- ✅ Балансировщики нагрузки (Load Balancers) - с IP адресами
- ✅ Плавающие IP (Floating IPs) - с информацией о подключенных ресурсах
- ✅ VPN соединения (IPSec Site Connections) - с Peer Address
- ✅ Роутеры (Routers)
- ❌ Kubernetes кластеры (в планах)

## Установка

### Требования

- Go 1.21+
- Доступ к OpenStack API

### Быстрый старт

#### Вариант 1: Docker (рекомендуется)

```bash
docker run -d \
  --name openstack-reporter \
  -p 8080:8080 \
  -e OS_USERNAME=your_username \
  -e OS_PASSWORD=your_password \
  -e OS_AUTH_URL=https://your-openstack.example.com:5000/v3 \
  -e OS_PROJECT_DOMAIN_NAME=your_domain \
  -e OS_USER_DOMAIN_NAME=your_domain \
  -e OS_IDENTITY_API_VERSION=3 \
  -e OS_AUTH_TYPE=password \
  -e OS_INSECURE=true \
  ghcr.io/[username]/openstack-reporter:latest
```

#### Вариант 2: Docker Compose

```bash
curl -O https://raw.githubusercontent.com/[username]/openstack-reporter/main/docker-compose.yml
# Отредактировать переменные окружения в docker-compose.yml
docker-compose up -d
```

#### Вариант 3: Сборка из исходников

1. **Клонировать репозиторий**
```bash
git clone <repository-url>
cd openstack-reporter
```

2. **Настроить переменные окружения**
```bash
cp .env.example .env
# Отредактировать .env файл с вашими данными OpenStack
```

3. **Установить зависимости**
```bash
go mod download
```

4. **Запустить приложение**
```bash
go run main.go
```

5. **Открыть в браузере**
```
http://localhost:8080
```

## Конфигурация

### Переменные окружения

Создайте файл `.env` на основе `.env.example`:

```bash
# OpenStack Authentication
OS_PROJECT_DOMAIN_NAME=vhc-pc
OS_USER_DOMAIN_NAME=vhc-pc
OS_USERNAME=your_username
OS_PASSWORD=your_password
OS_AUTH_URL=https://eu3-cloud.domain.cpm:5000/v3
OS_IDENTITY_API_VERSION=3
OS_AUTH_TYPE=password
OS_INSECURE=true

# Application Configuration
PORT=8080
```

### Параметры OpenStack

- `OS_AUTH_URL` - URL эндпоинта аутентификации OpenStack
- `OS_USERNAME` - Имя пользователя
- `OS_PASSWORD` - Пароль
- `OS_PROJECT_DOMAIN_NAME` - Домен проекта
- `OS_USER_DOMAIN_NAME` - Домен пользователя
- `OS_INSECURE` - Отключить проверку SSL сертификатов (true/false)

## Использование

### Веб-интерфейс

1. **Главная страница** - Обзор всех ресурсов с картами сводки
2. **Группировка** - Выберите группировку по проектам, типу или статусу
3. **Сортировка** - Сортировка по различным полям (с возможностью обратной сортировки)
4. **Фильтрация** - Фильтр по типу ресурса
5. **Информативные подписи** - показывают полезные данные:
   - ВМ: Flavor и IP адреса (например, "Flavor: va-2-4, IPs: 172.21.x.x")
   - Диски: Тип, загрузочность, подключение, размер
   - Floating IP: К какому ресурсу подключен
   - Load Balancer: Внутренние и внешние IP адреса
   - VPN: Peer Address
6. **Обновление данных** - Кнопка "Обновить данные" для получения свежей информации
7. **PDF экспорт** - Кнопка "Экспорт PDF" для скачивания отчета
8. **API документация** - Доступ к встроенной документации API

### API эндпоинты

- `GET /api/resources` - Получить список ресурсов
- `POST /api/refresh` - Обновить данные из OpenStack
- `GET /api/export/pdf` - Скачать PDF отчет
- `GET /api/status` - Статус кэшированных данных
- `GET /api/version` - Информация о версии приложения
- `GET /api/docs` - API документация в JSON формате

### Веб-страницы

- `GET /` - Главная страница с отчетом
- `GET /docs` - Страница с API документацией

## Архитектура

```
openstack-reporter/
├── main.go                 # Точка входа
├── internal/
│   ├── models/            # Модели данных
│   ├── openstack/         # OpenStack API клиент
│   ├── storage/           # JSON хранилище
│   ├── handlers/          # HTTP обработчики
│   ├── pdf/               # PDF генератор
│   └── version/           # Управление версиями
├── web/
│   ├── templates/         # HTML шаблоны
│   └── static/           # CSS/JS/изображения
└── data/                 # Кэшированные данные (создается автоматически)
```

## Разработка

### Структура проекта

- **internal/models** - Структуры данных для всех типов ресурсов
- **internal/openstack** - Клиент для работы с OpenStack API
- **internal/storage** - Компонент для сохранения/загрузки JSON данных
- **internal/handlers** - HTTP обработчики для API
- **internal/pdf** - Генератор PDF отчетов
- **web/** - Веб-интерфейс (HTML, CSS, JavaScript)

### CI/CD

Проект использует GitHub Actions для автоматизации:

- **Тестирование и сборка** - при каждом push и PR
- **Docker образы** - автоматическая публикация в GitHub Container Registry
- **Мультиплатформенная сборка** - поддержка `linux/amd64` и `linux/arm64`
- **Сканирование безопасности** - проверка образов на уязвимости
- **Релизы** - автоматическое создание GitHub релизов с бинарными файлами

### Docker образы

Доступны в GitHub Container Registry:
- `ghcr.io/[username]/openstack-reporter:latest` - последняя версия
- `ghcr.io/[username]/openstack-reporter:v1.0.0` - тегированные релизы

### Бинарные файлы

При каждом релизе автоматически создаются предкомпилированные бинарные файлы для:
- **Linux**: `amd64`, `arm64`
- **macOS**: `amd64`, `arm64`
- **Windows**: `amd64`, `arm64`

Скачайте с [страницы релизов GitHub](https://github.com/[username]/openstack-reporter/releases)

### Добавление новых типов ресурсов

1. Добавить модель в `internal/models/resource.go`
2. Реализовать сбор данных в `internal/openstack/client.go`
3. Обновить PDF генератор в `internal/pdf/generator.go`
4. Добавить отображение в веб-интерфейс

## Безопасность

- Используйте HTTPS в продакшене
- Храните учетные данные OpenStack в переменных окружения
- Ограничьте доступ к приложению через файрвол или прокси
- Регулярно обновляйте зависимости

## Устранение неполадок

### Проблемы с подключением к OpenStack

1. Проверьте правильность URL и учетных данных
2. Убедитесь, что `OS_INSECURE=true` для самоподписанных сертификатов
3. Проверьте сетевую доступность к OpenStack API

### Проблемы с правами доступа

- Убедитесь, что пользователь имеет права на чтение ресурсов
- Приложение работает с текущим проектом пользователя (не требует админских прав)
- Для получения данных всех проектов нужны админские права, но это не обязательно

### Логи

Приложение выводит логи в stdout. Для отладки проверьте:
```bash
go run main.go 2>&1 | tee app.log
```

## Лицензия

MIT License

## Поддержка

Для вопросов и предложений создавайте Issues в репозитории.
