-- +goose Up
CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email text NOT NULL UNIQUE CHECK (email = lower(email) AND length(email) <= 254),
    password_hash text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE sessions (
    token_hash bytea PRIMARY KEY CHECK (octet_length(token_hash) = 32),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL CHECK (expires_at > created_at)
);
CREATE INDEX sessions_user_id_idx ON sessions(user_id);
CREATE INDEX sessions_expires_at_idx ON sessions(expires_at);
GRANT SELECT, INSERT ON users TO cabinet_app;
GRANT SELECT, INSERT, DELETE ON sessions TO cabinet_app;

-- +goose Down
DROP TABLE sessions;
DROP TABLE users;
