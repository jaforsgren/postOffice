// Package shell runs the bash scripts of BASH requests.
package shell

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	nethttp "net/http"
	"os"
	"os/exec"
	"postOffice/internal/http"
	"postOffice/internal/postman"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	bashBinary  = "bash"
	commandFlag = "-c"

	// HeaderExitCode carries the script's exit code in the response headers.
	HeaderExitCode = "Exit-Code"

	statusCodeSucceeded = nethttp.StatusOK
	statusCodeFailed    = nethttp.StatusInternalServerError

	// waitDelay bounds how long a cancelled script's children may hold its
	// output open before Wait gives up on them.
	waitDelay = 2 * time.Second
)

// Executor runs bash requests. With a Terminal the script is attached to it
// and can prompt; without one its input is empty, so prompts read EOF.
type Executor struct {
	terminal Terminal
}

// NewExecutor returns an executor running scripts on terminal, which may be nil.
func NewExecutor(terminal Terminal) *Executor {
	return &Executor{terminal: terminal}
}

// ExecuteContext runs req's body as a bash script and kills it when ctx is
// cancelled.
//
// The URL is bash://<working directory> ("~/" expands to the home directory;
// empty runs in PostOffice's directory). Each header is exported to the
// script as an environment variable. Variables resolve in the URL, headers
// and body. The response body is everything the script printed (stdout and
// stderr); a zero exit code answers 200, any other 500, and the exit code
// is in the Exit-Code header.
func (e *Executor) ExecuteContext(ctx context.Context, req *postman.Request, variables []postman.VariableSource) *http.Response {
	start := time.Now()
	rawURL := postman.ResolveVariables(req.URL.Raw, variables)
	resp := &http.Response{
		RequestMethod:  postman.MethodBash,
		RequestURL:     rawURL,
		RequestHeaders: resolveHeaders(req.Header, variables),
	}
	if req.Body != nil {
		resp.RequestBody = postman.ResolveVariables(req.Body.Raw, variables)
	}

	workingDir, err := postman.ExpandPath(trimScheme(rawURL))
	if err != nil {
		resp.Error = fmt.Errorf("failed to resolve working directory %q: %w", rawURL, err)
		return resp
	}

	output := &lockedBuffer{}
	cmd := exec.CommandContext(ctx, bashBinary, commandFlag, resp.RequestBody)
	cmd.Dir = workingDir
	cmd.Env = append(os.Environ(), environment(resp.RequestHeaders)...)
	cmd.Stdout = output
	cmd.Stderr = output
	cmd.WaitDelay = waitDelay

	err = e.run(ctx, cmd)
	resp.Duration = time.Since(start)
	resp.Body = output.String()

	var exitErr *exec.ExitError
	switch {
	case ctx.Err() != nil:
		resp.Error = fmt.Errorf("bash script stopped: %w", ctx.Err())
	case errors.As(err, &exitErr):
		resp.StatusCode = statusCodeFailed
		setExitCode(resp, exitErr.ExitCode())
	case err != nil:
		resp.Error = fmt.Errorf("failed to run bash script: %w", err)
	default:
		resp.StatusCode = statusCodeSucceeded
		setExitCode(resp, 0)
	}
	return resp
}

func (e *Executor) run(ctx context.Context, cmd *exec.Cmd) error {
	if e.terminal == nil {
		return cmd.Run()
	}
	return e.terminal.Run(ctx, cmd)
}

func trimScheme(rawURL string) string {
	if strings.HasPrefix(strings.ToLower(rawURL), postman.SchemeBash) {
		return rawURL[len(postman.SchemeBash):]
	}
	return rawURL
}

func resolveHeaders(headers []postman.Header, variables []postman.VariableSource) map[string]string {
	resolved := make(map[string]string, len(headers))
	for _, header := range headers {
		resolved[header.Key] = postman.ResolveVariables(header.Value, variables)
	}
	return resolved
}

func environment(headers map[string]string) []string {
	env := make([]string, 0, len(headers))
	for key, value := range headers {
		env = append(env, key+"="+value)
	}
	return env
}

func setExitCode(resp *http.Response, code int) {
	resp.Status = fmt.Sprintf("Exit %d", code)
	resp.Headers = map[string][]string{HeaderExitCode: {strconv.Itoa(code)}}
}

// lockedBuffer collects stdout and stderr, which a Terminal may tee through
// separate writers and so separate goroutines.
type lockedBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}
