// Package storage owns Cabinet's PostgreSQL queries and transaction boundaries.
package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrNotFound    = errors.New("record not found")
	ErrEmailExists = errors.New("email already exists")
	postgres       = goqu.Dialect("postgres")
)

type User struct {
	ID           string
	Email        string
	PasswordHash string
	CreatedAt    time.Time
}

type Session struct {
	TokenHash []byte
	ExpiresAt time.Time
}

type Store struct{ db *sql.DB }

func New(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

func (s *Store) FindUserByEmail(ctx context.Context, email string) (User, error) {
	query, args, err := postgres.From("users").Select("id", "email", "password_hash", "created_at").Where(goqu.Ex{"email": email}).Prepared(true).ToSQL()
	if err != nil {
		return User{}, err
	}
	var user User
	err = s.db.QueryRowContext(ctx, query, args...).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return user, err
}

func (s *Store) FindUserBySession(ctx context.Context, hash []byte) (User, error) {
	query, args, err := postgres.From(goqu.T("sessions").As("s")).
		Join(goqu.T("users").As("u"), goqu.On(goqu.I("u.id").Eq(goqu.I("s.user_id")))).
		Select(goqu.I("u.id"), goqu.I("u.email"), goqu.I("u.created_at")).
		Where(goqu.I("s.token_hash").Eq(hash), goqu.I("s.expires_at").Gt(goqu.L("now()"))).Prepared(true).ToSQL()
	if err != nil {
		return User{}, err
	}
	var user User
	err = s.db.QueryRowContext(ctx, query, args...).Scan(&user.ID, &user.Email, &user.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return user, err
}

// RegisterWithSession commits the user and session together. A failed session
// write must neither leave a user behind nor invalidate the old browser session.
func (s *Store) RegisterWithSession(ctx context.Context, email, passwordHash string, session Session, oldHash []byte) (User, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, err
	}
	defer func() { _ = tx.Rollback() }()
	query, args, err := postgres.Insert("users").Rows(goqu.Record{"email": email, "password_hash": passwordHash}).Returning("id", "created_at").Prepared(true).ToSQL()
	if err != nil {
		return User{}, err
	}
	user := User{Email: email}
	if err = tx.QueryRowContext(ctx, query, args...).Scan(&user.ID, &user.CreatedAt); err != nil {
		if pg, ok := errors.AsType[*pgconn.PgError](err); ok && pg.Code == "23505" {
			return User{}, ErrEmailExists
		}
		return User{}, err
	}
	if err = replaceSession(ctx, tx, user.ID, session, oldHash); err != nil {
		return User{}, err
	}
	if err = tx.Commit(); err != nil {
		return User{}, err
	}
	return user, nil
}

func (s *Store) ReplaceSession(ctx context.Context, userID string, session Session, oldHash []byte) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err = replaceSession(ctx, tx, userID, session, oldHash); err != nil {
		return err
	}
	return tx.Commit()
}

func replaceSession(ctx context.Context, tx *sql.Tx, userID string, session Session, oldHash []byte) error {
	if len(oldHash) > 0 {
		query, args, err := postgres.Delete("sessions").Where(goqu.Ex{"token_hash": oldHash}).Prepared(true).ToSQL()
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, query, args...); err != nil {
			return err
		}
	}
	query, args, err := postgres.Insert("sessions").Rows(goqu.Record{"token_hash": session.TokenHash, "user_id": userID, "expires_at": session.ExpiresAt}).Prepared(true).ToSQL()
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, query, args...)
	return err
}

func (s *Store) DeleteSession(ctx context.Context, hash []byte) error {
	query, args, err := postgres.Delete("sessions").Where(goqu.Ex{"token_hash": hash}).Prepared(true).ToSQL()
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *Store) DeleteExpiredSessions(ctx context.Context) error {
	query, args, err := postgres.Delete("sessions").Where(goqu.C("expires_at").Lte(goqu.L("now()"))).Prepared(true).ToSQL()
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, query, args...)
	return err
}

// ErrorKind exposes only a safe category, never SQL, parameters or credentials.
func ErrorKind(err error) string {
	if pg, ok := errors.AsType[*pgconn.PgError](err); ok {
		return pg.Code
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	return "internal"
}
