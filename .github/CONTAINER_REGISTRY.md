# GitHub Container Registry

Этот проект автоматически публикует Docker образы в GitHub Container Registry (GHCR).

## Доступные образы

- **Latest**: `ghcr.io/[username]/openstack-reporter:latest`
- **Tagged releases**: `ghcr.io/[username]/openstack-reporter:v1.0.0`
- **Branch builds**: `ghcr.io/[username]/openstack-reporter:main`

## Использование

### Запуск с Docker

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

### Запуск с Docker Compose

```yaml
version: '3.8'
services:
  openstack-reporter:
    image: ghcr.io/[username]/openstack-reporter:latest
    ports:
      - "8080:8080"
    environment:
      - OS_USERNAME=your_username
      - OS_PASSWORD=your_password
      - OS_AUTH_URL=https://your-openstack.example.com:5000/v3
      - OS_PROJECT_DOMAIN_NAME=your_domain
      - OS_USER_DOMAIN_NAME=your_domain
      - OS_IDENTITY_API_VERSION=3
      - OS_AUTH_TYPE=password
      - OS_INSECURE=true
    volumes:
      - ./data:/app/data
    restart: unless-stopped
```

## Аутентификация

Для загрузки приватных образов:

```bash
echo $GITHUB_TOKEN | docker login ghcr.io -u [username] --password-stdin
```

## Поддерживаемые архитектуры

- `linux/amd64`
- `linux/arm64`

## Автоматическая сборка

Образы автоматически собираются при:

1. **Push в main** → `ghcr.io/[username]/openstack-reporter:latest`
2. **Создание тега** → `ghcr.io/[username]/openstack-reporter:v1.0.0`
3. **Pull Request** → сборка без публикации

## Безопасность

Все образы автоматически сканируются на уязвимости с помощью Trivy.
Результаты доступны на вкладке Security в GitHub репозитории.
