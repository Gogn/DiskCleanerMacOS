package explain

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	maxOutputBytes = 1024 * 1024
	maxAnswerBytes = 64 * 1024
)

var errOutputLimit = errors.New("Codex output exceeded the explanation limit; try a smaller item")

func cliArgs() []string {
	args := []string{"--no-daemon", "--ask-for-approval", "never", "--sandbox", "read-only", "--strict-config"}
	// These feature names are advertised by the installed CLI's features list.
	// This reduces integration exposure; it is not a global no-tools guarantee.
	for _, feature := range []string{"apps", "plugins", "remote_plugin", "hooks", "browser_use", "browser_use_external", "computer_use", "multi_agent", "image_generation", "memories", "shell_snapshot"} {
		args = append(args, "--disable", feature)
	}
	for _, config := range []string{`web_search="disabled"`, "project_doc_max_bytes=0", `shell_environment_policy.inherit="core"`} {
		args = append(args, "-c", config)
	}
	return append(args, "exec", "--ignore-user-config", "--ignore-rules", "--skip-git-repo-check", "--ephemeral", "--json", "--color", "never", "-")
}

func runCLI(ctx context.Context, binary string, prompt []byte, timeout time.Duration, outputLimit int) (string, error) {
	ctx, stopTimeout := context.WithTimeout(ctx, timeout)
	defer stopTimeout()
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	if err := ctx.Err(); err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp("", "diskcleaner-explain-")
	if err != nil {
		return "", errors.New("cannot create a temporary workspace for Codex")
	}
	defer os.RemoveAll(dir)
	cmd := exec.CommandContext(ctx, binary, cliArgs()...)
	cmd.Dir = dir
	cmd.Stdin = bytes.NewReader(prompt)
	cmd.WaitDelay = 2 * time.Second
	if err := configureProcess(cmd); err != nil {
		return "", err
	}
	stdout := &limitedBuffer{limit: outputLimit, cancel: cancel}
	stderr := &limitedBuffer{limit: 64 * 1024, cancel: cancel}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	err = cmd.Run()
	// A CLI may exit before a background child. Do not leave its process group
	// running merely because the group leader has already completed.
	cleanupProcess(cmd)
	if cause := context.Cause(ctx); cause != nil {
		return "", cause
	}
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", ErrCLINotFound
		}
		return "", cliFailure(stderr.String() + "\n" + errorMessages(stdout.Bytes()))
	}
	answer, parseErr := parseAnswer(stdout.Bytes())
	if parseErr != nil {
		return "", parseErr
	}
	return answer, nil
}

// limitedBuffer drains without retaining unbounded output and cancels the
// process as soon as its stream exceeds the limit. Raw CLI logs are never shown.
type limitedBuffer struct {
	mu     sync.Mutex
	buf    bytes.Buffer
	limit  int
	cancel context.CancelCauseFunc
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	remaining := b.limit - b.buf.Len()
	if n > remaining {
		b.buf.Write(p[:remaining])
		b.cancel(errOutputLimit)
	} else {
		b.buf.Write(p)
	}
	return n, nil
}

func (b *limitedBuffer) Bytes() []byte  { return b.buf.Bytes() }
func (b *limitedBuffer) String() string { return b.buf.String() }

type cliEvent struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Error   struct {
		Message string `json:"message"`
	} `json:"error"`
	Item struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"item"`
}

func parseAnswer(output []byte) (string, error) {
	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Buffer(make([]byte, 4096), maxOutputBytes)
	answer := ""
	completed := false
	for scanner.Scan() {
		if len(bytes.TrimSpace(scanner.Bytes())) == 0 {
			continue
		}
		var event cliEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return "", errors.New("Codex returned unreadable output; update Codex and retry")
		}
		switch event.Type {
		case "item.completed":
			if event.Item.Type == "agent_message" {
				answer = strings.TrimSpace(event.Item.Text)
			}
		case "turn.completed":
			completed = true
		case "turn.failed":
			return "", cliFailure(event.Error.Message)
		}
	}
	if scanner.Err() != nil {
		return "", errors.New("Codex returned unreadable output; update Codex and retry")
	}
	if !completed || answer == "" {
		return "", cliFailure(errorMessages(output))
	}
	if len(answer) > maxAnswerBytes {
		return "", errOutputLimit
	}
	return answer, nil
}

func errorMessages(output []byte) string {
	var messages []string
	for line := range bytes.SplitSeq(output, []byte{'\n'}) {
		var event cliEvent
		if json.Unmarshal(line, &event) == nil && (event.Type == "error" || event.Type == "turn.failed") {
			messages = append(messages, event.Message, event.Error.Message)
		}
	}
	return strings.Join(messages, "\n")
}

func cliFailure(log string) error {
	log = strings.ToLower(log)
	switch {
	case strings.Contains(log, "unauthorized"), strings.Contains(log, "401"), strings.Contains(log, "not logged in"), strings.Contains(log, "sign in"), strings.Contains(log, "authentication"), strings.Contains(log, "refresh token"):
		return errors.New("Codex sign-in is required or expired; sign in to Codex and retry")
	case strings.Contains(log, "quota"), strings.Contains(log, "rate limit"), strings.Contains(log, "usage limit"), strings.Contains(log, "429"):
		return errors.New("Codex usage limit reached; check your account or try again later")
	case strings.Contains(log, "unexpected argument"), strings.Contains(log, "unknown field"), strings.Contains(log, "unknown feature"):
		return errors.New("this Codex CLI version is incompatible; update Codex and retry")
	default:
		return errors.New("Codex could not finish the explanation; check its sign-in, connection and availability, then retry")
	}
}
