// Package brew is a typed client over the Homebrew CLI. It shells out to the
// `brew` binary and parses its JSON (and, for doctor, text) output. Shelling
// out — rather than reimplementing Homebrew's resolution logic — keeps the tool
// honest: it always reflects exactly what the user's brew would do.
package brew

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const defaultBin = "brew"

// Flags shared across subcommands. The JSON form is not uniform because brew's
// own surface isn't: info and outdated expose a v2 schema, tap-info doesn't.
const (
	// jsonV2 selects brew's v2 JSON — the {formulae, casks} envelope our parsers
	// expect. v1 returns a flat array and, for info, omits casks entirely.
	jsonV2 = "--json=v2"
	// jsonV1 is the only JSON form tap-info supports; it has no v2 variant.
	jsonV1 = "--json"
	// endOfOptions stops brew reading later args as flags, so a package name like
	// "-rf" or "--macports" is treated as an operand. Load-bearing — see CLAUDE.md.
	endOfOptions = "--"
)

// kindFlag is the brew selector that disambiguates a formula from a cask of the
// same name. Centralized because info, search, and desc all need it.
func kindFlag(kind Kind) string {
	if kind == KindCask {
		return "--cask"
	}
	return "--formula"
}

// Runner executes brew subcommands. It is an interface so tests can supply
// recorded fixtures instead of spawning real processes.
type Runner interface {
	// Output runs brew to completion, returning stdout and stderr separately.
	// They are split because brew interleaves JSON on stdout with progress and
	// API-download notices on stderr; the parser must only see stdout.
	Output(ctx context.Context, args ...string) (stdout, stderr []byte, err error)
	// Stream runs a long command (upgrade, cleanup) and emits combined output
	// line by line, closing the channel after a terminal Done event.
	Stream(ctx context.Context, args ...string) (<-chan Event, error)
}

// Event is one unit of streamed command output. Done marks the final event and
// carries the command's exit error (nil on success).
type Event struct {
	Line string
	Done bool
	Err  error
}

// Client is the typed brew API used by the rest of the app.
type Client struct {
	r Runner
}

// New returns a Client backed by the real `brew` binary on PATH.
func New() *Client { return &Client{r: &execRunner{bin: defaultBin}} }

// NewWithRunner injects a custom Runner; used by tests.
func NewWithRunner(r Runner) *Client { return &Client{r: r} }

// infoResponse is the `brew info --json=v2` envelope shared by every info call.
type infoResponse struct {
	Formulae []Formula `json:"formulae"`
	Casks    []Cask    `json:"casks"`
}

// Installed returns every installed formula and cask in a single brew call.
// `brew info --json=v2 --installed` reads local metadata, so it stays fast even
// with hundreds of packages and avoids N per-package info calls.
func (c *Client) Installed(ctx context.Context) ([]Formula, []Cask, error) {
	stdout, stderr, err := c.r.Output(ctx, "info", jsonV2, "--installed")
	if err != nil {
		return nil, nil, cmdErr("info --installed", stderr, err)
	}
	var resp infoResponse
	if err := parseJSON(stdout, &resp); err != nil {
		return nil, nil, fmt.Errorf("parse installed: %w", err)
	}
	return resp.Formulae, resp.Casks, nil
}

// Info fetches full details for a single package. kind selects the brew flag so
// a formula and a cask sharing a name don't collide.
func (c *Client) Info(ctx context.Context, name string, kind Kind) (*Formula, *Cask, error) {
	// endOfOptions guard before the name, as in Search.
	args := []string{"info", jsonV2, kindFlag(kind), endOfOptions, name}

	stdout, stderr, err := c.r.Output(ctx, args...)
	if err != nil {
		return nil, nil, cmdErr("info "+name, stderr, err)
	}
	var resp infoResponse
	if err := parseJSON(stdout, &resp); err != nil {
		return nil, nil, fmt.Errorf("parse info %s: %w", name, err)
	}
	if kind == KindCask {
		if len(resp.Casks) == 0 {
			return nil, nil, fmt.Errorf("no cask info for %q", name)
		}
		return nil, &resp.Casks[0], nil
	}
	if len(resp.Formulae) == 0 {
		return nil, nil, fmt.Errorf("no formula info for %q", name)
	}
	return &resp.Formulae[0], nil, nil
}

// Taps lists installed taps with their formula/cask counts and origin.
func (c *Client) Taps(ctx context.Context) ([]Tap, error) {
	stdout, stderr, err := c.r.Output(ctx, "tap-info", "--installed", jsonV1)
	if err != nil {
		return nil, cmdErr("tap-info", stderr, err)
	}
	var taps []Tap
	if err := parseJSON(stdout, &taps); err != nil {
		return nil, fmt.Errorf("parse taps: %w", err)
	}
	return taps, nil
}

// Outdated lists packages with a newer version available.
func (c *Client) Outdated(ctx context.Context) (*OutdatedReport, error) {
	stdout, stderr, err := c.r.Output(ctx, "outdated", jsonV2)
	if err != nil {
		return nil, cmdErr("outdated", stderr, err)
	}
	var report OutdatedReport
	if err := parseJSON(stdout, &report); err != nil {
		return nil, fmt.Errorf("parse outdated: %w", err)
	}
	return &report, nil
}

// Search returns package names matching term. evalAll widens the search to
// third-party taps' contents (off by default because it is noticeably slower).
func (c *Client) Search(ctx context.Context, term string, kind Kind, evalAll bool) ([]string, error) {
	args := []string{"search", kindFlag(kind)}
	if evalAll {
		args = append(args, "--eval-all")
	}
	// endOfOptions so a term starting with "-" is treated as text, not a brew
	// flag (e.g. "--macports" must not switch brew to another source).
	args = append(args, endOfOptions, term)

	stdout, stderr, err := c.r.Output(ctx, args...)
	if err != nil {
		return nil, cmdErr("search "+term, stderr, err)
	}
	return parseSearch(stdout), nil
}

// Descriptions returns a name→one-line-description map for the given packages,
// read from brew's local description cache (no install, no network). Tapped
// packages are keyed by their short name (brew desc drops the tap prefix), so
// callers should look up by short name. Unknown names are simply absent.
func (c *Client) Descriptions(ctx context.Context, names []string, kind Kind) (map[string]string, error) {
	if len(names) == 0 {
		return map[string]string{}, nil
	}
	args := []string{"desc", kindFlag(kind), endOfOptions}
	args = append(args, names...)

	stdout, stderr, err := c.r.Output(ctx, args...)
	if err != nil {
		return nil, cmdErr("desc", stderr, err)
	}
	return parseDescriptions(stdout, kind), nil
}

// Doctor runs `brew doctor` and parses its free-form warnings. A non-zero exit
// WITH output means warnings exist — expected, not a failure — so that exit code
// is ignored. A genuine failure to run brew (an error and no output at all) is
// surfaced, so the UI never reports a healthy system for a brew that never ran.
func (c *Client) Doctor(ctx context.Context) (*DoctorReport, error) {
	stdout, stderr, err := c.r.Output(ctx, "doctor")
	// brew doctor writes warnings to stderr and the all-clear to stdout.
	combined := strings.TrimSpace(string(stdout) + "\n" + string(stderr))
	if combined == "" && err != nil {
		return nil, cmdErr("doctor", stderr, err)
	}
	return parseDoctor(combined), nil
}

// Upgrade upgrades the named packages, or everything outdated when names is
// empty, streaming brew's progress output.
func (c *Client) Upgrade(ctx context.Context, names ...string) (<-chan Event, error) {
	// "--" guards against a name being parsed as an option; with no names it is
	// a no-op and brew still upgrades everything outdated.
	return c.r.Stream(ctx, append([]string{"upgrade", endOfOptions}, names...)...)
}

// Cleanup removes stale downloads and old versions, streaming progress.
func (c *Client) Cleanup(ctx context.Context) (<-chan Event, error) {
	return c.r.Stream(ctx, "cleanup")
}

// Autoremove uninstalls dependencies no longer required by any package.
func (c *Client) Autoremove(ctx context.Context) (<-chan Event, error) {
	return c.r.Stream(ctx, "autoremove")
}

// parseJSON unmarshals brew JSON, tolerating a stray leading notice line by
// trimming to the first JSON delimiter only when a direct parse fails.
func parseJSON(b []byte, v any) error {
	err := json.Unmarshal(bytes.TrimSpace(b), v)
	if err == nil {
		return nil
	}
	// Retry from the first JSON delimiter only when something precedes it — a
	// stray notice line brew occasionally prints before the payload.
	if i := bytes.IndexAny(b, "{["); i > 0 {
		return json.Unmarshal(b[i:], v)
	}
	return err
}

// parseSearch turns `brew search` line output into names, dropping the
// "==> Formulae" section headers and "Warning:" hints brew mixes in.
func parseSearch(b []byte) []string {
	var names []string
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "==>") || strings.HasPrefix(line, "Warning:") {
			continue
		}
		names = append(names, line)
	}
	return names
}

// parseDescriptions turns `brew desc` output ("name: description" per line)
// into a map. Cask lines are "token: (Human Name) description"; that redundant
// parenthetical name is stripped — but ONLY for casks, since a formula's
// description can legitimately begin with a parenthetical (e.g. "(GNU) ...").
func parseDescriptions(b []byte, kind Kind) map[string]string {
	out := make(map[string]string)
	for _, line := range strings.Split(string(b), "\n") {
		name, desc, ok := strings.Cut(line, ": ")
		if !ok {
			continue
		}
		name, desc = strings.TrimSpace(name), strings.TrimSpace(desc)
		if kind == KindCask && strings.HasPrefix(desc, "(") {
			if i := strings.Index(desc, ") "); i >= 0 {
				desc = strings.TrimSpace(desc[i+2:])
			}
		}
		if name != "" && desc != "" {
			out[name] = desc
		}
	}
	return out
}

// cmdErr wraps an exec failure with the brew subcommand and its stderr, which
// is where brew puts the actual reason (unknown formula, network error, etc.).
func cmdErr(what string, stderr []byte, err error) error {
	msg := strings.TrimSpace(string(stderr))
	if msg == "" {
		return fmt.Errorf("brew %s: %w", what, err)
	}
	return fmt.Errorf("brew %s: %w: %s", what, err, msg)
}

// brewEnv runs brew non-interactively and deterministically: no auto-update on
// every read (slow, surprising), no color codes (would corrupt text parsing),
// and no env hints cluttering output.
func brewEnv() []string {
	return append(os.Environ(),
		"HOMEBREW_NO_AUTO_UPDATE=1",
		"HOMEBREW_NO_COLOR=1",
		"HOMEBREW_NO_ENV_HINTS=1",
	)
}

// execRunner is the production Runner backed by os/exec.
type execRunner struct {
	bin string
}

func (r *execRunner) Output(ctx context.Context, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, r.bin, args...)
	cmd.Env = brewEnv()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

// Scanner sizing for streamed output: brew emits very long lines (progress
// bars, full paths), so start modest and allow growth to avoid ErrTooLong.
const (
	scanBufInit = 64 * 1024
	scanBufMax  = 1024 * 1024
)

// Stream wires the child's stdout and stderr to a single OS pipe so progress
// lines arrive in order. The parent closes its write end after Start so the
// reader sees EOF once the child exits; CommandContext kills the child if ctx
// is cancelled (e.g. the user quits mid-upgrade).
func (r *execRunner) Stream(ctx context.Context, args ...string) (<-chan Event, error) {
	cmd := exec.CommandContext(ctx, r.bin, args...)
	cmd.Env = brewEnv()

	pr, pw, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		pr.Close()
		pw.Close()
		return nil, err
	}
	pw.Close() // child holds its own dup; parent must release to get EOF

	ch := make(chan Event)
	go func() {
		defer close(ch)
		scanner := bufio.NewScanner(pr)
		scanner.Buffer(make([]byte, 0, scanBufInit), scanBufMax)
		for scanner.Scan() {
			select {
			case ch <- Event{Line: scanner.Text()}:
			case <-ctx.Done():
			}
		}
		scanErr := scanner.Err() // e.g. a line exceeding the buffer (ErrTooLong)
		pr.Close()
		// Wait reaps the child and releases fds; guard the terminal send so a
		// consumer that stopped reading (e.g. on quit) can't strand this goroutine.
		werr := cmd.Wait()
		if scanErr != nil {
			// A read failure is the real cause; surface it clearly instead of the
			// opaque broken-pipe error the killed child would otherwise report.
			werr = fmt.Errorf("reading brew output: %w", scanErr)
		}
		select {
		case ch <- Event{Done: true, Err: werr}:
		case <-ctx.Done():
		}
	}()
	return ch, nil
}
