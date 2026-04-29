# Cloud SQL Proxy Setup — HyperFleet API

Complete guide to connecting HyperFleet API to Cloud SQL via the auth proxy sidecar, from GCP IAM to Kubernetes configuration.

---

## Architecture

```
┌──────────────────────────────────────┐         ┌───────────────────────────┐
│  Pod: myapi-hyperfleet-api           │         │  GCP — Cloud SQL          │
│                                      │         │                           │
│  ┌────────────────────┐              │         │  ┌─────────────────────┐  │
│  │  hyperfleet-api    │              │   TLS   │  │  PostgreSQL 18      │  │
│  │  (Go / lib/pq)     │              │ tunnel  │  │  IAM database auth  │  │
│  └─────────┬──────────┘              ├─────────►  └─────────────────────┘  │
│            │ localhost:5432          │         │                           │
│  ┌─────────▼──────────┐              │         └───────────────────────────┘
│  │  cloud-sql-proxy   │              │
│  │  --auto-iam-authn  │              │
│  │  --port=5432       │              │
│  └────────────────────┘              │
│                                      │
└──────────────────────────────────────┘
```

**Key points:**

- The app sets `sslmode=disable` — the proxy handles TLS end-to-end
- With `--auto-iam-authn` the proxy injects an IAM token; the password in the secret is ignored
- `db.host` must always be `127.0.0.1` — never the Cloud SQL instance IP directly

---

## Step 1 — GCP Configuration

### 1.1 Create the GCP service account and grant Cloud SQL access

Our pod running in GKE will impersonate this service account.

Why do we need a service account? In Sentinel and Adapters we are using Google Pub/Sub and there is no need for a service account

In case of Sentinel and Adapters, a IAM `principalSet://` is created with permission for all the Kubernetes service accounts running in the hcm-hyperfleet project with permissions to Pub/Sub

In case of CloudSQL we need a IAM principal (user) that is a user of the database. That means that it can not be a `principalSet://`, it needs to be a plain old service account. The user in PosgreSQL will be something like `mysa-api@hcm-hyperfleet.iam`

```bash
# Create the service account (skip if already exists)
gcloud iam service-accounts create mysa-api \
  --display-name="HyperFleet API" \
  --project=hcm-hyperfleet

# Grant Cloud SQL Client role — allows the proxy to open the tunnel
gcloud projects add-iam-policy-binding hcm-hyperfleet \
  --member="serviceAccount:mysa-api@hcm-hyperfleet.iam.gserviceaccount.com" \
  --role="roles/cloudsql.client"

# Grant Cloud SQL Instance User role — required for IAM database authentication
# Without this, the proxy connects but Cloud SQL rejects login with:
# "Cloud SQL IAM service account authentication failed"
gcloud projects add-iam-policy-binding hcm-hyperfleet \
  --member="serviceAccount:mysa-api@hcm-hyperfleet.iam.gserviceaccount.com" \
  --role="roles/cloudsql.instanceUser"
```

### 1.2 Create the Cloud SQL IAM database user

```bash
gcloud sql users create mysa-api@hcm-hyperfleet.iam \
  --instance=mypsql \
  --type=CLOUD_IAM_SERVICE_ACCOUNT \
  --project=hcm-hyperfleet
```

> The `db.user` secret value must be `mysa-api@hcm-hyperfleet.iam` (with the `@project.iam` suffix). The proxy maps this to the GCP service account token for auth.

### 1.3 Grant PostgreSQL permissions to the IAM user — CRITICAL

Creating the IAM database user (step 1.2) only gives it `LOGIN` privilege. Without explicit
grants the user authenticates successfully but immediately gets `permission denied for database`
or `permission denied for schema public`.

Since `mysa-api@hcm-hyperfleet.iam` runs both `db-migrate` (needs CREATE/ALTER TABLE) and the
runtime API (SELECT/INSERT/UPDATE/DELETE), grant it Cloud SQL's built-in superuser role:

```bash
# Connect to the Cloud SQL instance as the postgres user
gcloud sql connect mypsql --user=postgres --project=hcm-hyperfleet
```

```sql
-- Run inside psql
\c hyperfleet

GRANT cloudsqlsuperuser TO "mysa-api@hcm-hyperfleet.iam";
```

Or apply least-privilege grants instead:

```sql
\c hyperfleet

GRANT CONNECT ON DATABASE hyperfleet TO "mysa-api@hcm-hyperfleet.iam";
GRANT ALL ON SCHEMA public TO "mysa-api@hcm-hyperfleet.iam";
GRANT ALL ON ALL TABLES IN SCHEMA public TO "mysa-api@hcm-hyperfleet.iam";
GRANT ALL ON ALL SEQUENCES IN SCHEMA public TO "mysa-api@hcm-hyperfleet.iam";
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES TO "mysa-api@hcm-hyperfleet.iam";
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON SEQUENCES TO "mysa-api@hcm-hyperfleet.iam";
```

> `ALTER DEFAULT PRIVILEGES` ensures tables and sequences created by future `db-migrate` runs
> are automatically accessible — without it, each migration adds objects the user cannot access.

---

### 1.4 Bind Workload Identity — CRITICAL

The role **must** be `roles/iam.workloadIdentityUser`, not `serviceAccountUser`.

Take note that we are setting the namespace from where the k8s service account is running (`test-api`)

```bash
gcloud iam service-accounts add-iam-policy-binding \
  mysa-api@hcm-hyperfleet.iam.gserviceaccount.com \
  --project=hcm-hyperfleet \
  --role="roles/iam.workloadIdentityUser" \
  --member="serviceAccount:hcm-hyperfleet.svc.id.goog[test-api/myapi-hyperfleet-api]"
```

| Field | Value |
|---|---|
| GCP project | `hcm-hyperfleet` |
| GCP service account | `mysa-api@hcm-hyperfleet.iam.gserviceaccount.com` |
| Workload Identity pool | `hcm-hyperfleet.svc.id.goog` |
| K8s namespace | `test-api` |
| K8s service account | `myapi-hyperfleet-api` |

> **Common mistake:** Using `roles/iam.serviceAccountUser` instead of `roles/iam.workloadIdentityUser` causes the proxy to log: `Permission 'iam.serviceAccounts.getAccessToken' denied`

---

## Step 2 — Kubernetes Resources

### 2.1 ServiceAccount annotation

The K8s ServiceAccount must be annotated so GKE knows which GCP SA to impersonate. Verify:

```bash
kubectl get serviceaccount -n test-api myapi-hyperfleet-api -o yaml
# Expected annotation:
# iam.gke.io/gcp-service-account: mysa-api@hcm-hyperfleet.iam.gserviceaccount.com
```

### 2.2 Database secret — password must NOT be empty — CRITICAL

```bash
kubectl create secret generic my-db-secret \
  -n test-api \
  --from-literal=db.host=127.0.0.1 \
  --from-literal=db.port=5432 \
  --from-literal=db.name=hyperfleet \
  --from-literal=db.user=mysa-api@hcm-hyperfleet.iam \
  --from-literal=db.password=placeholder

# Or patch an existing secret:
kubectl patch secret -n test-api your-db-secret \
  --type='json' \
  -p='[{"op":"replace","path":"/data/db.password","value":"'"$(echo -n "placeholder" | base64)"'"}]'
```

> Even though `--auto-iam-authn` makes the proxy ignore the password, lib/pq's DSN parser has a bug where an **empty unquoted password** causes it to swallow the rest of the connection string — including `sslmode=disable`. When sslmode is missing, lib/pq defaults to `require`, which fails against the proxy (see Bug #2).

---

## Step 3 — Helm Chart Configuration (`charts/values.yaml`)

### 3.1 Service account annotation

```yaml
serviceAccount:
  annotations:
    iam.gke.io/gcp-service-account: "mysa-api@hcm-hyperfleet.iam.gserviceaccount.com"
```

### 3.2 Cloud SQL Proxy sidecar — must use `nativeSidecars`

The proxy **must** be placed under `nativeSidecars`, not `sidecars`. Kubernetes runs init
containers (including `db-migrate`) before regular containers start. If the proxy is a
regular sidecar, `db-migrate` can never reach it → `connection refused`. Native sidecars
(K8s 1.28+) are init containers with `restartPolicy: Always` — they start before other
init containers and keep running for the pod lifetime.

You **must** include `restartPolicy: Always` in the spec explicitly — the chart renders the
entry as-is. See [Sidecar Containers](database.md#sidecar-containers) for a full comparison of
`nativeSidecars` vs `sidecars`.

```yaml
nativeSidecars:
  - name: cloud-sql-proxy
    restartPolicy: Always
    image: gcr.io/cloud-sql-connectors/cloud-sql-proxy:2.14.3
    args:
      - "--structured-logs"
      - "--port=5432"
      - "--auto-iam-authn"
      - "hcm-hyperfleet:europe-southwest1:mypsql"  # project:region:instance
    securityContext:
      allowPrivilegeEscalation: false
      capabilities:
        drop: [ALL]
      readOnlyRootFilesystem: true
      runAsNonRoot: true
      seccompProfile:
        type: RuntimeDefault
    resources:
      requests:
        cpu: 100m
        memory: 64Mi
      limits:
        cpu: 200m
        memory: 128Mi
```

Find the instance connection name via:

```bash
gcloud sql instances describe mypsql --project=hcm-hyperfleet --format='value(connectionName)'
```

### 3.3 Database credentials — wired automatically via `database.external`

When `database.external.enabled=true`, the chart injects `HYPERFLEET_DATABASE_*` env vars
from the secret into both the `db-migrate` init container and the main container automatically.
No manual `env:` entries needed.

```yaml
database:
  external:
    enabled: true
    secretName: my-db-secret  # must contain db.host, db.port, db.name, db.user, db.password
```

### 3.4 ConfigMap: SSL mode must be "disable"

```yaml
# config.yaml mounted at /config/config.yaml
database:
  ssl:
    mode: disable   # required — proxy handles TLS, not PostgreSQL
```

> If `ssl.mode` is absent from the config, it defaults to `"disable"`.

---

## Step 4 — Application Code (`pkg/config/db.go`)

### 4.1 `escapeDSNValue` must quote empty strings (already fixed in main)

```go
func escapeDSNValue(value string) string {
    if value == "" {
        // Per libpq spec, empty values must be quoted; an unquoted empty value
        // causes lib/pq's parser to consume the next key=value pair as the value.
        return "''"
    }
    escaped := strings.ReplaceAll(value, `\`, `\\`)
    escaped = strings.ReplaceAll(escaped, `'`, `\'`)
    if !simpleDSNValuePattern.MatchString(escaped) {
        return fmt.Sprintf("'%s'", escaped)
    }
    return escaped
}
```

This fix is already in `main`. The test in `pkg/config/db_test.go` asserts `""` → `''`.

### 4.2 What a correct DSN looks like

```
host=127.0.0.1 port=5432 dbname=hyperfleet user='mysa-api@hcm-hyperfleet.iam' password=placeholder sslmode=disable
```

With empty password (after fix — still correct):

```
host=127.0.0.1 port=5432 dbname=hyperfleet user='mysa-api@hcm-hyperfleet.iam' password='' sslmode=disable
```

---

## Step 5 — Verification

### 5.1 Check proxy logs

```bash
kubectl logs -n test-api deploy/myapi-hyperfleet-api -c cloud-sql-proxy --tail=50

# Healthy output:
# Authorizing with Application Default Credentials
# Listening on 127.0.0.1:5432
# The proxy has started successfully and is ready for new connections!
```

### 5.2 Test connectivity via psql

```bash
kubectl debug -it \
  -n test-api deploy/myapi-hyperfleet-api \
  --image=postgres:18 \
  --target=hyperfleet-api \
  -- bash

# Inside the container:
psql -h 127.0.0.1 -p 5432 -U mysa-api@hcm-hyperfleet.iam -d hyperfleet
# Should show: psql (18.x) ... hyperfleet=>
```

### 5.3 Verify Workload Identity binding

```bash
gcloud iam service-accounts get-iam-policy \
  mysa-api@hcm-hyperfleet.iam.gserviceaccount.com \
  --project=hcm-hyperfleet

# Look for:
# bindings:
# - members:
#   - serviceAccount:hcm-hyperfleet.svc.id.goog[test-api/myapi-hyperfleet-api]
#   role: roles/iam.workloadIdentityUser   ← must be this role
```

---

## Known Failure Modes

### Bug #1 — Wrong Workload Identity role

**Symptom:**

```
cloud-sql-proxy: googleapi: Error 403: Permission 'iam.serviceAccounts.getAccessToken' denied
```

**Root cause:** The IAM binding on the GCP SA uses `roles/iam.serviceAccountUser` instead of `roles/iam.workloadIdentityUser`.

**Fix:** Step 1.4 — rebind with `roles/iam.workloadIdentityUser`.

---

### Bug #2 — lib/pq DSN parser swallows `sslmode` when password is empty

**Symptom:**

```
pq: SSL is not enabled on the server
```

**Root cause:** DSN string contains `password= sslmode=disable` (unquoted empty value). The lib/pq parser (conn.go `parseOpts`) reads `sslmode=disable` as the password value, never setting sslmode. `ssl.go` handles `case "", "require"` identically — sends SSLRequest — Cloud SQL Proxy responds `N` — error.

**Fix (already in main):** `escapeDSNValue("")` returns `''`. Also: use a non-empty placeholder password in the K8s secret.

---

### Bug #3 — Proxy sidecar ordering: `db-migrate` can't reach proxy

**Symptom:**

```
dial tcp 127.0.0.1:5432: connect: connection refused   (from db-migrate init container)
```

**Root cause:** The proxy is configured under `sidecars` (regular containers). Kubernetes
starts init containers before any regular containers, so `db-migrate` runs while the proxy
isn't listening yet — a deadlock. The rollout stalls with both old and new pods stuck.

**Fix:** Move the proxy to `nativeSidecars` in `values.yaml`. The chart renders it as an
init container with `restartPolicy: Always`, which starts before `db-migrate` and keeps
running. Requires Kubernetes 1.28+ (GKE 1.29+ has this by default).

---

### Bug #4 — Missing `roles/cloudsql.instanceUser`

**Symptom:**

```
pq: Cloud SQL IAM service account authentication failed for user "mysa-api@hcm-hyperfleet.iam"
```

**Root cause:** The GCP SA has `roles/cloudsql.client` (allows the proxy tunnel) but is
missing `roles/cloudsql.instanceUser` (grants `cloudsql.instances.login`, required for IAM
database authentication). The proxy connects successfully but Cloud SQL rejects the login.

**Fix:**

```bash
gcloud projects add-iam-policy-binding hcm-hyperfleet \
  --member="serviceAccount:mysa-api@hcm-hyperfleet.iam.gserviceaccount.com" \
  --role="roles/cloudsql.instanceUser"
```

---

## Error Reference

| Error | Component | Root cause | Fix |
|---|---|---|---|
| `Permission 'iam.serviceAccounts.getAccessToken' denied` | cloud-sql-proxy | Wrong IAM role (`serviceAccountUser` vs `workloadIdentityUser`) | Step 1.4 |
| `pq: SSL is not enabled on the server` | hyperfleet-api | Empty password → sslmode swallowed by lib/pq parser | Non-empty password or code fix |
| `dial tcp 127.0.0.1:5432: connect: connection refused` (db-migrate) | db-migrate | Proxy in `sidecars` instead of `nativeSidecars` — starts after init containers | Bug #3 — move to `nativeSidecars` |
| `dial tcp 127.0.0.1:5432: connect: connection refused` (app) | hyperfleet-api | Proxy not listening — proxy auth failed (Bug #1 active) | Fix proxy auth, restart pod |
| `Cloud SQL IAM service account authentication failed` | hyperfleet-api | GCP SA missing `roles/cloudsql.instanceUser` | Step 1.1 — add instanceUser role |
| `permission denied for database` / `permission denied for schema public` | hyperfleet-api | IAM user lacks PostgreSQL-level grants | Step 1.3 — grant DB/schema/table privileges |
| `database "hyperfleet" does not exist` | hyperfleet-api | Database not created on Cloud SQL instance | `gcloud sql databases create hyperfleet --instance=mypsql` |
| `password authentication failed for user "..."` | hyperfleet-api | IAM DB user not created or wrong username format | Step 1.2 — create with `@project.iam` suffix |
| `PERMISSION_DENIED: cloudsql.instances.connect` | cloud-sql-proxy | GCP SA missing `roles/cloudsql.client` | Step 1.1 |

---

## Checklist

- [ ] GCP service account created with both `roles/cloudsql.client` and `roles/cloudsql.instanceUser`
- [ ] Cloud SQL IAM database user created (`CLOUD_IAM_SERVICE_ACCOUNT` type, name: `sa@project.iam`)
- [ ] PostgreSQL-level grants applied (`GRANT cloudsqlsuperuser` or schema/table grants + `ALTER DEFAULT PRIVILEGES`)
- [ ] Workload Identity binding uses **`roles/iam.workloadIdentityUser`** (not `serviceAccountUser`)
- [ ] K8s ServiceAccount annotated with `iam.gke.io/gcp-service-account`
- [ ] Secret has `db.host=127.0.0.1` and a **non-empty** `db.password`
- [ ] Proxy configured under **`nativeSidecars`** (not `sidecars`) with `--auto-iam-authn` and correct instance connection name (`project:region:instance`)
- [ ] `database.external.enabled=true` and `secretName` pointing to the correct secret
- [ ] ConfigMap sets `database.ssl.mode: disable`
- [ ] Proxy logs show "The proxy has started successfully and is ready for new connections!"
- [ ] `psql` from a debug container connects to `localhost:5432` successfully
- [ ] All pod containers reach 2/2 Running, 0 restarts
