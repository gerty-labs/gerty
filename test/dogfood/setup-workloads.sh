#!/usr/bin/env bash
# Deploys 8 synthetic workloads that exercise all classification patterns.
# See TESTING_VALIDATION.md for the full workload matrix.
set -euo pipefail

NS="sage-test"

echo "Creating namespace ${NS}..."
kubectl create namespace "${NS}" --dry-run=client -o yaml | kubectl apply -f -

echo ""
echo "Deploying synthetic workloads..."

# 1. nginx-overprovisioned — STEADY, high waste
# Requests 1 CPU / 1Gi but uses ~50m / 80Mi
kubectl apply -n "${NS}" -f - <<'EOF'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-overprovisioned
spec:
  replicas: 3
  selector:
    matchLabels:
      app: nginx-overprovisioned
  template:
    metadata:
      labels:
        app: nginx-overprovisioned
    spec:
      containers:
        - name: nginx
          image: nginx:1.27-alpine
          resources:
            requests:
              cpu: "1"
              memory: 1Gi
            limits:
              cpu: "2"
              memory: 2Gi
          ports:
            - containerPort: 80
EOF

# 2. api-bursty — BURSTABLE
# Low baseline with periodic load spikes from generate-load.sh
kubectl apply -n "${NS}" -f - <<'EOF'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api-bursty
spec:
  replicas: 2
  selector:
    matchLabels:
      app: api-bursty
  template:
    metadata:
      labels:
        app: api-bursty
    spec:
      containers:
        - name: api
          image: nginx:1.27-alpine
          resources:
            requests:
              cpu: 500m
              memory: 256Mi
            limits:
              cpu: "2"
              memory: 512Mi
          ports:
            - containerPort: 80
---
apiVersion: v1
kind: Service
metadata:
  name: api-bursty
spec:
  selector:
    app: api-bursty
  ports:
    - port: 80
      targetPort: 80
EOF

# 3. cronjob-batch — BATCH
# CPU-intensive task that runs every 5 minutes
kubectl apply -n "${NS}" -f - <<'EOF'
apiVersion: batch/v1
kind: CronJob
metadata:
  name: cronjob-batch
spec:
  schedule: "*/5 * * * *"
  jobTemplate:
    spec:
      template:
        spec:
          containers:
            - name: batch
              image: busybox:1.36
              command: ["sh", "-c", "echo 'Starting batch'; i=0; while [ $i -lt 5000000 ]; do i=$((i+1)); done; echo 'Done'"]
              resources:
                requests:
                  cpu: 500m
                  memory: 64Mi
                limits:
                  cpu: "1"
                  memory: 128Mi
          restartPolicy: OnFailure
EOF

# 4. idle-dev — IDLE
# Sleep container with high requests but zero actual usage
kubectl apply -n "${NS}" -f - <<'EOF'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: idle-dev
spec:
  replicas: 1
  selector:
    matchLabels:
      app: idle-dev
  template:
    metadata:
      labels:
        app: idle-dev
    spec:
      containers:
        - name: sleeper
          image: busybox:1.36
          command: ["sleep", "infinity"]
          resources:
            requests:
              cpu: 500m
              memory: 512Mi
            limits:
              cpu: "1"
              memory: 1Gi
EOF

# 5. java-app — STEADY (edge case: JVM heap)
# Simulates JVM memory — allocates and holds memory that looks like high usage
kubectl apply -n "${NS}" -f - <<'EOF'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: java-app
spec:
  replicas: 1
  selector:
    matchLabels:
      app: java-app
  template:
    metadata:
      labels:
        app: java-app
    spec:
      containers:
        - name: java
          image: busybox:1.36
          command: ["sh", "-c", "head -c 200M /dev/urandom > /dev/null; sleep infinity"]
          resources:
            requests:
              cpu: 250m
              memory: 512Mi
            limits:
              cpu: 500m
              memory: 1Gi
EOF

# 6. memory-leak — anomalous growing memory
# Simulates slow memory growth
kubectl apply -n "${NS}" -f - <<'EOF'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: memory-leak
spec:
  replicas: 1
  selector:
    matchLabels:
      app: memory-leak
  template:
    metadata:
      labels:
        app: memory-leak
    spec:
      containers:
        - name: leaker
          image: busybox:1.36
          command: ["sh", "-c", "while true; do head -c 1M /dev/urandom >> /tmp/leak; sleep 30; done"]
          resources:
            requests:
              cpu: 100m
              memory: 256Mi
            limits:
              cpu: 200m
              memory: 512Mi
EOF

# 7. right-sized — already well-tuned
# Low waste, recommendation should be minimal or none
kubectl apply -n "${NS}" -f - <<'EOF'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: right-sized
spec:
  replicas: 1
  selector:
    matchLabels:
      app: right-sized
  template:
    metadata:
      labels:
        app: right-sized
    spec:
      containers:
        - name: worker
          image: busybox:1.36
          command: ["sh", "-c", "while true; do i=0; while [ $i -lt 100000 ]; do i=$((i+1)); done; sleep 1; done"]
          resources:
            requests:
              cpu: 50m
              memory: 32Mi
            limits:
              cpu: 100m
              memory: 64Mi
EOF

# 8. best-effort — no resource requests/limits
# Should report but not recommend (no baseline)
kubectl apply -n "${NS}" -f - <<'EOF'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: best-effort
spec:
  replicas: 1
  selector:
    matchLabels:
      app: best-effort
  template:
    metadata:
      labels:
        app: best-effort
    spec:
      containers:
        - name: app
          image: busybox:1.36
          command: ["sleep", "infinity"]
EOF

echo ""
echo "All workloads deployed to namespace ${NS}."
echo "Waiting for pods to become ready..."
kubectl wait --for=condition=ready pod -l app --timeout=120s -n "${NS}" 2>/dev/null || true

echo ""
echo "Pod status:"
kubectl get pods -n "${NS}" -o wide
