# Procédure d'installation — KubePilot

Ce document couvre trois scénarios d'installation :

1. [Développement local](#1-développement-local) — docker-compose, idéal pour tester et contribuer
2. [Déploiement in-cluster (GitOps)](#2-déploiement-in-cluster-gitops) — Jenkins → Harbor → ArgoCD, chart hébergé dans `kubepilot-gitops`
3. [Connexion de clusters supplémentaires](#3-connexion-de-clusters-supplémentaires) — intégrer d'autres clusters au cockpit

---

## Prérequis

| Outil | Version minimale | Usage |
|---|---|---|
| Go | 1.22 | Backend (dev local) |
| Node.js | 20 LTS | Frontend (dev local) |
| Docker + Docker Compose | 24+ | Stack locale |
| kubectl | 1.27+ | Interaction avec le cluster |
| helm | 3.12+ | Déploiement in-cluster |
| PostgreSQL | 15+ | Base de données (fournie via docker-compose ou Helm) |
| Redis | 7+ | Cache (fourni via docker-compose ou Helm) |

---

## 1. Développement local

### 1.1 Cloner le projet

```bash
git clone https://github.com/kubepilot/kubepilot.git
cd kubepilot
```

### 1.2 Démarrer l'infrastructure locale

```bash
make dev-infra
```

Cette commande démarre PostgreSQL 15 et Redis 7 via docker-compose.
La base de données est initialisée automatiquement avec le schéma depuis `backend/migrations/001_initial.sql`.

Vérifier que les services sont prêts :

```bash
docker compose ps
# postgres   running (healthy)
# redis      running (healthy)
```

### 1.3 Configurer le backend

```bash
cp backend/.env.example backend/.env
```

Les valeurs par défaut dans `.env.example` sont prêtes pour docker-compose. Seul `JWT_SECRET` doit être changé pour une vraie valeur en production.

Variables importantes :

| Variable | Défaut | Description |
|---|---|---|
| `DB_URL` | `postgres://kubepilot:kubepilot@localhost:5432/kubepilot?sslmode=disable` | URL de connexion PostgreSQL |
| `REDIS_URL` | `redis://localhost:6379` | URL Redis |
| `JWT_SECRET` | `change-me-in-production` | Clé de signature JWT — **à changer** |
| `ADMIN_EMAIL` | `admin@kubepilot.local` | Email du compte admin créé au premier démarrage |
| `ADMIN_PASSWORD` | *(vide)* | Mot de passe admin — auto-généré si vide |
| `IN_CLUSTER` | `false` | Passer à `true` uniquement si le backend tourne dans K8s |
| `CLUSTER_NAME` | `local` | Nom du cluster auto-enregistré en mode in-cluster |
| `HEADLAMP_URL` | `http://localhost:4466` | URL de votre instance Headlamp pour les deep links |

### 1.4 Démarrer le backend

```bash
make dev-backend
# Ou directement :
cd backend && go run ./cmd/server
```

Au premier démarrage, le bootstrap s'exécute automatiquement :
- Les environnements par défaut sont créés (prod / preprod / staging / dev)
- Le compte admin est créé

Si `ADMIN_PASSWORD` est vide dans le `.env`, le mot de passe auto-généré est affiché dans le terminal :

```
╔══════════════════════════════════════════════════╗
║         KubePilot — First-Run Setup              ║
╠══════════════════════════════════════════════════╣
║  Admin email    : admin@kubepilot.local          ║
║  Admin password : a3f8c2d1e9b4...               ║
╚══════════════════════════════════════════════════╝
```

Vérifier que le backend répond :

```bash
curl http://localhost:8080/health
# {"status":"ok"}
```

### 1.5 Démarrer le frontend

```bash
make dev-frontend
# Ou directement :
cd frontend && npm install && npm run dev
```

L'UI est disponible sur **http://localhost:3000**.

Le frontend proxifie `/api` et `/events` vers le backend sur le port 8080 (configuré dans `vite.config.ts`).

### 1.6 Premier login

Ouvrir http://localhost:3000, saisir les credentials du compte admin créé à l'étape 1.4.

Si aucun compte n'existe encore (base vide), l'UI propose un écran de setup initial accessible aussi via :

```bash
# Vérifier si le setup est nécessaire
curl http://localhost:8080/auth/setup
# {"setup_required":true}

# Créer le premier admin manuellement
curl -X POST http://localhost:8080/auth/setup \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@exemple.com","password":"MonMotDePasse","name":"Admin"}'
```

### 1.7 Connecter un cluster Kubernetes local

Pour monitorer un cluster depuis le dev local, renseigner le kubeconfig dans l'UI :

1. Ouvrir **Intégrations** dans la sidebar
2. Cliquer **Ajouter un cluster**
3. Renseigner :
   - Nom : `mon-cluster`
   - Environnement : `dev`
   - Kubeconfig : coller le contenu de `~/.kube/config` (ou le contenu encodé en base64)

Ou via l'API :

```bash
KUBECONFIG_B64=$(cat ~/.kube/config | base64 -w0)

curl -X POST http://localhost:8080/api/v1/clusters \
  -H "Authorization: Bearer <votre_token_jwt>" \
  -H "Content-Type: application/json" \
  -d "{
    \"name\": \"mon-cluster\",
    \"environment_id\": \"<uuid_env_dev>\",
    \"kubeconfig_ref\": \"$KUBECONFIG_B64\"
  }"
```

---

## 2. Déploiement in-cluster (GitOps)

> Le chart Helm **ne vit pas dans ce dépôt**. Il a été déplacé dans le dépôt GitOps
> [`kubepilot-gitops`](http://gitea.10.0.60.107.nip.io/vanti/kubepilot-gitops) (`charts/kubepilot/`),
> qui porte aussi les valeurs par environnement et les Applications ArgoCD.
> Une seule source de vérité : ne pas recréer de chart ici.

### 2.1 Chaîne de déploiement

```
push sur dev/staging  ─▶  Jenkins  ─▶  build backend + frontend (parallèle)
                                    ─▶  push Harbor  harbor.<domaine>/kubepilot/{backend,frontend}:<tag>
                                    ─▶  sed du tag dans kubepilot-gitops  envs/<env>/values.yaml
                                                                    │
                                                          ArgoCD ◀──┘  sync  ─▶  cluster
```

| Branche source | Tag d'image | Values GitOps | Branche GitOps |
|---|---|---|---|
| `dev` | `v<VERSION>-alpha.<BUILD>` | `envs/dev/values.yaml` | `dev` |
| `staging` | `v<VERSION>-beta.<BUILD>` | `envs/staging/values.yaml` | `staging` |

`VERSION` (à la racine de ce dépôt) fournit le `MAJOR.MINOR.PATCH` ; Jenkins y accole le
numéro de build. Le pipeline est défini dans `kubepilot-gitops/ci/Jenkinsfile`.

### 2.2 Déployer une nouvelle version

Rien à faire manuellement : pousser sur `dev` suffit. Pour forcer un déploiement sans
nouveau build, modifier le tag dans `envs/<env>/values.yaml` du dépôt GitOps — ArgoCD
synchronise.

### 2.3 Adapter la configuration

Les réglages spécifiques à un environnement vont dans `envs/<env>/values.yaml`
(`config.*`, `ingress.*`, `resources.*`, `imagePullSecrets`…). Les réglages structurels
vont dans `charts/kubepilot/values.yaml`.

### 2.4 Droits RBAC accordés par le chart

| Ressource | Verbes | Pourquoi |
|---|---|---|
| `namespaces`, `nodes`, `pods`, `services`, `secrets`, `configmaps` | get, list, watch | Inventaire. Les `secrets` sont indispensables : Helm 3 y stocke chaque release, c'est la seule source des charts installés |
| `deployments`, `daemonsets`, `statefulsets`, `replicasets` (apps) | get, list, watch | Workloads et images |
| `cronjobs`, `jobs` (batch), `ingresses` (networking) | get, list, watch | Inventaire |
| `nodes/proxy`, `nodes/stats`, `nodes/metrics` | get | Métriques CPU/RAM/disque via le Summary API du kubelet (agentless) |

⚠️ `nodes/proxy` est **obligatoire pour les métriques nœuds** : le collector lit
`/api/v1/nodes/<name>/proxy/stats/summary`, il n'utilise **pas** l'API `metrics.k8s.io`.
Sans ce droit les jauges restent vides sans erreur visible. Désactivable via
`rbac.nodeMetrics=false` si la politique du cluster l'interdit.

### 2.5 Récupérer le mot de passe admin auto-généré

Si `admin.password` est vide dans les values de l'environnement :

```bash
kubectl logs -n kubepilot-dev deploy/kubepilot-backend | grep -A 5 "First-Run Setup"
```

### 2.6 Vérifier l'état du déploiement

```bash
kubectl get pods -n kubepilot-dev
kubectl logs -n kubepilot-dev deploy/kubepilot-backend --tail=50
kubectl exec -n kubepilot-dev deploy/kubepilot-backend -- wget -qO- http://localhost:8080/health/ready
```

Côté ArgoCD, l'état de synchronisation se lit sur l'Application définie dans
`kubepilot-gitops/argocd/app-<env>.yaml`.


## 3. Connexion de clusters supplémentaires

KubePilot peut surveiller plusieurs clusters en parallèle. Chaque cluster nécessite un ServiceAccount avec les droits lecture sur les ressources K8s.

### 3.1 Créer le ServiceAccount d'intégration sur le cluster distant

Sur le cluster distant (pas celui où KubePilot est déployé) :

```bash
# Appliquer le manifeste d'intégration (contenu dans le chart Helm)
kubectl apply -f - <<'EOF'
apiVersion: v1
kind: ServiceAccount
metadata:
  name: kubepilot-integration
  namespace: kube-system
---
apiVersion: v1
kind: Secret
metadata:
  name: kubepilot-integration-token
  namespace: kube-system
  annotations:
    kubernetes.io/service-account.name: kubepilot-integration
type: kubernetes.io/service-account-token
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kubepilot-integration
rules:
  - apiGroups: [""]
    resources: ["namespaces", "nodes", "pods", "services", "secrets", "configmaps", "events"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["apps"]
    resources: ["deployments", "daemonsets", "statefulsets", "replicasets"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["batch"]
    resources: ["cronjobs", "jobs"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["networking.k8s.io"]
    resources: ["ingresses"]
    verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: kubepilot-integration
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: kubepilot-integration
subjects:
  - kind: ServiceAccount
    name: kubepilot-integration
    namespace: kube-system
EOF
```

### 3.2 Récupérer le token et l'URL API

```bash
# Token
TOKEN=$(kubectl get secret kubepilot-integration-token \
  -n kube-system \
  -o jsonpath='{.data.token}' | base64 -d)

# URL de l'API server
API_URL=$(kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}')

echo "Token : $TOKEN"
echo "API URL : $API_URL"
```

### 3.3 Enregistrer le cluster dans KubePilot

**Via l'UI :**

1. Ouvrir KubePilot → **Intégrations**
2. Cliquer **Ajouter un cluster**
3. Renseigner :
   - **Nom** : `cluster-prod-eu` (identifiant unique)
   - **Environnement** : `Production`
   - **URL API** : valeur de `$API_URL`
   - **Token** : valeur de `$TOKEN`
   - **TLS Insecure** : cocher seulement si le cluster utilise un certificat auto-signé

**Via l'API REST :**

```bash
# Récupérer d'abord l'UUID de l'environnement prod
curl -s http://localhost:8080/api/v1/clusters \
  -H "Authorization: Bearer $JWT" | jq

# Créer le cluster
curl -X POST http://localhost:8080/api/v1/clusters \
  -H "Authorization: Bearer $JWT" \
  -H "Content-Type: application/json" \
  -d "{
    \"name\": \"cluster-prod-eu\",
    \"environment_id\": \"<uuid-env-prod>\",
    \"api_endpoint\": \"$API_URL\",
    \"kubeconfig_ref\": \"\",
    \"tls_insecure\": false
  }"
```

> **Note** : pour un token bearer sans kubeconfig complet, configurer `kubeconfig_ref` comme un kubeconfig minimal :
>
> ```bash
> cat <<EOF | base64 -w0
> apiVersion: v1
> kind: Config
> clusters:
> - cluster:
>     server: $API_URL
>   name: remote
> users:
> - name: kubepilot
>   user:
>     token: $TOKEN
> contexts:
> - context:
>     cluster: remote
>     user: kubepilot
>   name: remote
> current-context: remote
> EOF
> ```

### 3.4 Vérifier la collecte

Après enregistrement, le collector démarre dans les 60 secondes. Vérifier dans les logs :

```bash
kubectl logs -n kubepilot deploy/kubepilot-backend \
  | grep "collector started"
# {"level":"info","ts":"...","msg":"collector started","cluster_name":"cluster-prod-eu"}
```

L'état du cluster passe de `unknown` à `healthy` dans l'UI Overview.

---

## 4. Token d'intégration du cluster local (Helm)

Si KubePilot est installé via Helm avec `integrationToken.create: true` (défaut), un token long-lived est créé automatiquement dans le namespace `kubepilot`.

```bash
# Récupérer le token du cluster local
kubectl get secret kubepilot-integration-token \
  -n kubepilot \
  -o jsonpath='{.data.token}' | base64 -d
```

Ce token peut être réutilisé pour connecter ce même cluster depuis une autre instance KubePilot, ou pour des outils tiers nécessitant un accès lecture au cluster.

---

## 5. Référence des variables d'environnement

| Variable | Défaut | Obligatoire | Description |
|---|---|---|---|
| `DB_URL` | — | Si `STORAGE_DRIVER=postgres` | URL de connexion PostgreSQL |
| `STORAGE_DRIVER` | dérivé | Non | `postgres` \| `sqlite` — SQLite si `DB_URL` vide ou `LOCAL_MODE=true` |
| `SQLITE_PATH` | `kubepilot.db` | Non | Fichier SQLite (driver `sqlite`) |
| `CACHE_DRIVER` | dérivé | Non | `redis` \| `memory` — mémoire dès que le stockage est SQLite |
| `REDIS_URL` | `redis://localhost:6379` | Non | URL Redis |
| `HELM_AUTODISCOVER` | `true` | Non | Repli sur Artifact Hub pour retrouver le dépôt d'un chart installé — `false` = résolution hors-ligne uniquement |
| `JWT_SECRET` | `change-me-in-production` | **Oui en prod** | Clé de signature JWT (min. 32 chars) |
| `PORT` | `8080` | Non | Port d'écoute HTTP |
| `LOG_LEVEL` | `info` | Non | `debug` \| `info` \| `warn` \| `error` |
| `WORKER_INTERVAL_SECONDS` | `300` | Non | Intervalle de polling registry + Helm (secondes) |
| `HEADLAMP_URL` | — | Non | URL de base Headlamp pour les deep links |
| `TLS_INSECURE` | `false` | Non | Désactiver la vérification TLS des clusters |
| `ADMIN_EMAIL` | `admin@kubepilot.local` | Non | Email du compte admin créé au bootstrap |
| `ADMIN_PASSWORD` | — | Non | Mot de passe admin — auto-généré si vide |
| `ADMIN_NAME` | `Administrator` | Non | Nom d'affichage de l'admin |
| `IN_CLUSTER` | `false` | Non | Auto-enregistrement du cluster local (Helm le passe à `true`) |
| `CLUSTER_NAME` | `local` | Non | Nom du cluster auto-enregistré |

---

## 6. Dépannage courant

### Le backend ne démarre pas — erreur de connexion PostgreSQL

```
failed to connect to postgres after 10 attempts
```

Vérifier que PostgreSQL est accessible :
```bash
docker compose ps          # dev local
kubectl get pods -n kubepilot  # in-cluster
```

Vérifier que `DB_URL` est correcte (utilisateur, mot de passe, nom de base, host).

### Le collector K8s ne démarre pas

```
build rest config for cluster ...: ...
```

En mode développement local (`IN_CLUSTER=false`), le collector utilise le kubeconfig fourni lors de l'enregistrement du cluster. Vérifier que le kubeconfig est valide :
```bash
kubectl --kubeconfig <chemin> get namespaces
```

### Les mises à jour d'images ne sont pas détectées

1. Vérifier que le registry est accessible depuis le backend (réseau, firewall)
2. Vérifier les logs du watcher :
   ```bash
   kubectl logs -n kubepilot deploy/kubepilot-backend | grep "image_watcher"
   ```
3. Si le registry est privé, enregistrer les credentials dans **Intégrations → Registries**

### SSE (temps réel) ne fonctionne pas derrière un proxy

Ajouter ces annotations Ingress pour Nginx :
```yaml
nginx.ingress.kubernetes.io/proxy-read-timeout: "3600"
nginx.ingress.kubernetes.io/proxy-buffering: "off"
```

Pour Traefik, ajouter le middleware `buffering` désactivé.

### Récupérer les logs complets

```bash
# Backend
kubectl logs -n kubepilot deploy/kubepilot-backend --previous  # si pod redémarré
kubectl logs -n kubepilot deploy/kubepilot-backend --tail=200

# Frontend (nginx)
kubectl logs -n kubepilot deploy/kubepilot-frontend
```
