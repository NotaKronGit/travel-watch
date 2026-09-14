\getenv owner_password CABINET_DATABASE_OWNER_PASSWORD
\getenv app_password CABINET_DATABASE_APP_PASSWORD
CREATE ROLE cabinet_owner LOGIN PASSWORD :'owner_password';
CREATE ROLE cabinet_app LOGIN PASSWORD :'app_password';
CREATE DATABASE cabinet OWNER cabinet_owner;
REVOKE ALL ON DATABASE cabinet FROM PUBLIC;
GRANT CONNECT ON DATABASE cabinet TO cabinet_owner, cabinet_app;
\connect cabinet
REVOKE ALL ON SCHEMA public FROM PUBLIC;
GRANT USAGE, CREATE ON SCHEMA public TO cabinet_owner;
GRANT USAGE ON SCHEMA public TO cabinet_app;
