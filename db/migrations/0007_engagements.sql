-- Engagement / Rules-of-Engagement layer. Additive: a task with a NULL
-- engagement_id behaves exactly as before. An engagement groups tasks under a
-- client (company) with an authorised scope, a deny-list, a time window, an
-- intensity cap, a named authoriser and a signed-RoE reference. Enforcement is
-- additive to the guard/target gate (it can only subtract reach, never widen).

CREATE TABLE IF NOT EXISTS engagements (
    id                   BIGSERIAL PRIMARY KEY,
    company_id           BIGINT NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    name                 TEXT NOT NULL,
    status               TEXT NOT NULL DEFAULT 'draft'
                           CHECK (status IN ('draft','active','suspended','closed')),
    starts_at            TIMESTAMPTZ NOT NULL,
    ends_at              TIMESTAMPTZ NOT NULL,
    permitted_test_types JSONB NOT NULL DEFAULT '[]'::jsonb,
    intensity_cap        TEXT NOT NULL DEFAULT 'balanced'
                           CHECK (intensity_cap IN ('off','quiet','balanced','aggressive')),
    authorised_by_name   TEXT NOT NULL DEFAULT '',
    authorised_by_email  TEXT NOT NULL DEFAULT '',
    authorisation_ref    TEXT NOT NULL DEFAULT '',
    notes                TEXT NOT NULL DEFAULT '',
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (ends_at > starts_at)
);
CREATE INDEX IF NOT EXISTS idx_engagements_company ON engagements(company_id);
CREATE INDEX IF NOT EXISTS idx_engagements_active
    ON engagements(status, starts_at, ends_at) WHERE status = 'active';

DROP TRIGGER IF EXISTS trg_engagements_upd ON engagements;
CREATE TRIGGER trg_engagements_upd BEFORE UPDATE ON engagements
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Authorised scope and explicit exclusions. mode='allow' is the authorised
-- allow-list; mode='deny' is an explicit exclusion that always wins.
CREATE TABLE IF NOT EXISTS engagement_scope (
    id             BIGSERIAL PRIMARY KEY,
    engagement_id  BIGINT NOT NULL REFERENCES engagements(id) ON DELETE CASCADE,
    mode           TEXT NOT NULL CHECK (mode IN ('allow','deny')),
    kind           TEXT NOT NULL
                     CHECK (kind IN ('domain','subdomain','ip','cidr','cloud_account')),
    domain         TEXT,       -- domain / subdomain
    net            CIDR,       -- ip / cidr
    cloud_account  TEXT,       -- e.g. "aws:123456789012", "gcp:project-id", "azure:sub-guid"
    reason         TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_eng_scope_eng ON engagement_scope(engagement_id, mode);
CREATE INDEX IF NOT EXISTS idx_eng_scope_domain
    ON engagement_scope(domain) WHERE kind IN ('domain','subdomain');
CREATE INDEX IF NOT EXISTS idx_eng_scope_net
    ON engagement_scope USING GIST(net inet_ops) WHERE kind IN ('ip','cidr');
-- Dedup identical rows per engagement.
CREATE UNIQUE INDEX IF NOT EXISTS uq_eng_scope ON engagement_scope(
    engagement_id, mode, kind,
    COALESCE(domain,''), COALESCE(net::text,''), COALESCE(cloud_account,''));

-- Immutable-ish authorisation records (signed RoE, change authorisations).
CREATE TABLE IF NOT EXISTS engagement_authorisations (
    id              BIGSERIAL PRIMARY KEY,
    engagement_id   BIGINT NOT NULL REFERENCES engagements(id) ON DELETE CASCADE,
    kind            TEXT NOT NULL DEFAULT 'signed_roe'
                      CHECK (kind IN ('signed_roe','scope_change','window_extension','verbal_followed_up')),
    reference       TEXT NOT NULL,              -- document id / ticket / signature id
    authorised_by   TEXT NOT NULL,
    authorised_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    document_uri    TEXT,                       -- optional stored artefact
    recorded_by     TEXT NOT NULL DEFAULT '',   -- operator who entered it
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_eng_auth_eng ON engagement_authorisations(engagement_id, authorised_at);

-- Nullable FK on tasks: existing tasks keep working (engagement_id = NULL).
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS engagement_id BIGINT
    REFERENCES engagements(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_tasks_engagement
    ON tasks(engagement_id) WHERE engagement_id IS NOT NULL;
