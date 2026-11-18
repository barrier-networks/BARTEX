# external-infra — reference manual

Probing recipes, TLS checks, and exposure signatures. Everything here assumes the host/IP is **in scope and target-owned** (see SKILL.md gates) and that the mode (passive vs active) matches program policy. Write raw output to `/app/data/tasks/<task_id>/external-infra/`; keep conversation to summaries.

Pacing defaults for active scans: `-T3`, `--max-rate` tuned to the program limit, split into smaller scans rather than one broad loud pass (mirrors `bbot-recon`'s rate discipline).

---

## A. Live-host and port triage

```bash
# Verify a sample of discovery's OPEN_TCP_PORT events instead of re-scanning:
nmap -sV -Pn -p 443,8443,22,3389 --max-rate 50 host.acme.com -oA \
  /app/data/tasks/<task_id>/external-infra/nmap-sample

# Scoped service/version scan (top ports, CDN-aware, paced):
nmap -sV -Pn --top-ports 100 -T3 --max-rate 100 -iL hosts.txt -oA .../nmap-top100

# Fast port triage feeding httpx:
naabu -list hosts.txt -top-ports 1000 -rate 100 -o .../naabu.txt

# Passive (zero-touch): pull ports/banners by IP from enrichment sources, no packets sent.
shodan host 203.0.113.10
```

Interpretation: `open` = answer received; `filtered` = no answer (firewall) → intel, not a finding; distinguish a real listener from a CDN edge before claiming an exposed backend.

---

## B. Web fingerprint

```bash
httpx -list hosts.txt -sc -title -td -tls-grab -cdn -location -ip \
  -json -o /app/data/tasks/<task_id>/external-infra/httpx.json
# -td tech-detect, -tls-grab pulls SAN names, -cdn flags CDN/WAF fronting

# Non-HTTP services:
fingerprintx -l hostports.txt --json -o .../fingerprintx.json
```

Route each result: HTTP app/API → `web-api-deep`/`api-recon`; cloud-native host → `cloud-recon`; DB/RDP/SSH/mail/broker → Phase 4 exposure checks here.

---

## C. TLS / certificate hygiene

```bash
# Full TLS posture (protocols, ciphers, cert, known bugs) within budget:
testssl.sh --quiet --color 0 --jsonfile .../testssl.json https://host.acme.com:443
sslscan --no-colour host.acme.com:443

# Manual cert inspection:
echo | openssl s_client -connect host.acme.com:443 -servername host.acme.com 2>/dev/null \
  | openssl x509 -noout -subject -issuer -dates -ext subjectAltName
```

Checklist and severity drivers:

| Check | Weak result | Severity driver |
|---|---|---|
| Protocol versions | SSLv3 / TLS 1.0 / 1.1 enabled | config weakness; low/medium unless a usable attack (ROBOT/Heartbleed) is confirmed |
| Certificate validity | expired / self-signed on prod edge / CN-SAN mismatch | medium; MITM/trust impact |
| Key / signature | RSA < 2048, SHA-1 cert | config weakness |
| Ciphers | NULL/export/RC4/3DES, no forward secrecy | config weakness |
| Known bugs | Heartbleed, ROBOT, insecure reneg | finding only if reproduced; can leak memory/keys |
| **SAN names** | internal/extra hostnames in cert | **attack-surface intel** → feed back to `bbot-recon`/discovery, not a finding itself |

---

## D. Exposed admin / management / infra interfaces

Signature table — identify, do not exploit. Reachability + unauthenticated access (or a single documented-default where auth testing is permitted) is the finding.

| Interface | Typical port / path | Exposure signal |
|---|---|---|
| phpMyAdmin / Adminer | `/phpmyadmin`, `/adminer.php` | login page reachable from internet |
| Jenkins | 8080 `/`, `/login` | dashboard or `/script` reachable |
| GitLab / Gitea | 80/443 | sign-in + open registration / public repos |
| Grafana / Kibana / Prometheus | 3000 / 5601 / 9090 | dashboards or `/metrics` unauthenticated |
| Portainer / Docker API | 9000 / 2375 | UI reachable; **2375 unauth = critical** |
| Kubernetes API | 6443 / 10250 | anonymous `/version`, kubelet read |
| Databases | 3306/5432/1433/27017/6379/9200 | listener reachable from internet (should never be) |
| RDP / VNC / SMB | 3389 / 5900 / 445 | reachable from internet |
| Spring Boot Actuator | `/actuator`, `/actuator/env` | sensitive endpoints unauthenticated |
| Web server status | `/server-status`, `/server-info` | reachable |
| Source / config leak | `/.git/`, `/.env`, `/config.*`, dir listing | readable |

```bash
# Example reachability probes (single, read-only):
curl -sI https://host.acme.com/actuator/env
curl -s https://host.acme.com/.git/HEAD
redis-cli -h 203.0.113.10 ping          # PONG unauth = exposed Redis (one probe)
```

Default-credential policy: at most **one** vendor-documented default (e.g. `admin:admin` on a panel that ships with it), only where the program permits authentication testing, read-only, and stop on success — the exposure is proven; the attack belongs to `web-api-deep`.

---

## E. Targeted exposure checks (gated)

```bash
# Scope nuclei to detected tech + exposure/default-login, within the impact budget:
nuclei -list httpx_live.txt \
  -t http/exposures/ -t http/default-logins/ -t http/misconfiguration/ \
  -rl 50 -json -o /app/data/tasks/<task_id>/external-infra/nuclei.json
# Add -t cves/ only for the fingerprinted stack and only when the program allows CVE probing.

# Virtual-host discovery (LOUD — explicit permission only):
ffuf -u https://203.0.113.10/ -H "Host: FUZZ.acme.com" -w vhosts.txt -fs 0 -mc 200,301,302
```

Every nuclei/ffuf hit is a candidate. Re-issue the exact request by hand and capture the response before it becomes a `report_finding`.

---

## F. Passive mode (scanner ban / tight rate limit)

Target is never contacted. Enrich only:

```bash
shodan host 203.0.113.10
shodan search "ssl.cert.subject.cn:acme.com"
censys search "services.tls.certificates.leaf_data.subject.common_name: acme.com"
curl -s "https://crt.sh/?q=%25.acme.com&output=json" | jq -r '.[].name_value' | sort -u
```

Feed results to `insert_assets`/`record_fact` with `source=third_party_intel`; do not escalate to active probing without a policy change.

---

## G. Troubleshooting

| Symptom | Cause → fix |
|---|---|
| In-scope host resolves to a CDN/shared IP | Do not scan the IP; probe the app via hostname; confirm backend ownership before any direct probe |
| nmap all-filtered | Firewall/rate limit; lower rate, confirm liveness via httpx on web ports, or switch to passive |
| httpx flags everything as CDN | WAF/CDN fronting; findings about the origin need confirmed origin ownership |
| nuclei flood of low-confidence hits | Over-broad template set → scope to detected tech; verify each by hand |
| testssl.sh times out | Single host, raise timeout, or use `sslscan` for a quick protocol/cipher read |
| Redis/Mongo "exposed" but auth required | Not a finding → record as intel (service reachable, auth enforced) |
| Proxy/TLS errors on enrichment APIs | See `/root/.ccr/README.md`; prefer installed CLIs over raw curl |
| Overlap with bbot/scopesentry output | Deduplicate on host:port before `insert_assets`; one asset, merged tags |
