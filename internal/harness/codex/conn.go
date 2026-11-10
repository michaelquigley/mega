package codex

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/michaelquigley/df/dd"
	"github.com/michaelquigley/df/dl"
)

// errClosed fails a call whose app-server exited before answering.
var errClosed = errors.New("the codex app-server exited")

// replyError is an explicit JSON-RPC error reply: the server answered, and
// refused. it is distinct from a call that got no answer at all.
type replyError struct {
	message string
}

func (e *replyError) Error() string {
	return e.message
}

// handler receives what the app-server sends besides responses. calls arrive
// in order on the connection's reader goroutine.
type handler interface {
	notify(method string, params map[string]any)
	request(id any, method string, params map[string]any)
}

// conn is one `codex app-server` child speaking newline-delimited JSON-RPC on
// stdio. it runs in its own process group, so a terminal interrupt aimed at the
// service does not reach it, and so the processes it starts can be found after
// it exits.
type conn struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser

	mu     sync.Mutex
	nextID int64
	calls  map[int64]chan callResult
	closed bool

	done chan struct{} // closed once the process has exited and been waited
	err  error         // the exit, set before done closes

	userAgent string // from initialize; carries the app-server's version
}

type callResult struct {
	result any
	err    error
}

func dial(binary, cwd string, h handler) (*conn, error) {
	cmd := exec.Command(binary, "app-server")
	cmd.Dir = cwd
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting codex app-server: %w", err)
	}
	c := &conn{cmd: cmd, stdin: stdin, calls: make(map[int64]chan callResult), done: make(chan struct{})}
	go logStderr(cmd.Process.Pid, stderr)
	go c.read(stdout, h)
	return c, nil
}

// initialize performs the handshake every connection needs.
func (c *conn) initialize(ctx context.Context) error {
	params := &initializeParams{
		ClientInfo:   clientInfo{Name: "roundhouse", Title: "Roundhouse", Version: "0"},
		Capabilities: clientCapabilities{ExperimentalAPI: true},
	}
	result := &initializeResult{}
	if err := c.call(ctx, "initialize", params, result); err != nil {
		return err
	}
	c.userAgent = result.UserAgent
	return c.send(&rpcNotification{JSONRPC: "2.0", Method: "initialized", Params: map[string]any{}})
}

// call sends a request and binds its result into out, which may be nil.
func (c *conn) call(ctx context.Context, method string, params any, out any) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return errClosed
	}
	c.nextID++
	id := c.nextID
	reply := make(chan callResult, 1)
	c.calls[id] = reply
	c.mu.Unlock()
	if err := c.send(&rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params}); err != nil {
		c.forget(id)
		return err
	}
	select {
	case r := <-reply:
		if r.err != nil {
			return fmt.Errorf("%s: %w", method, r.err)
		}
		if out == nil {
			return nil
		}
		data, ok := r.result.(map[string]any)
		if !ok {
			return fmt.Errorf("%s: unexpected result", method)
		}
		return dd.Bind(out, data)
	case <-ctx.Done():
		c.forget(id)
		return fmt.Errorf("%s: %w", method, ctx.Err())
	}
}

func (c *conn) forget(id int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.calls, id)
}

// respond answers a server request.
func (c *conn) respond(id any, result any) error {
	return c.send(&rpcResponse{JSONRPC: "2.0", ID: id, Result: result})
}

func (c *conn) send(frame any) error {
	line, err := dd.UnbindJSONL(frame)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errClosed
	}
	if _, err := c.stdin.Write(line); err != nil {
		return fmt.Errorf("writing to codex app-server: %w", err)
	}
	return nil
}

// closeInput ends the app-server's input; it exits when it sees the end.
func (c *conn) closeInput() error {
	return c.stdin.Close()
}

func (c *conn) read(stdout io.Reader, h handler) {
	reader := bufio.NewReader(stdout)
	for {
		line, err := reader.ReadBytes('\n')
		if len(strings.TrimSpace(string(line))) > 0 {
			c.dispatch(line, h)
		}
		if err != nil {
			break
		}
	}
	waitErr := c.cmd.Wait()
	c.mu.Lock()
	c.closed = true
	for id, reply := range c.calls {
		reply <- callResult{err: errClosed}
		delete(c.calls, id)
	}
	c.err = waitErr
	c.mu.Unlock()
	close(c.done)
}

func (c *conn) dispatch(line []byte, h handler) {
	frame := &rpcFrame{}
	if err := dd.BindJSON(frame, line); err != nil {
		dl.Warnf("unreadable frame from codex app-server %d: %v", c.cmd.Process.Pid, err)
		return
	}
	switch {
	case frame.Method != "" && frame.ID != nil:
		h.request(frame.ID, frame.Method, frame.Params)
	case frame.Method != "":
		h.notify(frame.Method, frame.Params)
	case frame.ID != nil:
		id, ok := frameID(frame.ID)
		if !ok {
			return
		}
		c.mu.Lock()
		reply, ok := c.calls[id]
		delete(c.calls, id)
		c.mu.Unlock()
		if !ok {
			return
		}
		if frame.Error != nil {
			reply <- callResult{err: &replyError{message: frame.Error.Message}}
			return
		}
		reply <- callResult{result: frame.Result}
	}
}

func frameID(id any) (int64, bool) {
	switch v := id.(type) {
	case float64:
		return int64(v), true
	case int64:
		return v, true
	case int:
		return int64(v), true
	case string:
		n, err := strconv.ParseInt(v, 10, 64)
		return n, err == nil
	}
	return 0, false
}

// proc identifies a process across pid reuse by its start time.
type proc struct {
	pid   int
	start string
}

// descendants lists the processes the app-server started, at any depth. the
// sandbox codex runs commands in starts its own session, so after the
// app-server exits nothing ties it back; the tree has to be read while the
// app-server is alive.
func (c *conn) descendants() []proc {
	if c.cmd == nil || c.cmd.Process == nil {
		return nil
	}
	stats := readStats()
	children := make(map[int][]int)
	for pid, st := range stats {
		children[st.ppid] = append(children[st.ppid], pid)
	}
	var found []proc
	queue := []int{c.cmd.Process.Pid}
	for len(queue) > 0 {
		next := queue[0]
		queue = queue[1:]
		for _, child := range children[next] {
			found = append(found, proc{pid: child, start: stats[child].start})
			queue = append(queue, child)
		}
	}
	return found
}

// survivors waits up to grace for the given processes to go and returns the
// pids of those still alive.
func survivors(procs []proc, grace time.Duration) []int {
	deadline := time.Now().Add(grace)
	for {
		stats := readStats()
		var alive []int
		for _, p := range procs {
			if st, ok := stats[p.pid]; ok && st.start == p.start {
				alive = append(alive, p.pid)
			}
		}
		if len(alive) == 0 || time.Now().After(deadline) {
			return alive
		}
		time.Sleep(250 * time.Millisecond)
	}
}

type stat struct {
	ppid  int
	start string
}

// readStats reads the parent and start time of every live process.
func readStats() map[int]stat {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	stats := make(map[int]stat)
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "stat"))
		if err != nil {
			continue
		}
		// the command sits in parentheses and may hold spaces; after it come
		// state, ppid, and at index 19 the start time.
		closing := strings.LastIndexByte(string(data), ')')
		if closing < 0 {
			continue
		}
		fields := strings.Fields(string(data[closing+1:]))
		if len(fields) < 20 || fields[0] == "Z" {
			continue
		}
		ppid, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}
		stats[pid] = stat{ppid: ppid, start: fields[19]}
	}
	return stats
}

func logStderr(pid int, stderr io.Reader) {
	scanner := bufio.NewScanner(stderr)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		if line := strings.TrimSpace(scanner.Text()); line != "" {
			dl.Debugf("codex app-server %d: %s", pid, line)
		}
	}
}
