# cloud-recon — reference manual

Per-provider enumeration recipes and posture checklists. Every command here is **read-only**; none mutates state. All credentialed commands assume operator-supplied, scoped keys for an authorized account (see SKILL.md hard gates). Write raw JSON to `/app/data/tasks/<task_id>/cloud-recon/`.

Identity-first, always:

```bash
aws sts get-caller-identity
az account show -o json
gcloud auth list ; gcloud config list --format=json
```

Confirm the returned account / subscription / project matches the authorized scope before anything else.

---

## A. AWS

### A1. External / anonymous

```bash
# Bucket name sweep (feed in-scope keywords only)
cloud_enum -k acme -k acme-corp -k acmeapp

# S3 public list / read (no credentials)
aws s3 ls s3://acme-backups --no-sign-request
curl -s https://acme-backups.s3.amazonaws.com/            # XML listing if public-list
curl -sI https://acme-backups.s3.amazonaws.com/<key>      # HEAD one object for public-read
# region redirect tells you the bucket exists even on 403:
curl -sI https://acme-backups.s3.amazonaws.com/

# Public service endpoints already found by external-infra, flagged cloud-native:
#   *.s3-website-<region>.amazonaws.com, *.cloudfront.net origins,
#   <id>.lambda-url.<region>.on.aws, public RDS/Redshift endpoints,
#   *.elasticbeanstalk.com, exposed ECR (public.ecr.aws)
nuclei -u https://host.acme.com -t cloud/ -t http/exposures/
```

Distinguish `NoSuchBucket` (does not exist → drop) from `AccessDenied` (exists, private → intel fact) from a successful list/HEAD (public → finding candidate, then confirm).

### A2. Credentialed read-only

```bash
prowler aws -M json-ocsf,html -o /app/data/tasks/<task_id>/cloud-recon/prowler-aws \
  --severity critical high
# scope to regions/services if the account is large:
prowler aws --region us-east-1 --service s3 iam ec2
scout aws --report-dir /app/data/tasks/<task_id>/cloud-recon/scout-aws
```

Manual read-only spot checks (no mutation):

```bash
aws s3api get-bucket-policy-status --bucket <b>          # IsPublic true/false
aws s3api get-public-access-block --bucket <b>
aws ec2 describe-security-groups --query \
  'SecurityGroups[?IpPermissions[?contains(to_string(IpRanges),`0.0.0.0/0`)]]'
aws ec2 describe-instances --query \
  'Reservations[].Instances[].{id:InstanceId,imds:MetadataOptions.HttpTokens}'  # "optional" = IMDSv1
aws iam list-users ; aws iam get-account-authorization-details   # policy shapes, trusts
aws ec2 describe-snapshots --owner-ids self --query 'Snapshots[?Public]'
aws rds describe-db-instances --query 'DBInstances[?PubliclyAccessible]'
```

### A3. AWS posture checklist

- [ ] **S3**: public-access-block off; bucket policy / ACL granting `*`; website hosting; unencrypted; no access logging.
- [ ] **IAM**: policies with `Action:"*"` / `Resource:"*"`; role trust policies with `Principal:"*"` or an external/unknown account; users without MFA; access keys older than the policy's rotation window; `iam:PassRole` paired with `ec2:RunInstances`/`lambda:CreateFunction`; known privesc shapes (`iam:CreatePolicyVersion`, `sts:AssumeRole` chains).
- [ ] **Network**: security groups open to `0.0.0.0/0` on 22/3389/3306/5432/6379/9200/27017; public subnets with sensitive workloads; public ALB/NLB fronting internal apps.
- [ ] **Metadata**: IMDSv1 enabled (`HttpTokens: optional`) → SSRF-to-credential risk; record location, hand exploitation to web-api-deep.
- [ ] **Data**: public EBS/RDS snapshots; public AMIs; unencrypted volumes/DBs; secrets in EC2 user-data or Lambda env vars.
- [ ] **Logging**: CloudTrail disabled / not multi-region / not validated; Config off; GuardDuty off.

---

## B. Azure

### B1. External / anonymous

```bash
cloud_enum -k acme -k acmecorp     # also sweeps *.blob.core.windows.net, *.azurewebsites.net

# Blob container anonymous list (if public)
curl -s "https://acmestorage.blob.core.windows.net/backups?restype=container&comp=list"
curl -sI "https://acmestorage.blob.core.windows.net/backups/<blob>"

# Public app/function/service hosts (flag, then feed to external-infra for TLS/port):
#   *.azurewebsites.net, *.cloudapp.azure.com, *.cosmos.azure.com,
#   *.database.windows.net, *.azurecr.io, *.blob.core.windows.net
nuclei -u https://acme.azurewebsites.net -t cloud/ -t http/exposures/
```

### B2. Credentialed read-only

```bash
prowler azure --subscription-id <sub> \
  -M json-ocsf,html -o /app/data/tasks/<task_id>/cloud-recon/prowler-azure
scout azure --cli --report-dir /app/data/tasks/<task_id>/cloud-recon/scout-azure

# Manual read-only spot checks
az storage account list --query "[].{n:name,blobPublic:allowBlobPublicAccess,https:enableHttpsTrafficOnly}"
az network nsg list --query "[].securityRules[?sourceAddressPrefix=='*' && direction=='Inbound']"
az role assignment list --all --query "[?roleDefinitionName=='Owner' || roleDefinitionName=='Contributor']"
az ad sp list --all --query "[].{id:appId,name:displayName}"   # service principals
```

### B3. Azure posture checklist

- [ ] **Storage**: `allowBlobPublicAccess` true; anonymous container/blob access; no secure-transfer-required; shared-key access enabled; SAS tokens over-broad.
- [ ] **IAM / Entra ID**: Owner/Contributor assigned broadly or to guests; service principals with excessive roles; no Conditional Access / MFA; legacy auth allowed; custom roles with `*` actions.
- [ ] **Network**: NSG inbound rules from `*`/`Internet` on 22/3389/1433/3306/5432/6379; public IPs on sensitive VMs; Azure SQL / Cosmos / Postgres firewall allowing `0.0.0.0`.
- [ ] **Compute / metadata**: IMDS reachable via SSRF (record location); secrets in VM custom-data / App Service settings.
- [ ] **Data**: SQL/Cosmos TDE off; disks unencrypted; Key Vault with public network access or permissive access policies.
- [ ] **Logging**: Activity Log / diagnostic settings off; Defender for Cloud off; no log retention.

---

## C. GCP

### C1. External / anonymous

```bash
cloud_enum -k acme -k acme-prod    # sweeps storage.googleapis.com, *.appspot.com, *.run.app

# GCS bucket anonymous list / read
curl -s "https://storage.googleapis.com/acme-assets"            # XML listing if public
gcloud storage ls gs://acme-assets                              # with no active auth
curl -sI "https://storage.googleapis.com/acme-assets/<object>"

# Public service hosts (flag, feed to external-infra):
#   *.appspot.com, *.run.app (Cloud Run), *.cloudfunctions.net,
#   *.googleapis.com endpoints, public Cloud SQL IPs, *.firebaseio.com / *.firebaseapp.com
nuclei -u https://acme.appspot.com -t cloud/ -t http/exposures/
# Firebase open DB classic check:
curl -s "https://acme.firebaseio.com/.json"
```

### C2. Credentialed read-only

```bash
prowler gcp --project-id acme-prod \
  -M json-ocsf,html -o /app/data/tasks/<task_id>/cloud-recon/prowler-gcp
scout gcp --service-account /path/key.json --report-dir \
  /app/data/tasks/<task_id>/cloud-recon/scout-gcp

# Manual read-only spot checks
gcloud storage buckets list --format=json
gcloud projects get-iam-policy acme-prod --format=json       # allUsers / allAuthenticatedUsers
gcloud compute firewall-rules list \
  --filter="sourceRanges:0.0.0.0/0" --format=json
gcloud iam service-accounts list --project acme-prod
gcloud compute instances list --format="value(name,metadata)"  # serial-port, secrets
```

### C3. GCP posture checklist

- [ ] **GCS**: IAM binding granting `allUsers` / `allAuthenticatedUsers`; uniform-bucket-level-access off; public legacy ACLs; no bucket logging.
- [ ] **IAM**: `allUsers`/`allAuthenticatedUsers` in any binding; primitive roles (`Owner`/`Editor`) on users or service accounts; service-account keys (user-managed) present and unrotated; `iam.serviceAccountUser` / `actAs` + compute pairings; `setIamPolicy`/privesc roles.
- [ ] **Network**: firewall rules `0.0.0.0/0` on 22/3389/3306/5432/6379/9200; default network in use; public IPs on sensitive VMs; Cloud SQL with public IP and `0.0.0.0/0` authorized networks.
- [ ] **Compute / metadata**: legacy metadata endpoints enabled; OS Login off; serial-port access on; secrets in instance metadata; default service account with broad scopes.
- [ ] **Data**: Cloud SQL without SSL; unencrypted (CMEK) where policy requires; public BigQuery datasets; Secret Manager bindings to `allUsers`.
- [ ] **Logging**: Audit Logs (Admin/Data Access) not enabled; log sinks missing; no retention.

---

## D. Cross-provider name permutation seeds

Build candidate resource names from in-scope identifiers only:

```
<org>, <org>-<env>, <env>-<org>, <product>, <brand>
env ∈ {prod, dev, stage, staging, test, qa, uat}
suffix ∈ {backups, backup, assets, static, media, logs, data, db, dump,
          archive, cdn, public, private, internal, tf, terraform, state,
          secrets, config, uploads, images, files, reports}
```

Feed these to `cloud_enum` and the manual bucket probes. Do **not** add names belonging to other organizations, and stop name-probing once the authorized namespace is covered.

---

## E. Severity reasoning (no exercise required)

| Observation | Confirmed by | Severity driver |
|---|---|---|
| Public-read bucket with sensitive object names | anonymous HEAD/list success | data sensitivity + record count (denominator) |
| Public-list-only bucket, no sensitive data | anonymous list success | low/info unless names leak internal structure |
| Role trust policy `Principal:"*"` | reading the policy document | account-takeover potential; proof basis `policy-read, not-exercised` |
| IMDSv1 enabled | describe-instances | conditional on an SSRF primitive existing elsewhere; medium, note dependency |
| Security group `0.0.0.0/0` to 3306 | describe-security-groups + a reachable listener | confirm the port is actually open via external-infra before raising |
| Logging disabled | read config | hardening/compliance note, usually not a standalone exploit finding |

Never raise severity by actually using a privilege. A capability is demonstrated by the read policy plus a clear, written exploitation path, with the finding marked `not-exercised`.

---

## F. Troubleshooting

| Symptom | Cause → fix |
|---|---|
| `AccessDenied` on every anonymous S3 probe | Bucket exists but is private (good posture) → record as intel, not a finding |
| Prowler emits thousands of rows | Triage with `--severity critical high` first; scope by `--service` / region |
| `cloud_enum` returns nothing | Keyword set too narrow → add product/brand permutations (section D), still in-scope only |
| Credentialed call hits a region/quota error | Scope to the authorized region; do not broaden the account filter |
| Tool not in image | Record unavailable; fall back to the manual CLI spot checks above |
| Caller identity ≠ authorized account | Stop immediately; do not enumerate; report the credential/scope mismatch |
| Proxy 403 on a provider API | See `/root/.ccr/README.md`; the aws/az/gcloud CLIs are pre-proxy-configured — prefer them over raw curl |
