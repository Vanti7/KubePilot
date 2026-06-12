# Mode local single-binary & serveur MCP

Deux fonctionnalités complémentaires pour utiliser KubePilot depuis un poste de
travail, sans déployer PostgreSQL ni Redis.

---

## 1. Mode local (single-binary)

Le mode local fait tourner KubePilot avec **SQLite** (fichier sur disque) et un
**cache en mémoire**, et enregistre automatiquement le cluster pointé par votre
kubeconfig. Idéal pour évaluer KubePilot ou déboguer en local.

### Lancement

```bash
# Build (CGO non requis : driver SQLite pur-Go)
cd backend
go mod tidy           # nécessaire : nouvelles dépendances + go.sum
go build -o kubepilot ./cmd/server

# Lancer en mode local
LOCAL_MODE=true ./kubepilot
```

KubePilot :
1. crée/ouvre `kubepilot.db` (SQLite) et applique le schéma via `AutoMigrate` ;
2. seed les environnements + crée l'admin (mot de passe affiché dans les logs) ;
3. enregistre le cluster courant depuis `~/.kube/config` (ou `KUBECONFIG_PATH`) ;
4. démarre l'API sur `:8080` + collector + watchers + scoring.

### Variables d'environnement

| Variable | Défaut | Rôle |
|---|---|---|
| `LOCAL_MODE` | `false` | Active SQLite + mémoire + cluster depuis kubeconfig |
| `STORAGE_DRIVER` | auto | `postgres` ou `sqlite` (force le driver) |
| `SQLITE_PATH` | `kubepilot.db` | Chemin du fichier SQLite |
| `CACHE_DRIVER` | auto | `redis` ou `memory` |
| `KUBECONFIG_PATH` | *(règles par défaut)* | Kubeconfig explicite pour l'auto-enregistrement |

> **Défauts dérivés** : si `DB_URL` est vide *ou* `LOCAL_MODE=true`, le driver
> de stockage est SQLite et le cache est en mémoire. Sinon PostgreSQL + Redis
> (comportement de production inchangé).

### Notes d'implémentation

- Les UUID sont générés côté Go (callback GORM `BeforeCreate`), donc le schéma
  est identique sur PostgreSQL et SQLite — aucune dépendance à `gen_random_uuid()`.
- SQLite est ouvert en `MaxOpenConns(1)` + `busy_timeout` + WAL pour éviter les
  erreurs « database is locked » sous écritures concurrentes (collector + watchers).

---

## 1bis. Connexion cluster par SSH

Quand l'API server du cluster n'est **pas joignable directement** depuis ton poste
(firewall DSI sur 6443/443), KubePilot peut passer par SSH : il se connecte à un
nœud, lit son kubeconfig, puis **fait transiter tout le trafic Kubernetes dans la
connexion SSH**. Le kubeconfig du nœud pointe typiquement vers `127.0.0.1:6443` —
injoignable depuis ton PC, mais joignable *depuis le nœud*, donc via le tunnel.

```
Ton PC ──SSH──► nœud control-plane ──127.0.0.1:6443──► API server
        (auth mot de passe)        (tunnel direct-tcpip)
```

### Lancement (mode local)

```bash
LOCAL_MODE=true \
SSH_HOST=10.0.0.12 \
SSH_USER=ubuntu \
SSH_PASSWORD='********' \
SSH_SUDO=true \
./kubepilot
```

Au démarrage, le cluster est enregistré en mode `ssh` ; le collector se connecte,
récupère le kubeconfig et commence la collecte à travers le tunnel.

### Variables d'environnement

| Variable | Défaut | Rôle |
|---|---|---|
| `SSH_HOST` | — | Hôte/nœud SSH (active le mode ssh quand défini) |
| `SSH_PORT` | `22` | Port SSH |
| `SSH_USER` | — | Utilisateur SSH |
| `SSH_PASSWORD` | — | Mot de passe SSH (jamais renvoyé en API) |
| `SSH_KUBECONFIG_PATH` | *(auto)* | Chemin du kubeconfig sur le nœud |
| `SSH_SUDO` | `false` | Lire le kubeconfig via `sudo -S cat` (mot de passe injecté) |

Si `SSH_KUBECONFIG_PATH` n'est pas fourni, KubePilot essaie dans l'ordre :
`/etc/rancher/k3s/k3s.yaml`, `~/.kube/config`, `/etc/kubernetes/admin.conf`.

### Limites / sécurité (alpha)

- **Auth par mot de passe uniquement** (clé privée / agent : à venir).
- **Clé d'hôte non épinglée** (`InsecureIgnoreHostKey`) — réservé à un réseau de
  confiance ou un bastion. Le pinning known_hosts est à ajouter avant prod.
- Le mot de passe est stocké avec le cluster (cohérent avec le stockage actuel du
  kubeconfig en clair) ; champ `json:"-"`, jamais exposé via l'API.

---

## 2. Serveur MCP

Le **Model Context Protocol** permet à un assistant IA (Claude, Cursor, Copilot…)
d'interroger KubePilot en langage naturel. KubePilot expose un serveur MCP sur
**stdio**, en JSON-RPC 2.0.

### Lancement

```bash
./kubepilot mcp                 # lecture seule
MCP_ALLOW_WRITES=true ./kubepilot mcp   # autorise set_finding_status
```

Le serveur MCP partage le même stockage que le serveur HTTP (même `kubepilot.db`
en mode local, ou la même base PostgreSQL en production).

### Configuration côté client (Claude Desktop)

```json
{
  "mcpServers": {
    "kubepilot": {
      "command": "/chemin/vers/kubepilot",
      "args": ["mcp"],
      "env": { "LOCAL_MODE": "true" }
    }
  }
}
```

### Outils exposés

| Outil | Type | Description |
|---|---|---|
| `list_clusters` | lecture | Liste les clusters enregistrés (env + statut) |
| `list_findings` | lecture | Findings triés par score ; filtres cluster/severity/status/kind/limit |
| `get_finding` | lecture | Un finding par UUID + son score de risque détaillé |
| `findings_summary` | lecture | Compte des findings ouverts par sévérité (scope cluster optionnel) |
| `top_risks` | lecture | Les N findings ouverts les plus à risque |
| `set_finding_status` | écriture | Change le statut d'un finding (si `MCP_ALLOW_WRITES=true`) |

`list_findings` et `findings_summary` acceptent un nom de cluster *ou* un UUID
(résolution automatique du nom/slug).

### Exemple d'usage

> « Quelles sont les 5 mises à jour les plus critiques sur le cluster prod ? »

L'assistant appelle `list_findings(cluster="prod", severity="critical", limit=5)`
et renvoie les findings priorisés avec leurs scores.

### Limites actuelles

- Transport **stdio uniquement** (clients locaux / desktop). Un transport HTTP
  (Streamable HTTP) pour exposer MCP depuis le serveur in-cluster est prévu.
- Pas d'auth au niveau MCP : la portée d'accès est celle du processus
  (`MCP_ALLOW_WRITES` gate les écritures).
