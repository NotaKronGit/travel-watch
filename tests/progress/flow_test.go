//go:build integration && kafka

// Package progress_test exercises real service binaries over HTTP, Kafka and PostgreSQL.
package progress_test

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	contract "github.com/NotaKronGit/travel-watch/api/progress"
	"github.com/NotaKronGit/travel-watch/api/transport"
	cabinetv1 "github.com/NotaKronGit/travel-watch/gen/travelwatch/cabinet/v1"
	"github.com/NotaKronGit/travel-watch/gen/travelwatch/cabinet/v1/cabinetv1connect"
	eventsv1 "github.com/NotaKronGit/travel-watch/gen/travelwatch/events/v1"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
	"github.com/segmentio/kafka-go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// The only substitute is Collector's transport source. No network or real API keys.
func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == "transport-stdio" {
		if fakeTransport() != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}
func fakeTransport() error {
	requests := make(chan transport.Request, 1)
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		scan := bufio.NewScanner(os.Stdin)
		for scan.Scan() {
			var q transport.Request
			if json.Unmarshal(scan.Bytes(), &q) != nil {
				return
			}
			requests <- q
		}
	}()
	out := json.NewEncoder(os.Stdout)
	for {
		select {
		case <-closed:
			return nil
		case q := <-requests:
			if q.Method == "settlement" {
				if err := os.WriteFile(os.Getenv("TW_TEST_MARKER"), []byte("entered"), 0600); err != nil {
					return err
				}
				ticker := time.NewTicker(20 * time.Millisecond)
				for {
					b, _ := os.ReadFile(os.Getenv("TW_TEST_GATE"))
					if string(b) == "release" {
						break
					}
					select {
					case <-closed:
						ticker.Stop()
						return nil
					case <-ticker.C:
					}
				}
				ticker.Stop()
			}
			r := transport.Response{Version: 1}
			switch q.Method {
			case "settlement":
				r.Stations = []transport.Station{{Code: "fixture-city", Title: "Synthetic origin"}}
				r.Count = 1
				r.Total = 1
			case "search":
				if q.From == "ORG" && q.To == "DST" && q.Mode == "plane" {
					r.Connections = []transport.Connection{{From: transport.Station{Code: "fixture-origin-airport", Title: "Synthetic origin airport", IATA: "ORG"}, To: transport.Station{Code: "fixture-destination-airport", Title: "Synthetic destination airport", IATA: "DST"}, Mode: "plane", Number: "FIXTURE-1"}}
					r.Count = 1
					r.Total = 1
				}
			}
			if err := out.Encode(r); err != nil {
				return err
			}
		}
	}
}

type process struct {
	cmd     *exec.Cmd
	stopped bool
	log     string
}

func (p *process) stop() {
	if !p.stopped {
		p.stopped = true
		_ = p.cmd.Process.Kill()
		_ = p.cmd.Wait()
	}
}
func waitFor(t *testing.T, ctx context.Context, label string, fn func() bool) {
	t.Helper()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if fn() {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("timed out: " + label)
		case <-ticker.C:
		}
	}
}
func TestProgressFlow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	env, err := godotenv.Read(filepath.Join(root, ".env"))
	if err != nil {
		t.Fatal("local integration .env required")
	}
	schema := "progress_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	host := env["CABINET_DATABASE_HOST"]
	if host == "" {
		host = "127.0.0.1"
	}
	port := env["CABINET_DATABASE_PORT"]
	if port == "" {
		port = "55432"
	}
	databases := map[string]*sql.DB{}
	for _, service := range []string{"cabinet", "search"} {
		prefix := strings.ToUpper(service)
		name := env[prefix+"_DATABASE_NAME"]
		if name == "" {
			name = service
		}
		password := env[prefix+"_DATABASE_OWNER_PASSWORD"]
		if password == "" {
			t.Fatal("integration owner password required")
		}
		u := url.URL{Scheme: "postgres", Host: net.JoinHostPort(host, port), Path: name, User: url.UserPassword(service+"_owner", password)}
		u.RawQuery = "sslmode=disable&connect_timeout=5"
		db, err := sql.Open("pgx", u.String())
		if err != nil {
			t.Fatal("open integration database")
		}
		if _, err = db.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
			t.Fatal("create isolated schema", err)
		}
		if _, err = db.ExecContext(ctx, "GRANT USAGE ON SCHEMA "+schema+" TO "+service+"_app"); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			clean, c := context.WithTimeout(context.Background(), 10*time.Second)
			defer c()
			_, e := db.ExecContext(clean, "DROP SCHEMA "+schema+" CASCADE")
			if e != nil {
				t.Error(e)
			}
			_ = db.Close()
		})
		values := u.Query()
		values.Set("search_path", schema)
		u.RawQuery = values.Encode()
		isolated, err := sql.Open("pgx", u.String())
		if err != nil {
			t.Fatal(err)
		}
		databases[service] = isolated
		t.Cleanup(func() { _ = isolated.Close() })
	}
	requestTopic := "test-progress-requests-" + uuid.NewString()
	progressTopic := "test-progress-events-" + uuid.NewString()
	conn, err := kafka.DialContext(ctx, "tcp", "127.0.0.1:19092")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err = conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatal(err)
	}
	for _, topic := range []string{requestTopic, progressTopic} {
		if err = conn.CreateTopics(kafka.TopicConfig{Topic: topic, NumPartitions: 1, ReplicationFactor: 1}); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		clean, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		client := &kafka.Client{Addr: kafka.TCP("127.0.0.1:19092")}
		r, e := client.DeleteTopics(clean, &kafka.DeleteTopicsRequest{Topics: []string{requestTopic, progressTopic}})
		if e != nil {
			t.Error(e)
		} else {
			for _, e := range r.Errors {
				if e != nil {
					t.Error(e)
				}
			}
		}
	})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	_ = listener.Close()
	baseURL := "http://" + address
	childEnv := []string{}
	for _, v := range os.Environ() {
		key, _, _ := strings.Cut(v, "=")
		if strings.HasPrefix(key, "CABINET_") || strings.HasPrefix(key, "SEARCH_") || strings.HasPrefix(key, "COLLECTOR_") || strings.HasPrefix(key, "PG") {
			continue
		}
		childEnv = append(childEnv, v)
	}
	add := func(k, v string) { childEnv = append(childEnv, k+"="+v) }
	add("PGOPTIONS", "-c search_path="+schema)
	for _, service := range []string{"cabinet", "search"} {
		prefix := strings.ToUpper(service)
		add(prefix+"_CONFIG", filepath.Join(root, "services", service, "config.yaml"))
		add(prefix+"_DATABASE_HOST", host)
		add(prefix+"_DATABASE_PORT", port)
		name := env[prefix+"_DATABASE_NAME"]
		if name == "" {
			name = service
		}
		add(prefix+"_DATABASE_NAME", name)
		add(prefix+"_DATABASE_APP_PASSWORD", env[prefix+"_DATABASE_APP_PASSWORD"])
		add(prefix+"_DATABASE_OWNER_PASSWORD", env[prefix+"_DATABASE_OWNER_PASSWORD"])
		add(prefix+"_PROGRESS_BROKERS", "127.0.0.1:19092")
		add(prefix+"_PROGRESS_TOPIC", progressTopic)
		add(prefix+"_PROGRESS_GROUP_ID", "test-"+schema)
		add(prefix+"_PROGRESS_TIMEOUT", "1s")
		add(prefix+"_PROGRESS_POLL_INTERVAL", "1s")
		add(prefix+"_PROGRESS_LEASE", "1m")
	}
	add("CABINET_SERVER_ADDRESS", address)
	add("CABINET_SERVER_ORIGIN", baseURL)
	add("CABINET_AUTH_COOKIE_SECURE", "false")
	add("CABINET_OUTBOX_BROKERS", "127.0.0.1:19092")
	add("CABINET_OUTBOX_TOPIC", requestTopic)
	add("CABINET_OUTBOX_POLL_INTERVAL", "100ms")
	add("SEARCH_PROGRESS_SOURCES", "graph,gemini")
	add("SEARCH_CONSUMER_BROKERS", "127.0.0.1:19092")
	add("SEARCH_CONSUMER_TOPIC", requestTopic)
	add("SEARCH_CONSUMER_GROUP_ID", "test-"+schema)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	add("SEARCH_PLANNER_COLLECTOR_BINARY", executable)
	add("SEARCH_PLANNER_TIMEOUT", "20s")
	gate := filepath.Join(dir, "gate")
	marker := filepath.Join(dir, "marker")
	add("TW_TEST_GATE", gate)
	add("TW_TEST_MARKER", marker)
	// Explicitly blank external credentials, even if root .env contains them.
	add("COLLECTOR_YANDEX_API_KEY", "")
	add("SEARCH_GEMINI_API_KEY", "")
	binaries := map[string]string{}
	for _, service := range []string{"cabinet", "search"} {
		path := filepath.Join(dir, service)
		build := exec.CommandContext(ctx, "go", "build", "-race", "-o", path, "./services/"+service+"/cmd/"+service)
		build.Dir = root
		if output, e := build.CombinedOutput(); e != nil {
			t.Fatalf("build failed: %s", output)
		}
		binaries[service] = path
	}
	start := func(t *testing.T, service, command string) *process {
		t.Helper()
		logPath := filepath.Join(dir, service+"-"+command+"-"+uuid.NewString()+".log")
		log, e := os.Create(logPath)
		if e != nil {
			t.Fatal(e)
		}
		cmd := exec.CommandContext(ctx, binaries[service], command) // #nosec G204 -- binaries are built by this test in its temporary directory; commands are test constants.
		cmd.Dir = root
		cmd.Env = childEnv
		cmd.Stdout = log
		cmd.Stderr = log
		if e = cmd.Start(); e != nil {
			t.Fatal(e)
		}
		p := &process{cmd: cmd, log: logPath}
		t.Cleanup(func() {
			p.stop()
			_ = log.Close()
			if t.Failed() {
				b, _ := os.ReadFile(logPath)
				if len(b) > 4000 {
					b = b[len(b)-4000:]
				}
				t.Logf("%s %s: %s", service, command, b)
			}
		})
		return p
	}
	for _, service := range []string{"cabinet", "search"} {
		p := start(t, service, "migrate")
		err = p.cmd.Wait()
		p.stopped = true
		if err != nil {
			t.Fatal("migration process failed")
		}
	}
	cabinetDB, searchDB := databases["cabinet"], databases["search"]
	origin, destination := uuid.NewString(), uuid.NewString()
	if _, err = cabinetDB.ExecContext(ctx, `INSERT INTO catalog_countries(code,name) VALUES('ZZ','Synthetic country')`); err != nil {
		t.Fatal(err)
	}
	for i, id := range []string{origin, destination} {
		if _, err = cabinetDB.ExecContext(ctx, `INSERT INTO catalog_cities(id,source,source_id,name,name_ru,country_code,region_code,timezone,aliases,latitude,longitude,population) VALUES($1,'e2e-fixture',$2,'Synthetic city','Synthetic city','ZZ','','UTC','[]',0,$3,1)`, id, i+1, i*20); err != nil {
			t.Fatal(err)
		}
	}
	for i, code := range []string{"ORG", "DST"} {
		if _, err = searchDB.ExecContext(ctx, `INSERT INTO catalog_airports(source_id,ident,name,type,latitude,longitude,country,region,municipality,iata,icao,scheduled,imported_at) VALUES($1,$2,$2,'large_airport',0,$3,'ZZ','','',$2,'',true,now())`, i+1, code, i*20); err != nil {
			t.Fatal(err)
		}
	}
	start(t, "cabinet", "serve")
	start(t, "cabinet", "publish-outbox")
	start(t, "search", "consume")
	start(t, "search", "publish-progress")
	start(t, "cabinet", "consume-progress")
	jar, _ := cookiejar.New(nil)
	httpClient := &http.Client{Jar: jar, Timeout: 3 * time.Second}
	waitFor(t, ctx, "HTTP ready", func() bool {
		res, e := httpClient.Get(baseURL + "/healthz")
		if e != nil {
			return false
		}
		_ = res.Body.Close()
		return res.StatusCode == 200
	})
	auth := cabinetv1connect.NewAuthServiceClient(httpClient, baseURL)
	trips := cabinetv1connect.NewTripServiceClient(httpClient, baseURL)
	headers := func(h http.Header) { h.Set("Origin", baseURL); h.Set("X-Travel-Watch-CSRF", "1") }
	register := connect.NewRequest(&cabinetv1.RegisterRequest{Email: "progress-" + schema + "@example.com", Password: "synthetic e2e password only"})
	headers(register.Header())
	if _, err = auth.Register(ctx, register); err != nil {
		t.Fatal(err)
	}
	get := func(id string) *cabinetv1.TripDetails {
		r := connect.NewRequest(&cabinetv1.GetTripRequest{Id: id})
		headers(r.Header())
		out, e := trips.GetTrip(ctx, r)
		if e != nil {
			return nil
		}
		return out.Msg.Trip
	}
	create := func(t *testing.T) string {
		t.Helper()
		if err = os.WriteFile(gate, []byte("hold"), 0600); err != nil {
			t.Fatal(err)
		}
		_ = os.Remove(marker)
		date := time.Now().AddDate(0, 0, 7).Format(time.DateOnly)
		r := connect.NewRequest(&cabinetv1.CreateTripRequest{RequestId: uuid.NewString(), OriginId: origin, DestinationId: destination, DepartureFrom: date, DepartureTo: date, Adults: 1})
		headers(r.Header())
		out, e := trips.CreateTrip(ctx, r)
		if e != nil {
			t.Fatal(e)
		}
		id := out.Msg.Id
		waitFor(t, ctx, "queued through Kafka", func() bool { v := get(id); return v != nil && v.BuildingStage == "queued" && len(v.History) == 1 })
		return id
	}
	release := func(t *testing.T) {
		t.Helper()
		if e := os.WriteFile(gate, []byte("release"), 0600); e != nil {
			t.Fatal(e)
		}
	}
	waitBuilding := func(t *testing.T, id string) {
		t.Helper()
		waitFor(t, ctx, "building and provider entered", func() bool {
			v := get(id)
			_, e := os.Stat(marker)
			return v != nil && v.BuildingStage == "building" && e == nil
		})
	}
	assertUnique := func(t *testing.T, v *cabinetv1.TripDetails) {
		t.Helper()
		seen := map[int64]bool{}
		for _, e := range v.History {
			if seen[e.Revision] {
				t.Fatal("duplicate history revision")
			}
			seen[e.Revision] = true
		}
	}
	var worker *process
	t.Run("success", func(t *testing.T) {
		id := create(t)
		worker = start(t, "search", "build-routes")
		waitBuilding(t, id)
		release(t)
		waitFor(t, ctx, "completed through Kafka", func() bool { v := get(id); return v != nil && v.BuildingStage == "awaiting_schedules" })
		v := get(id)
		if v.Status != cabinetv1.TripStatus_TRIP_STATUS_RUNNING || len(v.History) != 7 {
			t.Fatal("bad lifecycle/history", v)
		}
		last := v.History[len(v.History)-1]
		if last.RouteCount != 1 || last.DurationMs <= 0 || last.StartedAt == nil || last.FinishedAt == nil || !last.Incomplete {
			t.Fatal("missing measured result", last)
		}
		assertUnique(t, v)
		worker.stop()
		// Replay the exact final event through Kafka; projection and history stay unchanged.
		var payload []byte
		if e := searchDB.QueryRowContext(ctx, `SELECT payload FROM progress_outbox WHERE request_id=$1 ORDER BY revision DESC LIMIT 1`, id).Scan(&payload); e != nil {
			t.Fatal(e)
		}
		end := replay(t, ctx, progressTopic, id, payload)
		client := &kafka.Client{Addr: kafka.TCP("127.0.0.1:19092")}
		waitFor(t, ctx, "duplicate committed by Cabinet", func() bool {
			op, cancel := context.WithTimeout(ctx, time.Second)
			defer cancel()
			r, err := client.OffsetFetch(op, &kafka.OffsetFetchRequest{GroupID: "test-" + schema, Topics: map[string][]int{progressTopic: {0}}})
			if err != nil || r.Error != nil {
				return false
			}
			for _, p := range r.Topics[progressTopic] {
				if p.Error == nil && p.CommittedOffset >= end {
					return true
				}
			}
			return false
		})
		if len(get(id).History) != 7 {
			t.Fatal("replay duplicated history")
		}
		assertUnique(t, get(id))
	})
	t.Run("cancel", func(t *testing.T) {
		id := create(t)
		worker = start(t, "search", "build-routes")
		waitBuilding(t, id)
		r := connect.NewRequest(&cabinetv1.CancelTripRequest{Id: id})
		headers(r.Header())
		if _, e := trips.CancelTrip(ctx, r); e != nil {
			t.Fatal(e)
		}
		waitFor(t, ctx, "cancellation reaches worker", func() bool {
			var s string
			_ = searchDB.QueryRowContext(ctx, `SELECT stage FROM route_building WHERE request_id=$1`, id).Scan(&s)
			return s == "cancelled"
		})
		release(t)
		now := timestamppb.Now()
		late := &eventsv1.TripRouteBuildingUpdated{EventId: uuid.NewString(), SchemaVersion: 1, RequestId: id, Revision: 99, Stage: 3, Attempt: 1, OccurredAt: now, StartedAt: now, FinishedAt: now, RouteCount: 1, Incomplete: true, PlannerId: "graph"}
		b, _ := proto.Marshal(late)
		replay(t, ctx, progressTopic, id, b)
		waitFor(t, ctx, "late result observed", func() bool {
			v := get(id)
			if v == nil {
				return false
			}
			for _, h := range v.History {
				if h.Revision == 99 {
					return true
				}
			}
			return false
		})
		v := get(id)
		if v.Status != cabinetv1.TripStatus_TRIP_STATUS_CANCELLED {
			t.Fatal("late result revived trip")
		}
		assertUnique(t, v)
		var stored bool
		if e := searchDB.QueryRowContext(ctx, `SELECT result IS NOT NULL FROM route_building WHERE request_id=$1`, id).Scan(&stored); e != nil || stored {
			t.Fatal("cancelled worker committed result", e)
		}
		worker.stop()
	})
	t.Run("process_restart", func(t *testing.T) {
		id := create(t)
		worker = start(t, "search", "build-routes")
		waitBuilding(t, id)
		worker.stop()
		_ = os.Remove(marker)
		var firstToken string
		var lease time.Time
		if e := searchDB.QueryRowContext(ctx, `SELECT lease_token,lease_until FROM route_building WHERE request_id=$1`, id).Scan(&firstToken, &lease); e != nil {
			t.Fatal(e)
		}
		worker = start(t, "search", "build-routes")
		t.Log("Waiting for the real one-minute lease; no database timestamp manipulation")
		waitFor(t, ctx, "lease expires and new process takes over", func() bool {
			var token string
			var attempt int
			e := searchDB.QueryRowContext(ctx, `SELECT lease_token,attempt FROM route_building WHERE request_id=$1`, id).Scan(&token, &attempt)
			return e == nil && token != firstToken && attempt == 2
		})
		if time.Now().Before(lease) {
			t.Fatal("reclaimed before lease expiry")
		}
		waitBuilding(t, id)
		release(t)
		waitFor(t, ctx, "recovered completion", func() bool { v := get(id); return v != nil && v.BuildingStage == "awaiting_schedules" })
		v := get(id)
		if len(v.History) != 9 || v.History[8].Attempt != 2 || v.History[8].RouteCount != 1 {
			t.Fatal("bad restart history", v.History)
		}
		assertUnique(t, v)
		worker.stop()
	})
}
func replay(t *testing.T, ctx context.Context, topic, id string, b []byte) int64 {
	t.Helper()
	w := &kafka.Writer{Addr: kafka.TCP("127.0.0.1:19092"), Topic: topic, RequiredAcks: kafka.RequireAll, MaxAttempts: 1, WriteTimeout: 5 * time.Second, ReadTimeout: 5 * time.Second, BatchSize: 1}
	defer w.Close()
	if err := w.WriteMessages(ctx, kafka.Message{Key: []byte(id), Value: b, Headers: []kafka.Header{{Key: "event_type", Value: []byte(contract.EventType)}}}); err != nil {
		t.Fatal(err)
	}
	conn, err := kafka.DialLeader(ctx, "tcp", "127.0.0.1:19092", topic, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err = conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	end, err := conn.ReadLastOffset()
	if err != nil {
		t.Fatal(err)
	}
	return end

}
