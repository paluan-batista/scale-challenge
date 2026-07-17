//go:build acceptance

package acceptance

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"scale-challenge/internal/simulator"
)

func TestFoundationTopologyAndDeterministicScenarioFixture(t *testing.T) {
	composeFile := filepath.Join("..", "..", "docker-compose.yml")
	contents, err := os.ReadFile(composeFile)
	if err != nil {
		t.Fatal(err)
	}

	for _, required := range []string{"api:", "worker:", "postgres:", "redis:", "simulator:", "test:", "profiles: [\"simulator\"]", "condition: service_healthy"} {
		if !strings.Contains(string(contents), required) {
			t.Errorf("compose topology is missing %q", required)
		}
	}

	scenario, err := simulator.Load(filepath.Join("..", "..", "testdata", "scenarios", "happy-path.json"))
	if err != nil {
		t.Fatal(err)
	}
	first, err := simulator.Sequence(scenario, 42)
	if err != nil {
		t.Fatal(err)
	}
	second, err := simulator.Sequence(scenario, 42)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("seed 42 produced different reading sequences")
	}
}

func TestGherkinSuccessFullEnvironment(t *testing.T) {
	compose, available := newComposeRunner(t)
	if !available {
		t.Skip("Docker Compose executable is unavailable in this test environment")
	}
	if output, err := compose.run("version"); err != nil {
		t.Skipf("Docker Compose prerequisite is unavailable: %s", strings.TrimSpace(string(output)))
	}
	t.Cleanup(func() {
		if output, err := compose.run("down", "--volumes", "--remove-orphans"); err != nil {
			t.Logf("compose cleanup failed: %v: %s", err, strings.TrimSpace(string(output)))
		}
	})

	if output, err := compose.run("up", "--build", "-d"); err != nil {
		t.Fatalf("start Compose environment: %v: %s", err, strings.TrimSpace(string(output)))
	}
	for _, service := range []string{"api", "worker", "postgres", "redis"} {
		if err := compose.waitForHealth(service); err != nil {
			t.Fatal(err)
		}
	}
	if response, err := (&http.Client{Timeout: 5 * time.Second}).Get("http://127.0.0.1:" + compose.apiPort + "/health/ready"); err != nil {
		t.Fatalf("call API readiness endpoint: %v", err)
	} else if response.StatusCode != http.StatusOK {
		_ = response.Body.Close()
		t.Fatalf("API readiness status = %d, want %d", response.StatusCode, http.StatusOK)
	} else {
		_ = response.Body.Close()
	}
	if output, err := compose.run("run", "--rm", "seed"); err != nil {
		t.Fatalf("apply deterministic seed: %v: %s", err, strings.TrimSpace(string(output)))
	}

	firstOutput, err := compose.run("--profile", "simulator", "run", "--rm", "simulator")
	if err != nil {
		t.Fatalf("run first simulator sequence: %v: %s", err, strings.TrimSpace(string(firstOutput)))
	}
	secondOutput, err := compose.run("--profile", "simulator", "run", "--rm", "simulator")
	if err != nil {
		t.Fatalf("run second simulator sequence: %v: %s", err, strings.TrimSpace(string(secondOutput)))
	}
	if got, want := jsonLines(firstOutput), jsonLines(secondOutput); len(got) == 0 || !reflect.DeepEqual(got, want) {
		t.Fatal("seed 42 did not produce a repeatable emitted event sequence")
	}

	scenario, err := simulator.Load(filepath.Join("..", "..", "testdata", "scenarios", "happy-path.json"))
	if err != nil {
		t.Fatal(err)
	}
	first, err := simulator.Sequence(scenario, 42)
	if err != nil {
		t.Fatal(err)
	}
	second, err := simulator.Sequence(scenario, 42)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("seed 42 did not produce a repeatable event sequence")
	}
}

func TestGherkinErrorInvalidSimulatorConfigurationFailsWithoutSecret(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "negative seed", args: []string{"--base-url", "http://api:8080", "--scenario", "../../testdata/scenarios/happy-path.json", "--seed", "-1"}, want: "non-negative seed"},
		{name: "zero frequency", args: []string{"--base-url", "http://api:8080", "--scenario", "../../testdata/scenarios/happy-path.json", "--frequency-ms", "0"}, want: "frequency_ms must be greater than zero"},
		{name: "missing base URL", args: []string{"--scenario", "../../testdata/scenarios/happy-path.json"}, want: "base-url, scenario"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			command := exec.Command("go", append([]string{"run", "../../cmd/simulator"}, testCase.args...)...)
			output, err := command.CombinedOutput()
			if err == nil {
				t.Fatal("invalid simulator command succeeded")
			}
			if strings.Contains(string(output), "scale-local-only") {
				t.Fatalf("simulator output leaked a secret: %s", output)
			}
			if !strings.Contains(string(output), testCase.want) {
				t.Fatalf("simulator output did not identify invalid configuration: %s", output)
			}
		})
	}
}

func TestGherkinErrorServiceMissingRequiredVariableFailsWithoutSecret(t *testing.T) {
	for _, service := range []string{"api", "worker"} {
		t.Run(service, func(t *testing.T) {
			command := exec.Command("go", "run", "../../cmd/"+service)
			command.Env = append(withoutEnvironment("DATABASE_URL", "REDIS_ADDR"), "QA_TEST_SECRET=must-not-appear")
			output, err := command.CombinedOutput()
			if err == nil {
				t.Fatal("service started with missing required configuration")
			}
			if strings.Contains(string(output), "must-not-appear") {
				t.Fatalf("service output leaked a secret: %s", output)
			}
			if !strings.Contains(string(output), "DATABASE_URL") {
				t.Fatalf("service output did not identify the missing variable: %s", output)
			}
		})
	}
}

func withoutEnvironment(names ...string) []string {
	blocked := make(map[string]struct{}, len(names))
	for _, name := range names {
		blocked[name] = struct{}{}
	}
	var retained []string
	for _, entry := range os.Environ() {
		name, _, found := strings.Cut(entry, "=")
		if found {
			if _, excluded := blocked[name]; excluded {
				continue
			}
		}
		retained = append(retained, entry)
	}
	return retained
}

type composeRunner struct {
	path    string
	prefix  []string
	env     []string
	apiPort string
}

func newComposeRunner(t testing.TB) (composeRunner, bool) {
	t.Helper()
	path, err := exec.LookPath("docker-compose")
	prefix := []string{}
	if err != nil {
		path, err = exec.LookPath("docker")
		prefix = []string{"compose"}
	}
	if err != nil {
		return composeRunner{}, false
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve local API port: %v", err)
	}
	port := fmt.Sprintf("%d", listener.Addr().(*net.TCPAddr).Port)
	if err := listener.Close(); err != nil {
		t.Fatalf("release local API port: %v", err)
	}
	project := fmt.Sprintf("scale-challenge-acceptance-%d", os.Getpid())
	return composeRunner{
		path:    path,
		prefix:  prefix,
		apiPort: port,
		env: append(os.Environ(),
			"COMPOSE_PROJECT_NAME="+project,
			"API_PORT="+port,
		),
	}, true
}

func (runner composeRunner) run(arguments ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, runner.path, append(runner.prefix, append([]string{"-f", filepath.Join("..", "..", "docker-compose.yml")}, arguments...)...)...)
	command.Env = runner.env
	return command.CombinedOutput()
}

func (runner composeRunner) waitForHealth(service string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	for {
		output, err := runner.run("ps", service)
		if err == nil && strings.Contains(string(output), "healthy") {
			return nil
		}
		timer := time.NewTimer(500 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("Compose service %q did not become healthy: %w; last output: %s", service, ctx.Err(), strings.TrimSpace(string(output)))
		case <-timer.C:
		}
	}
}

func jsonLines(output []byte) []string {
	var events []string
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "{") && strings.HasSuffix(line, "}") {
			events = append(events, line)
		}
	}
	return events
}
