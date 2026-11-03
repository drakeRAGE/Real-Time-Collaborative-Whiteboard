-- Scratch postgres:16 has no Supabase roles. Create them before golang-migrate.
-- CREATE ROLE cannot run inside a migration transaction. service_role is
-- NOLOGIN, matching docs/SCHEMA.md. Local services connect as the postgres
-- superuser (same as the integration tests), which bypasses FORCE RLS.
CREATE ROLE anon NOLOGIN NOINHERIT;
CREATE ROLE authenticated NOLOGIN NOINHERIT;
CREATE ROLE service_role NOLOGIN NOINHERIT BYPASSRLS;
