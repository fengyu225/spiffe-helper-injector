# SPIFFE Helper Injector

A Kubernetes admission controller that injects SPIFFE helper containers into pods with the label `spiffe.io/spire-managed-identity: "true"`.

## Overview

This project provides a mutating webhook admission controller that:
- Injects a SPIFFE helper init container for initial certificate setup
- Injects a SPIFFE helper sidecar container for ongoing certificate renewal
- Injects volumes for SPIRE agent socket, certificates, and configuration


## Installation

### 1. Setup Vault

```bash
# Start Vault server
cd spire/vault
docker-compose up -d

# Initialize Vault and configure PKI as SPIRE upstream CA
./init-vault.sh
```

### 2. Setup PostgreSQL

```bash
cd spire/postgres
docker-compose up -d
```

### 3. Deploy SPIRE and OIDC Discovery

```bash
# Create SPIRE namespace
kubectl create namespace spire

# Apply Vault certificates secret
kubectl apply -f spire/vault/vault-certs-secret.yaml

# Deploy SPIRE server and agent
kubectl apply -k spire
```

### 4. Deploy the Webhook Injector

```bash
# Deploy the webhook components
kubectl apply -k webhook/manifests
```

### 5. Deploy Workloads with SPIFFE Identity

```bash
# Deploy example workloads
kubectl apply -k workload
```

## Project Structure

```
├── kind-config.yaml           # Kind cluster configuration
├── spire                      # SPIRE installation manifests
│   ├── agent-cluster-kubeconfig
│   ├── bundle.crt
│   ├── cluster-spiffe-ids.yaml
│   ├── configmaps
│   ├── crds
│   ├── deployments
│   ├── kustomization.yaml
│   ├── namespace.yaml
│   ├── postgres
│   ├── rbac
│   ├── services
│   ├── spiffe-csi-driver.yaml
│   ├── vault
│   └── webhooks
├── webhook                    # SPIFFE Helper webhook injector
│   ├── Dockerfile
│   ├── Makefile
│   ├── go.mod
│   ├── go.sum
│   ├── main.go               # Main webhook implementation
│   └── manifests             # Kubernetes manifests for webhook
└── workload                   # Example workloads
    ├── deployments
    ├── kustomization.yaml
    ├── namespace.yaml
    └── rbac
```