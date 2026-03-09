# Kubernetes Manifests

## Legal Disclaimer

This project and these manifests are provided **as-is** for educational and informational
purposes only. Nothing here is financial advice or a recommendation to buy/sell securities.
Use at your own risk and validate all outputs independently.

This folder contains Kubernetes resources for:
- `stock-forecast-db` StatefulSet (PostgreSQL) with PVC
- `stock-forecast-backend` Deployment + Service
- `stock-forecast-frontend` Deployment + Service
- `secrets.example.yaml` templates for DB/backend secrets and DB init script

## Storage

The DB StatefulSet uses:
- Storage Class: `px-csi-db`
- Access Mode: `ReadWriteOnce`
- Size: `10Gi`

PVC is created by StatefulSet volume claim template:
- `data-stock-forecast-db-0`

## DB Connection Model

Backend consumes DB through Kubernetes service `stock-forecast-db:5432`
using:
- backend `envFrom` secret: `stock-forecast-backend-secrets`
- DB `envFrom` secret: `stock-forecast-db-secrets`
- DB init script secret: `stock-forecast-db-init`

The DB init script creates the application role/database from secret values.
It runs only when PostgreSQL initializes a fresh data directory.

## Frontend Runtime Notes

Frontend runs as non-root with read-only root filesystem. The deployment
mounts `emptyDir` volumes for:
- `/etc/nginx/conf.d` (entrypoint `envsubst` output)
- `/var/cache/nginx` (client/temp/cache dirs)
- `/var/run` and `/tmp` (runtime pid/temp files)

## Apply

1) Create secrets from templates:

```bash
cp k8s/secrets.example.yaml k8s/secrets.yaml
# edit k8s/secrets.yaml and replace placeholder passwords and API key
kubectl apply -f k8s/secrets.yaml
```

2) Apply workloads:

```bash
kubectl apply -f k8s/db-statefulset.yaml
kubectl apply -f k8s/backend-deployment.yaml
kubectl apply -f k8s/frontend-deployment.yaml
```

## Verify

```bash
kubectl get pods
kubectl get pvc
kubectl get svc
kubectl describe pod -l app=stock-forecast-backend
```

## Probes

- Postgres uses `pg_isready` for startup/readiness/liveness probes.
- Backend uses `/healthz` for startup/readiness/liveness probes.
