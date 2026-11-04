-- Phase 1: create the first administrator manually.
--
-- The server intentionally does not auto-create an administrator. Run this
-- script once against a database initialized with db/postgres.sql.
--
-- Usage:
--   read -r -s PASSWORD
--   HASH=$(printf '%s' "$PASSWORD" | go run ./cmd/hash-password)
--   psql "$DB" \
--     -v admin_email='admin@example.com' \
--     -v admin_display_name='Admin' \
--     -v admin_password_hash="$HASH" \
--     -f db/seed_admin.example.sql
--
-- The email/password login provider stores an Argon2id PHC hash. The helper
-- reads the password from stdin so it is not placed in shell arguments.

\set ON_ERROR_STOP on

INSERT INTO auth.auth_user (email, display_name, created_at, updated_at)
SELECT :'admin_email', :'admin_display_name', now(), now()
WHERE NOT EXISTS (
    SELECT 1 FROM auth.auth_user WHERE email = :'admin_email'
);

INSERT INTO auth.auth_user_provider
    (user_id, login_key, provider_type, provider_data, created_at, updated_at)
SELECT u.id,
       :'admin_email',
       'EmailPassword',
       jsonb_build_object('password', :'admin_password_hash'),
       now(),
       now()
FROM auth.auth_user u
WHERE u.email = :'admin_email'
  AND NOT EXISTS (
      SELECT 1
      FROM auth.auth_user_provider p
      WHERE p.user_id = u.id
        AND p.provider_type = 'EmailPassword'
  );

INSERT INTO auth.auth_admin (user_id, created_at, updated_at)
SELECT u.id, now(), now()
FROM auth.auth_user u
WHERE u.email = :'admin_email'
ON CONFLICT (user_id) DO NOTHING;
