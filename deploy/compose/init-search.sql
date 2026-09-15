\getenv owner_password SEARCH_DATABASE_OWNER_PASSWORD
\getenv app_password SEARCH_DATABASE_APP_PASSWORD
SELECT format('CREATE ROLE search_owner LOGIN PASSWORD %L', :'owner_password')
WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname='search_owner') \gexec
SELECT format('CREATE ROLE search_app LOGIN PASSWORD %L', :'app_password')
WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname='search_app') \gexec
SELECT 'CREATE DATABASE search OWNER search_owner'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname='search') \gexec
REVOKE ALL ON DATABASE search FROM PUBLIC;
GRANT CONNECT ON DATABASE search TO search_owner, search_app;
\connect search
REVOKE ALL ON SCHEMA public FROM PUBLIC;
GRANT USAGE, CREATE ON SCHEMA public TO search_owner;
GRANT USAGE ON SCHEMA public TO search_app;
