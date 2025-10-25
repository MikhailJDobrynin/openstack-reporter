# OpenStack Reporter Helm Chart

This Helm chart is designed for deploying **OpenStack Resource Reporter** in Kubernetes.

## Description

**OpenStack Reporter** is a web application for monitoring and reporting on OpenStack resources.  
The application collects information about virtual machines, volumes, networks, load balancers, and other OpenStack resources.

**Note:** The application is intended to run as a single replica since it uses local data storage.

## Requirements

- Kubernetes 1.19+
- Helm 3.0+
- Access to the OpenStack API
- PersistentVolume for data storage (optional)

## Installation

### Basic Installation

```bash
# Add the repository (if available)
helm repo add openstack-reporter https://your-repo-url

# Install the chart
helm install openstack-reporter ./helm/openstack-reporter
```

### Installation with Custom Values

```bash
# Create a values file
cp helm/openstack-reporter/values-production.yaml my-values.yaml

# Edit my-values.yaml with your OpenStack settings

# Install with custom values
helm install openstack-reporter ./helm/openstack-reporter -f my-values.yaml
```

## Configuration

### Main Parameters

| Parameter              | Description                      | Default Value                         |
|------------------------|----------------------------------|---------------------------------------|
| `image.repository`     | Docker image repository           | `ghcr.io/vasyakrg/openstack-reporter` |
| `image.tag`            | Docker image tag                  | `latest`                              |
| `service.type`         | Kubernetes service type           | `ClusterIP`                           |
| `ingress.enabled`      | Enable Ingress                    | `false`                               |
| `persistence.enabled`  | Enable persistent storage         | `true`                                |

### OpenStack Configuration

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

### Application Configuration

```yaml
config:
  collectionInterval: 30  # Data collection interval in minutes
  maxBackups: 7           # Maximum number of backups
  logLevel: "info"        # Logging level
```

## Usage

### Accessing the Application

After installation, the application will be accessible at the address specified in `NOTES.txt`:

```bash
# Get the URL
helm status openstack-reporter

# Port-forward (if using ClusterIP)
kubectl port-forward svc/openstack-reporter 8080:8080
```

### Checking Status

```bash
# Check pod status
kubectl get pods -l app.kubernetes.io/name=openstack-reporter

# View logs
kubectl logs -l app.kubernetes.io/name=openstack-reporter

# Check the service
kubectl get svc openstack-reporter
```

### Upgrading

```bash
# Upgrade the release
helm upgrade openstack-reporter ./helm/openstack-reporter -f my-values.yaml

# Upgrade image only
helm upgrade openstack-reporter ./helm/openstack-reporter --set image.tag=v1.0.29
```

### Uninstalling

```bash
# Uninstall the release
helm uninstall openstack-reporter

# Uninstall and keep history
helm uninstall openstack-reporter --keep-history
```

## Security

### Secret Management

The OpenStack password is stored in a Kubernetes Secret:

```bash
# Create the secret manually (alternative)
kubectl create secret generic openstack-secret   --from-literal=password=your-password
```

### RBAC

The chart creates a ServiceAccount with minimal privileges.  
For production use, it is recommended to adjust RBAC rules appropriately.

## Monitoring

### Health Checks

The application provides a health check endpoint:
- `/api/health` — application health status

### Metrics

The application can be integrated with Prometheus for metrics collection.

## Troubleshooting

### OpenStack Connection Issues

1. Verify the authentication URL
2. Make sure the credentials are correct
3. Check the availability of the OpenStack API

### Storage Issues

1. Ensure the `StorageClass` exists
2. Verify access rights to the PersistentVolume
3. Check available disk space

### Network Issues

1. Review the Ingress configuration
2. Make sure DNS is configured correctly
3. Check firewall rules

## Support

For support, please create an issue in the project's GitHub repository.
https://github.com/vasyakrg

