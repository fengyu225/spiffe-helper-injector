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

```bash
# Check pod has spiffe-helper init container and sidecar container
k get pod test-workload-6476db76bc-8tcdr -n app -o yaml
````

```bash
apiVersion: v1
kind: Pod
metadata:
  annotations:
    kubectl.kubernetes.io/restartedAt: "2025-03-26T17:10:08-07:00"
  creationTimestamp: "2025-03-27T00:10:08Z"
  generateName: test-workload-6476db76bc-
  labels:
    app: test-workload
    pod-template-hash: 6476db76bc
    spiffe.io/spire-managed-identity: "true"
  name: test-workload-6476db76bc-8tcdr
  namespace: app
  ownerReferences:
  - apiVersion: apps/v1
    blockOwnerDeletion: true
    controller: true
    kind: ReplicaSet
    name: test-workload-6476db76bc
    uid: 9f6991da-c713-4313-90f3-15d0ff6e885f
  resourceVersion: "9911"
  uid: b2b506de-7d82-41fe-986c-df0040d39a54
spec:
  containers:
  - command:
    - sleep
    - infinity
    image: curlimages/curl
    imagePullPolicy: Always
    name: main-app
    resources: {}
    terminationMessagePath: /dev/termination-log
    terminationMessagePolicy: File
    volumeMounts:
    - mountPath: /var/run/secrets/kubernetes.io/serviceaccount
      name: kube-api-access-9scjm
      readOnly: true
  - args:
    - -config
    - /etc/spiffe-helper/helper.conf
    image: docker.io/fengyu225/spiffe-helper:v0.0.1
    imagePullPolicy: Always
    name: spiffe-helper
    resources: {}
    terminationMessagePath: /dev/termination-log
    terminationMessagePolicy: File
    volumeMounts:
    - mountPath: /etc/spiffe-helper
      name: spiffe-helper-config
    - mountPath: /run/spire/agent-sockets
      name: spire-agent-socket
    - mountPath: /run/spiffe/certs
      name: spiffe-certs
    - mountPath: /var/run/secrets/kubernetes.io/serviceaccount
      name: kube-api-access-9scjm
      readOnly: true
  dnsPolicy: ClusterFirst
  enableServiceLinks: true
  initContainers:
  - args:
    - -config
    - /etc/spiffe-helper/helper.conf
    - -daemon-mode=false
    image: docker.io/fengyu225/spiffe-helper:v0.0.1
    imagePullPolicy: Always
    name: spiffe-helper-init
    resources: {}
    terminationMessagePath: /dev/termination-log
    terminationMessagePolicy: File
    volumeMounts:
    - mountPath: /etc/spiffe-helper
      name: spiffe-helper-config
    - mountPath: /run/spire/agent-sockets
      name: spire-agent-socket
    - mountPath: /run/spiffe/certs
      name: spiffe-certs
    - mountPath: /var/run/secrets/kubernetes.io/serviceaccount
      name: kube-api-access-9scjm
      readOnly: true
  nodeName: spire-demo-worker
  preemptionPolicy: PreemptLowerPriority
  priority: 0
  restartPolicy: Always
  schedulerName: default-scheduler
  securityContext: {}
  serviceAccount: test-workload
  serviceAccountName: test-workload
  terminationGracePeriodSeconds: 30
  tolerations:
  - effect: NoExecute
    key: node.kubernetes.io/not-ready
    operator: Exists
    tolerationSeconds: 300
  - effect: NoExecute
    key: node.kubernetes.io/unreachable
    operator: Exists
    tolerationSeconds: 300
  volumes:
  - name: kube-api-access-9scjm
    projected:
      defaultMode: 420
      sources:
      - serviceAccountToken:
          expirationSeconds: 3607
          path: token
      - configMap:
          items:
          - key: ca.crt
            path: ca.crt
          name: kube-root-ca.crt
      - downwardAPI:
          items:
          - fieldRef:
              apiVersion: v1
              fieldPath: metadata.namespace
            path: namespace
  - hostPath:
      path: /run/spire/agent-sockets
      type: Directory
    name: spire-agent-socket
  - configMap:
      defaultMode: 420
      name: webhook-spiffe-helper-config
    name: spiffe-helper-config
  - emptyDir: {}
    name: spiffe-certs
status:
  conditions:
  - lastProbeTime: null
    lastTransitionTime: "2025-03-27T00:10:19Z"
    status: "True"
    type: Initialized
  - lastProbeTime: null
    lastTransitionTime: "2025-03-27T00:10:21Z"
    status: "True"
    type: Ready
  - lastProbeTime: null
    lastTransitionTime: "2025-03-27T00:10:21Z"
    status: "True"
    type: ContainersReady
  - lastProbeTime: null
    lastTransitionTime: "2025-03-27T00:10:08Z"
    status: "True"
    type: PodScheduled
  containerStatuses:
  - containerID: containerd://d9023acb7b2e439458adbbc34796dc881ffd036fd219241acd99d1780c082ce9
    image: docker.io/curlimages/curl:latest
    imageID: docker.io/curlimages/curl@sha256:94e9e444bcba979c2ea12e27ae39bee4cd10bc7041a472c4727a558e213744e6
    lastState: {}
    name: main-app
    ready: true
    restartCount: 0
    started: true
    state:
      running:
        startedAt: "2025-03-27T00:10:20Z"
  - containerID: containerd://448b9c82591862c4755d50e8f761c203a6c15ae7859c4f43628949e84f664929
    image: docker.io/fengyu225/spiffe-helper:v0.0.1
    imageID: docker.io/fengyu225/spiffe-helper@sha256:7c8a27a32ec4d493d279d522d4247573cec41be1ba609ab09e09dc4332aaa021
    lastState: {}
    name: spiffe-helper
    ready: true
    restartCount: 0
    started: true
    state:
      running:
        startedAt: "2025-03-27T00:10:21Z"
  hostIP: 172.18.0.2
  initContainerStatuses:
  - containerID: containerd://28c223acdd17f60b55ac112a50d4362d1f628d1dfb1944250ffe34e51fef25b3
    image: docker.io/fengyu225/spiffe-helper:v0.0.1
    imageID: docker.io/fengyu225/spiffe-helper@sha256:7c8a27a32ec4d493d279d522d4247573cec41be1ba609ab09e09dc4332aaa021
    lastState: {}
    name: spiffe-helper-init
    ready: true
    restartCount: 0
    state:
      terminated:
        containerID: containerd://28c223acdd17f60b55ac112a50d4362d1f628d1dfb1944250ffe34e51fef25b3
        exitCode: 0
        finishedAt: "2025-03-27T00:10:19Z"
        reason: Completed
        startedAt: "2025-03-27T00:10:09Z"
  phase: Running
  podIP: 10.240.1.20
  podIPs:
  - ip: 10.240.1.20
  qosClass: BestEffort
  startTime: "2025-03-27T00:10:08Z"
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