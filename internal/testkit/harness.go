// Package testkit provides isolated, opt-in integration infrastructure for Go
// tests. Unit tests use New(t) without options and never start Docker containers.
package testkit

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const containerStartupTimeout = 90 * time.Second

var dockerHostConfiguration sync.Once

// Clock supplies deterministic time to tests.
type Clock interface {
	Now() time.Time
}

// FixedClock is a deterministic clock suitable for fixtures and polling tests.
type FixedClock struct{ instant time.Time }

// NewFixedClock creates a clock that always returns instant.
func NewFixedClock(instant time.Time) FixedClock { return FixedClock{instant: instant} }

// Now returns the configured instant.
func (c FixedClock) Now() time.Time { return c.instant }

// Fixtures names isolates future fixture rows without defining T02 entities.
type Fixtures struct{ Namespace string }

// Name creates a deterministic fixture label scoped to this harness.
func (f Fixtures) Name(kind string) string {
	return f.Namespace + "-" + strings.ToLower(strings.TrimSpace(kind))
}

// Request sends a request to the in-process API with the harness context.
func (h *Harness) Request(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	if h == nil || h.API == nil {
		return nil, errors.New("API driver is not configured")
	}
	request, err := http.NewRequestWithContext(ctx, method, h.API.URL+path, body)
	if err != nil {
		return nil, err
	}
	return h.API.Client().Do(request)
}

// Harness owns all test resources and terminates them using t.Cleanup.
type Harness struct {
	Context  context.Context
	DB       *pgxpool.Pool
	Redis    redis.UniversalClient
	API      *httptest.Server
	Clock    Clock
	Fixtures Fixtures

	cancel     context.CancelFunc
	containers []testcontainers.Container
}

type config struct {
	postgres   bool
	redis      bool
	apiHandler http.Handler
}

// Option explicitly selects an integration dependency.
type Option func(*config)

// WithPostgres starts an isolated PostgreSQL 16 container for this test.
func WithPostgres() Option { return func(c *config) { c.postgres = true } }

// WithRedis starts an isolated Redis 7 container for this test.
func WithRedis() Option { return func(c *config) { c.redis = true } }

// WithAPI starts an in-process API driver with handler.
func WithAPI(handler http.Handler) Option { return func(c *config) { c.apiHandler = handler } }

// New creates a cancellable deterministic harness. Containers start only when a
// test explicitly passes WithPostgres or WithRedis.
func New(t testing.TB, options ...Option) *Harness {
	t.Helper()
	configuration := config{}
	for _, option := range options {
		option(&configuration)
	}

	ctx, cancel := context.WithTimeout(context.Background(), containerStartupTimeout)
	harness := &Harness{
		Context:  ctx,
		Clock:    NewFixedClock(time.Date(2026, time.July, 17, 0, 0, 0, 0, time.UTC)),
		Fixtures: Fixtures{Namespace: sanitizeName(t.Name())},
		cancel:   cancel,
	}
	t.Cleanup(harness.Close)

	if configuration.postgres {
		harness.startPostgres(t)
	}
	if configuration.redis {
		harness.startRedis(t)
	}
	if configuration.apiHandler != nil {
		harness.API = httptest.NewServer(configuration.apiHandler)
	}
	return harness
}

func (h *Harness) startPostgres(t testing.TB) {
	t.Helper()
	configureDockerHostFromActiveContext()
	container, err := testcontainers.Run(h.Context, "postgres:16-alpine",
		testcontainers.WithEnv(map[string]string{"POSTGRES_DB": "scale", "POSTGRES_USER": "scale", "POSTGRES_PASSWORD": "scale-test"}),
		testcontainers.WithExposedPorts("5432/tcp"),
		testcontainers.WithWaitStrategy(wait.ForListeningPort("5432/tcp").WithStartupTimeout(containerStartupTimeout)),
	)
	if err != nil {
		t.Fatalf("start PostgreSQL container: %v", err)
	}
	h.containers = append(h.containers, container)

	endpoint, err := container.Endpoint(h.Context, "")
	if err != nil {
		t.Fatalf("resolve PostgreSQL endpoint: %v", err)
	}
	pool, err := pgxpool.New(h.Context, "postgres://scale:scale-test@"+endpoint+"/scale?sslmode=disable")
	if err != nil {
		t.Fatalf("open PostgreSQL pool: %v", err)
	}
	h.DB = pool
	if err := Eventually(h.Context, 100*time.Millisecond, func(ctx context.Context) (bool, error) {
		return h.DB.Ping(ctx) == nil, nil
	}); err != nil {
		t.Fatalf("wait for usable PostgreSQL: %v", err)
	}
}

func (h *Harness) startRedis(t testing.TB) {
	t.Helper()
	configureDockerHostFromActiveContext()
	container, err := testcontainers.Run(h.Context, "redis:7-alpine",
		testcontainers.WithExposedPorts("6379/tcp"),
		testcontainers.WithWaitStrategy(wait.ForListeningPort("6379/tcp").WithStartupTimeout(containerStartupTimeout)),
	)
	if err != nil {
		t.Fatalf("start Redis container: %v", err)
	}
	h.containers = append(h.containers, container)

	endpoint, err := container.Endpoint(h.Context, "")
	if err != nil {
		t.Fatalf("resolve Redis endpoint: %v", err)
	}
	client := redis.NewClient(&redis.Options{Addr: endpoint})
	h.Redis = client
	if err := Eventually(h.Context, 100*time.Millisecond, func(ctx context.Context) (bool, error) {
		return client.Ping(ctx).Err() == nil, nil
	}); err != nil {
		t.Fatalf("wait for usable Redis: %v", err)
	}
}

// Eventually polls predicate until it succeeds, returns an error, or ctx ends.
// It does not use arbitrary sleep for asynchronous test completion.
func Eventually(ctx context.Context, interval time.Duration, predicate func(context.Context) (bool, error)) error {
	if interval <= 0 {
		return errors.New("poll interval must be greater than zero")
	}
	for {
		matched, err := predicate(ctx)
		if err != nil {
			return err
		}
		if matched {
			return nil
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// AssertNoBusinessMutation is a T01 guard: no schema or domain mutation exists.
func (h *Harness) AssertNoBusinessMutation(t testing.TB) {
	t.Helper()
	if h.DB == nil {
		return
	}
	var tables int
	err := h.DB.QueryRow(h.Context, "select count(*) from information_schema.tables where table_schema = 'public'").Scan(&tables)
	if err != nil {
		t.Fatalf("count business tables: %v", err)
	}
	if tables != 0 {
		t.Fatalf("business table count = %d, want 0 before T02", tables)
	}
}

// Close releases servers, clients, pools, and containers in reverse order.
func (h *Harness) Close() {
	if h.API != nil {
		h.API.Close()
	}
	if h.Redis != nil {
		_ = h.Redis.Close()
	}
	if h.DB != nil {
		h.DB.Close()
	}
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for index := len(h.containers) - 1; index >= 0; index-- {
		_ = h.containers[index].Terminate(cleanupCtx)
	}
	h.cancel()
}

func sanitizeName(name string) string {
	var builder strings.Builder
	for _, character := range strings.ToLower(name) {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') {
			builder.WriteRune(character)
		}
	}
	if builder.Len() == 0 {
		return "test"
	}
	return builder.String()
}

// configureDockerHostFromActiveContext supports Docker CLI contexts (notably
// Colima on macOS). Testcontainers otherwise defaults to /var/run/docker.sock.
// A configured DOCKER_HOST is always respected.
func configureDockerHostFromActiveContext() {
	dockerHostConfiguration.Do(func() {
		if strings.TrimSpace(os.Getenv("DOCKER_HOST")) != "" {
			return
		}
		output, err := exec.Command("docker", "context", "inspect", "--format", "{{ .Endpoints.docker.Host }}").Output()
		if err != nil {
			return
		}
		if host := strings.TrimSpace(string(output)); host != "" {
			_ = os.Setenv("DOCKER_HOST", host)
			if host != "unix:///var/run/docker.sock" && strings.TrimSpace(os.Getenv("TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE")) == "" {
				// Docker Desktop/Colima expose a host socket to the client but the
				// daemon's containers see its Unix socket at the conventional path.
				_ = os.Setenv("TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE", "/var/run/docker.sock")
			}
		}
	})
}
