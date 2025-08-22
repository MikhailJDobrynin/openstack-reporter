# OpenStack Reporter Helm Chart

Этот Helm chart предназначен для развертывания OpenStack Resource Reporter в Kubernetes.

## Описание

OpenStack Reporter - это веб-приложение для мониторинга и отчетности по ресурсам OpenStack. Приложение собирает информацию о виртуальных машинах, дисках, сетях, балансировщиках нагрузки и других ресурсах OpenStack.

## Требования

- Kubernetes 1.19+
- Helm 3.0+
- Доступ к OpenStack API
- PersistentVolume для хранения данных (опционально)

## Установка

### Базовая установка

```bash
# Добавить репозиторий (если есть)
helm repo add openstack-reporter https://your-repo-url

# Установить chart
helm install openstack-reporter ./helm/openstack-reporter
```

### Установка с кастомными значениями

```bash
# Создать values файл
cp helm/openstack-reporter/values-production.yaml my-values.yaml

# Отредактировать my-values.yaml с вашими настройками OpenStack

# Установить с кастомными значениями
helm install openstack-reporter ./helm/openstack-reporter -f my-values.yaml
```

## Конфигурация

### Основные параметры

| Параметр | Описание | По умолчанию |
|----------|----------|--------------|
| `replicaCount` | Количество реплик | `1` |
| `image.repository` | Docker image репозиторий | `ghcr.io/vasyakrg/openstack-reporter` |
| `image.tag` | Docker image тег | `latest` |
| `service.type` | Тип Kubernetes сервиса | `ClusterIP` |
| `ingress.enabled` | Включить Ingress | `false` |
| `persistence.enabled` | Включить постоянное хранилище | `true` |

### Конфигурация OpenStack

```yaml
openstack:
  auth:
    authUrl: "https://your-openstack-auth-url:5000/v3"
    username: "your-username"
    password: "your-password"
    projectName: "your-project"
    projectId: "your-project-id"
    domainName: "your-domain"
    regionName: "your-region"
```

### Конфигурация приложения

```yaml
config:
  collectionInterval: 30  # Интервал сбора данных в минутах
  maxBackups: 7          # Максимальное количество резервных копий
  logLevel: "info"       # Уровень логирования
```

## Использование

### Доступ к приложению

После установки приложение будет доступно по адресу, указанному в NOTES.txt:

```bash
# Для получения URL
helm status openstack-reporter

# Для port-forward (если используется ClusterIP)
kubectl port-forward svc/openstack-reporter 8080:8080
```

### Проверка статуса

```bash
# Проверить статус подов
kubectl get pods -l app.kubernetes.io/name=openstack-reporter

# Посмотреть логи
kubectl logs -l app.kubernetes.io/name=openstack-reporter

# Проверить сервис
kubectl get svc openstack-reporter
```

### Обновление

```bash
# Обновить релиз
helm upgrade openstack-reporter ./helm/openstack-reporter -f my-values.yaml

# Обновить только image
helm upgrade openstack-reporter ./helm/openstack-reporter --set image.tag=v1.0.29
```

### Удаление

```bash
# Удалить релиз
helm uninstall openstack-reporter

# Удалить с сохранением данных
helm uninstall openstack-reporter --keep-history
```

## Безопасность

### Хранение секретов

Пароль OpenStack хранится в Kubernetes Secret:

```bash
# Создать secret вручную (альтернатива)
kubectl create secret generic openstack-secret \
  --from-literal=password=your-password
```

### RBAC

Chart создает ServiceAccount с минимальными правами. Для продакшена рекомендуется настроить RBAC правила.

## Мониторинг

### Health Checks

Приложение предоставляет health check endpoint:
- `/api/health` - проверка состояния приложения

### Метрики

Приложение может быть интегрировано с Prometheus для сбора метрик.

## Troubleshooting

### Проблемы с подключением к OpenStack

1. Проверьте правильность URL аутентификации
2. Убедитесь, что учетные данные корректны
3. Проверьте доступность OpenStack API

### Проблемы с хранилищем

1. Убедитесь, что StorageClass существует
2. Проверьте права доступа к PersistentVolume
3. Проверьте доступное место на диске

### Проблемы с сетью

1. Проверьте настройки Ingress
2. Убедитесь, что DNS настроен правильно
3. Проверьте firewall правила

## Поддержка

Для получения поддержки создайте issue в GitHub репозитории проекта.
