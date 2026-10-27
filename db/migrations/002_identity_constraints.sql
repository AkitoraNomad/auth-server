-- Identity uniqueness and registration race protection.
-- Apply after 001_phase1_admin_spa.sql:
--   psql "$DB" -f db/migrations/002_identity_constraints.sql
--
-- The lower(email) index makes case-variant email registration deterministic;
-- the provider index guarantees one local account per external identity and
-- closes the check-then-insert race in registration.

BEGIN;

CREATE UNIQUE INDEX IF NOT EXISTS auth_user_email_lower_key
    ON auth.auth_user (lower(email))
    WHERE email IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS auth_user_provider_type_login_key
    ON auth.auth_user_provider (provider_type, login_key);

-- Disable the legacy implicit grant for existing clients. The server no longer
-- accepts response_type=token.
UPDATE auth.auth_client
SET grant_types = array_remove(grant_types, 'implicit')
WHERE 'implicit' = ANY(grant_types);

COMMIT;
