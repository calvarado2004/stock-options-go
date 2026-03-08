# Kubernetes Manifests

This folder contains Kubernetes resources for:
- `stock-forecast-db` StatefulSet (SQLite holder) with PVC
- `stock-forecast-backend` Deployment + Service
- `stock-forecast-frontend` Deployment + Service

## Storage

The DB StatefulSet uses:
- Storage Class: `px-csi-db`
- Access Mode: `ReadWriteOnce`
- Size: `10Gi`

PVC is created by StatefulSet volume claim template:
- `data-stock-forecast-db-0`

## Apply

```bash
kubectl apply -f k8s/db-statefulset.yaml
kubectl apply -f k8s/backend-deployment.yaml
kubectl apply -f k8s/frontend-deployment.yaml
```

## Optional secret for Alpha Vantage

```bash
kubectl create secret generic stock-forecast-secrets \
  --from-literal=alpha_api_key="<your-alpha-key>"
```

## Verify

```bash
kubectl get pods
kubectl get pvc
kubectl get svc
```
