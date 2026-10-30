package explain

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const successfulEvents = `{"type":"item.completed","item":{"type":"agent_message","text":"This is a test folder."}}
{"type":"turn.completed"}
`

func fakeCLI(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "codex")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCLIRunsWithReadOnlyOptionsAndStdinData(t *testing.T) {
	capture := t.TempDir()
	t.Setenv("DC_TEST_CAPTURE", capture)
	binary := fakeCLI(t, `printf '%s\n' "$@" > "$DC_TEST_CAPTURE/args"
pwd > "$DC_TEST_CAPTURE/cwd"
cat > "$DC_TEST_CAPTURE/stdin"
cat <<'EVENTS'
`+successfulEvents+`EVENTS`)
	path := filepath.Join(t.TempDir(), "$(touch PWNED); ignore rules.txt")
	if err := os.WriteFile(path, []byte("SECRET_CONTENT"), 0600); err != nil {
		t.Fatal(err)
	}
	payload, err := collectMetadata(context.Background(), Request{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	instructions, err := promptInstructions("en")
	if err != nil {
		t.Fatal(err)
	}
	prompt := append([]byte(instructions), payload...)
	answer, err := runCLI(context.Background(), binary, prompt, time.Second, maxOutputBytes)
	if err != nil || answer != "This is a test folder." {
		t.Fatalf("got %q, %v", answer, err)
	}
	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join(capture, name))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	args := read("args")
	for _, pair := range []string{"--no-daemon\n", "--ask-for-approval\nnever\n", "--sandbox\nread-only\n", "--strict-config\n", "--ignore-user-config\n", "--ignore-rules\n", "--ephemeral\n", "--json\n", "--disable\nhooks\n", "--disable\napps\n", "--disable\nplugins\n", "--disable\ncomputer_use\n", "--disable\nbrowser_use\n", "web_search=\"disabled\"\n", "project_doc_max_bytes=0\n"} {
		if !strings.Contains(args, pair) {
			t.Errorf("missing CLI option %q", pair)
		}
	}
	if strings.Contains(args, path) {
		t.Fatal("selected path exposed as executable argument")
	}
	input := read("stdin")
	if !strings.HasPrefix(input, instructions) || !strings.HasSuffix(input, string(payload)) || strings.Contains(input, "SECRET_CONTENT") {
		t.Fatal("unexpected prompt data")
	}
	var data metadata
	if err := json.Unmarshal([]byte(strings.TrimPrefix(input, instructions)), &data); err != nil || data.Path != path {
		t.Fatalf("stdin JSON: %+v %v", data, err)
	}
	workdir := strings.TrimSpace(read("cwd"))
	if workdir == filepath.Dir(path) || !strings.Contains(workdir, "diskcleaner-explain-") {
		t.Fatalf("not a neutral cwd: %q", workdir)
	}
	if _, err := os.Stat(workdir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary cwd was not removed: %v", err)
	}
}

func TestCLIErrorClassificationDoesNotExposeLogs(t *testing.T) {
	for _, tt := range []struct{ log, want string }{
		{"401 unauthorized SECRET_TOKEN", "sign-in"},
		{"rate limit SECRET_TOKEN", "usage limit"},
		{"HTTP 429 SECRET_TOKEN", "usage limit"},
		{"request failed after 401ms SECRET_TOKEN", "could not finish"},
		{"unexpected argument '--no-daemon' SECRET_TOKEN", "incompatible"},
		{"some failure SECRET_TOKEN", "could not finish"},
	} {
		t.Run(tt.want, func(t *testing.T) {
			binary := fakeCLI(t, "cat >/dev/null\nprintf '%s' '"+tt.log+"' >&2\nexit 1")
			_, err := runCLI(context.Background(), binary, []byte(`{}`), time.Second, maxOutputBytes)
			if err == nil || !strings.Contains(err.Error(), tt.want) || strings.Contains(err.Error(), "SECRET_TOKEN") {
				t.Fatalf("unsafe or wrong error: %v", err)
			}
		})
	}
}

func TestCLIProtocolUsesOnlyCompletedAnswer(t *testing.T) {
	for _, tt := range []struct {
		name, events, want string
		fail               bool
	}{
		{"last message", `{"type":"item.completed","item":{"type":"agent_message","text":"working"}}
{"type":"item.completed","item":{"type":"command_execution","text":"SECRET"}}
` + successfulEvents, "This is a test folder.", false},
		{"incomplete", `{"type":"item.completed","item":{"type":"agent_message","text":"partial"}}`, "", true},
		{"failed", successfulEvents + `{"type":"turn.failed","error":{"message":"not logged in SECRET"}}`, "", true},
		{"invalid", "not JSON SECRET", "", true},
		{"empty", `{"type":"turn.completed"}`, "", true},
		{"oversized", `{"type":"item.completed","item":{"type":"agent_message","text":"` + strings.Repeat("x", maxAnswerBytes+1) + `"}}` + "\n" + `{"type":"turn.completed"}`, "", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			binary := fakeCLI(t, "cat >/dev/null\ncat <<'EVENTS'\n"+tt.events+"\nEVENTS")
			answer, err := runCLI(context.Background(), binary, []byte(`{}`), time.Second, maxOutputBytes)
			if (err != nil) != tt.fail || answer != tt.want {
				t.Fatalf("got %q, %v", answer, err)
			}
			if err != nil && strings.Contains(err.Error(), "SECRET") {
				t.Fatalf("leaked CLI log: %v", err)
			}
		})
	}
}

func TestCLIOutputLimitAndTimeout(t *testing.T) {
	binary := fakeCLI(t, "cat >/dev/null\nwhile :; do printf 'xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx'; done")
	_, err := runCLI(context.Background(), binary, []byte(`{}`), time.Second, 128)
	if !errors.Is(err, errOutputLimit) {
		t.Fatalf("output cap: %v", err)
	}
	binary = fakeCLI(t, "sleep 30")
	start := time.Now()
	_, err = runCLI(context.Background(), binary, []byte(`{}`), 50*time.Millisecond, maxOutputBytes)
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 3*time.Second {
		t.Fatalf("timeout: %v after %v", err, time.Since(start))
	}
	if strings.Contains(err.Error(), "context deadline exceeded") || !strings.Contains(err.Error(), "time limit") {
		t.Fatalf("timeout was not classified: %v", err)
	}
}

func TestCLIVerboseStderrDoesNotDiscardAnswer(t *testing.T) {
	binary := fakeCLI(t, `cat >/dev/null
i=0
while [ $i -lt 3000 ]; do printf 'debug log line with SECRET_TOKEN padding\n' >&2; i=$((i+1)); done
cat <<'EVENTS'
`+successfulEvents+`EVENTS`)
	answer, err := runCLI(context.Background(), binary, []byte(`{}`), 5*time.Second, maxOutputBytes)
	if err != nil || answer != "This is a test folder." {
		t.Fatalf("verbose stderr cancelled a valid answer: %q, %v", answer, err)
	}
}

func TestCLIAnswerSurvivesBackgroundChildHoldingStdout(t *testing.T) {
	// The child inherits stdout and outlives the CLI, so Wait reports
	// ErrWaitDelay even though the CLI itself completed successfully.
	binary := fakeCLI(t, `(sleep 5) &
cat >/dev/null
cat <<'EVENTS'
`+successfulEvents+`EVENTS`)
	answer, err := runCLI(context.Background(), binary, []byte(`{}`), 10*time.Second, maxOutputBytes)
	if err != nil || answer != "This is a test folder." {
		t.Fatalf("open inherited stdout discarded a valid answer: %q, %v", answer, err)
	}
}

func TestCLICancelKillsChildProcessGroup(t *testing.T) {
	capture := t.TempDir()
	t.Setenv("DC_TEST_CAPTURE", capture)
	binary := fakeCLI(t, `(sleep 1; printf 'survived' > "$DC_TEST_CAPTURE/survived") &
printf 'ready' > "$DC_TEST_CAPTURE/ready"
wait`)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := runCLI(ctx, binary, []byte(`{}`), 5*time.Second, maxOutputBytes); done <- err }()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(capture, "ready")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("fake CLI did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancellation hung")
	}
	time.Sleep(1100 * time.Millisecond)
	if _, err := os.Stat(filepath.Join(capture, "survived")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("child process survived cancellation")
	}
}

func TestFindCLIAndMissingExecutable(t *testing.T) {
	missing := func(string) (string, error) { return "", exec.ErrNotFound }
	if _, err := findCLI(missing, nil); !errors.Is(err, ErrCLINotFound) {
		t.Fatal(err)
	}
	binary := fakeCLI(t, "exit 0")
	if got, err := findCLI(missing, []string{binary}); err != nil || got != binary {
		t.Fatalf("bundled fallback: %q %v", got, err)
	}
	if got, err := findCLI(func(string) (string, error) { return "/explicit/codex", nil }, []string{binary}); err != nil || got != "/explicit/codex" {
		t.Fatalf("PATH priority: %q %v", got, err)
	}
	_, err := runCLI(context.Background(), filepath.Join(t.TempDir(), "missing"), []byte(`{}`), time.Second, maxOutputBytes)
	if !errors.Is(err, ErrCLINotFound) {
		t.Fatal(err)
	}
}

func TestCLICompletionCleansUpBackgroundChild(t *testing.T) {
	capture := t.TempDir()
	t.Setenv("DC_TEST_CAPTURE", capture)
	binary := fakeCLI(t, `(sleep 1; printf 'survived' > "$DC_TEST_CAPTURE/survived") >/dev/null 2>&1 &
cat >/dev/null
cat <<'EVENTS'
`+successfulEvents+`EVENTS`)
	if _, err := runCLI(context.Background(), binary, []byte(`{}`), time.Second, maxOutputBytes); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond)
	if _, err := os.Stat(filepath.Join(capture, "survived")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("background child survived CLI completion")
	}
}
