//go:build integration

package storage

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/NotaKronGit/travel-watch/services/search/internal/config"
	"github.com/NotaKronGit/travel-watch/services/search/internal/consumer"
	"github.com/NotaKronGit/travel-watch/services/search/migrations"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
	"github.com/pressly/goose/v3"
)

func testDB(t *testing.T, ctx context.Context) (*sql.DB, *sql.DB) {
	t.Helper()
	env, err := godotenv.Read("../../../../.env")
	if err != nil {
		t.Fatal("test .env unavailable")
	}
	for _, key := range []string{"SEARCH_DATABASE_APP_PASSWORD", "SEARCH_DATABASE_OWNER_PASSWORD"} {
		t.Setenv(key, env[key])
	}
	c, err := config.Load("../../config.yaml", "migrate")
	if err != nil {
		t.Fatal(err)
	}
	if env["CABINET_DATABASE_PORT"] != "" {
		t.Setenv("SEARCH_DATABASE_PORT", env["CABINET_DATABASE_PORT"])
		c, err = config.Load("../../config.yaml", "migrate")
		if err != nil {
			t.Fatal(err)
		}
	}
	owner, err := sql.Open("pgx", c.Database.URL(true))
	if err != nil {
		t.Fatal(err)
	}
	schema := "test_search_" + uuid.New().String()
	schema = "\"" + schema + "\""
	if _, err = owner.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		_ = owner.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		clean, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		_, err := owner.ExecContext(clean, "DROP SCHEMA "+schema+" CASCADE")
		if err != nil {
			t.Error(err)
		}
		_ = owner.Close()
	})
	if _, err = owner.ExecContext(ctx, "GRANT USAGE ON SCHEMA "+schema+" TO search_app"); err != nil {
		t.Fatal(err)
	}
	open := func(migrate bool) *sql.DB {
		u, err := url.Parse(c.Database.URL(migrate))
		if err != nil {
			t.Fatal(err)
		}
		q := u.Query()
		q.Set("search_path", schema)
		u.RawQuery = q.Encode()
		db, err := sql.Open("pgx", u.String())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		return db
	}
	migrator, app := open(true), open(false)
	goose.SetBaseFS(migrations.Files)
	if err = goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err = goose.UpContext(ctx, migrator, "."); err != nil {
			t.Fatal(err)
		}
	}
	return migrator, app
}
func TestInbox(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	owner, db := testDB(t, ctx)
	s := New(db)
	if _, err := db.ExecContext(ctx, "CREATE TABLE forbidden(id int)"); err == nil {
		t.Fatal("app can migrate")
	}
	makeEvent := func(kind, id string) consumer.Event {
		return consumer.Event{ID: uuid.NewString(), RequestID: id, Type: kind, OccurredAt: time.Now().UTC(), Payload: []byte(kind + id)}
	}
	assertState := func(id, status string, hasCreation bool) {
		t.Helper()
		var got string
		var created bool
		if err := db.QueryRowContext(ctx, "SELECT status,created_payload IS NOT NULL FROM search_requests WHERE request_id=$1", id).Scan(&got, &created); err != nil {
			t.Fatal(err)
		}
		if got != status || created != hasCreation {
			t.Fatalf("state %s %v", got, created)
		}
	}
	for _, cancelFirst := range []bool{false, true} {
		id := uuid.NewString()
		create, stop := makeEvent(consumer.Created, id), makeEvent(consumer.Cancelled, id)
		first, second := create, stop
		if cancelFirst {
			first, second = stop, create
		}
		if err := s.Apply(ctx, first); err != nil {
			t.Fatal(err)
		}
		if cancelFirst {
			assertState(id, "cancelled", false)
		} else {
			assertState(id, "pending", true)
		}
		var wg sync.WaitGroup
		for range 4 {
			wg.Go(func() {
				if err := s.Apply(ctx, second); err != nil {
					t.Error(err)
				}
			})
		}
		wg.Wait()
		if err := New(db).Apply(ctx, create); err != nil {
			t.Fatal(err)
		}
		assertState(id, "cancelled", true)
		create.Payload = []byte("changed")
		if err := s.Apply(ctx, create); !errors.Is(err, ErrConflict) {
			t.Fatal("identity conflict accepted", err)
		}
	}
	e := makeEvent(consumer.Created, uuid.NewString())
	if _, err := owner.ExecContext(ctx, "ALTER TABLE search_requests ADD CONSTRAINT reject_test CHECK (false) NOT VALID"); err != nil {
		t.Fatal(err)
	}
	if err := s.Apply(ctx, e); err == nil {
		t.Fatal("expected failure")
	}
	var n int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM inbox_events WHERE event_id=$1", e.ID).Scan(&n); err != nil || n != 0 {
		t.Fatal("inbox survived rollback", err)
	}
	if _, err := owner.ExecContext(ctx, "ALTER TABLE search_requests DROP CONSTRAINT reject_test"); err != nil {
		t.Fatal(err)
	}
	if err := s.Apply(ctx, e); err != nil {
		t.Fatal(err)
	}
}
