package aiplugin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Options configure a Client: which process to spawn, and how long to wait
// for its handshake.
type Options struct {
	// Command is the plugin's executable and arguments, and is required.
	// The host passes itself with a subcommand, as cmd/lightcode does.
	Command []string
	// Dir is the plugin's working directory.
	Dir string
	// Env adds to the host's environment.
	Env []string
	// Timeout bounds the plugin's start, handshake included. Zero means
	// five seconds.
	Timeout time.Duration
	// Stderr receives the plugin's standard error; nil discards it.
	Stderr io.Writer
}

// Status reports what the plugin process is doing, for the capabilities
// view.
type Status struct {
	// Running is whether a plugin process answers now.
	Running bool
	// PID is the process's id, 0 before the first start.
	PID int
	// Name and Version come from the plugin's welcome.
	Name, Version string
	// Err is why the last process ended, "" when it runs or never started.
	Err string
}

// Client is the host's end of the plugin: it spawns the process on first
// use, streams answers through it, and starts another if it dies.
//
// Standard input and output are the plugin's pipes, so the child's own
// output must go to standard error: a line of log on stdout would be read
// as a message.
type Client struct {
	opts Options

	mu      sync.Mutex
	peer    *Peer
	proc    *process
	name    string
	version string
	err     error
	closed  bool
}

// New makes a client. It starts no process: the first Stream does.
func New(opts Options) *Client {
	if opts.Timeout <= 0 {
		opts.Timeout = 5 * time.Second
	}
	if opts.Stderr == nil {
		opts.Stderr = io.Discard
	}
	return &Client{opts: opts}
}

// Status reports the plugin's state.
func (c *Client) Status() Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	st := Status{Name: c.name, Version: c.version}
	if c.err != nil {
		st.Err = c.err.Error()
	}
	if c.peer != nil {
		select {
		case <-c.peer.Done():
		default:
			st.Running = true
			if c.proc != nil && c.proc.cmd.Process != nil {
				st.PID = c.proc.cmd.Process.Pid
			}
		}
	}
	return st
}

// Stream asks the plugin for an answer to msgs and calls onDelta with each
// piece as it arrives; it returns the whole answer. Cancel ctx to stop: the
// call fails with ctx's error, and the plugin stops reading the endpoint.
func (c *Client) Stream(ctx context.Context, cfg Config, msgs []ChatMessage, onDelta func(string)) (string, error) {
	p, err := c.ensure()
	if err != nil {
		return "", err
	}
	id := newID()
	ch := make(chan Delta, 64)
	off := p.OnEvent(EventDelta, func(raw json.RawMessage) {
		var d Delta
		if json.Unmarshal(raw, &d) != nil || d.RequestID != id {
			return
		}
		select {
		case ch <- d:
		case <-p.Done():
		}
	})
	defer off()

	req := StreamRequest{RequestID: id, Config: cfg.Normalize(), Messages: msgs}
	type answer struct {
		text string
		err  error
	}
	done := make(chan answer, 1)
	go func() {
		raw, err := p.Call(context.Background(), MethodStream, req)
		if err != nil {
			done <- answer{err: err}
			return
		}
		var res StreamResult
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &res); err != nil {
				done <- answer{err: fmt.Errorf("aiplugin: cannot decode answer: %w", err)}
				return
			}
		}
		done <- answer{text: res.Text}
	}()
	// A canceled context tells the plugin too, rather than only leaving it
	// reading a reply nobody wants. The watcher must not take the answer:
	// the loop below takes it.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-ctx.Done():
			callCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_, _ = p.Call(callCtx, MethodCancel, CancelRequest{RequestID: id})
		case <-stop:
		case <-p.Done():
		}
	}()

	var full strings.Builder
	take := func(d Delta) {
		full.WriteString(d.Text)
		if onDelta != nil {
			onDelta(d.Text)
		}
	}
	for {
		select {
		case d := <-ch:
			take(d)
		case a := <-done:
			// The plugin writes its deltas before the reply, on one pipe;
			// the ones still buffered here arrived first.
		drain:
			for {
				select {
				case d := <-ch:
					take(d)
				default:
					break drain
				}
			}
			if a.err != nil {
				return full.String(), a.err
			}
			if a.text != "" {
				return a.text, nil
			}
			return full.String(), nil
		case <-ctx.Done():
			return full.String(), ctx.Err()
		case <-p.Done():
			return full.String(), p.Err()
		}
	}
}

// Close stops the plugin: it says bye, closes the pipes and stops the
// process if it did not exit by itself.
func (c *Client) Close() error {
	c.mu.Lock()
	p, proc := c.peer, c.proc
	c.peer, c.proc = nil, nil
	c.closed = true
	c.mu.Unlock()
	c.stop(p, proc)
	return nil
}

// Restart stops the process, as a reload; the next Stream starts a new one.
func (c *Client) Restart() {
	c.mu.Lock()
	p, proc := c.peer, c.proc
	c.peer, c.proc = nil, nil
	c.mu.Unlock()
	c.stop(p, proc)
}

func (c *Client) stop(p *Peer, proc *process) {
	if p != nil {
		_ = p.Send(&Message{T: "bye"})
		p.Close()
	}
	if proc != nil {
		proc.stop(500 * time.Millisecond)
	}
}

// ensure returns a running peer, starting the process when there is none.
func (c *Client) ensure() (*Peer, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, ErrClosed
	}
	if c.peer != nil {
		select {
		case <-c.peer.Done():
			if c.err == nil {
				c.err = c.peer.Err()
			}
			c.peer, c.proc = nil, nil
		default:
			return c.peer, nil
		}
	}
	return c.start()
}

// start spawns the plugin and takes its welcome; the caller holds the mutex.
func (c *Client) start() (*Peer, error) {
	if len(c.opts.Command) == 0 {
		return nil, errors.New("aiplugin: no plugin command")
	}
	cmd := exec.Command(c.opts.Command[0], c.opts.Command[1:]...)
	cmd.Dir = c.opts.Dir
	cmd.Env = append(os.Environ(), c.opts.Env...)
	cmd.Stderr = c.opts.Stderr
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("aiplugin: stdin: %w", err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		in.Close()
		return nil, fmt.Errorf("aiplugin: stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		in.Close()
		out.Close()
		return nil, fmt.Errorf("aiplugin: start %s: %w", c.opts.Command[0], err)
	}
	proc := newProcess(cmd)
	p := NewQuietPeer(&pipeConn{r: out, w: in})
	welcome := make(chan *Message, 1)
	p.OnMessage(func(m *Message) bool {
		if m.T != "welcome" {
			return false
		}
		select {
		case welcome <- m:
		default:
		}
		return true
	})
	p.Start()
	fail := func(err error) (*Peer, error) {
		p.Close()
		proc.stop(500 * time.Millisecond)
		c.err = err
		return nil, err
	}
	if err := p.Send(&Message{T: "hello", Proto: Proto, Name: Name}); err != nil {
		return fail(fmt.Errorf("aiplugin: hello: %w", err))
	}
	select {
	case w := <-welcome:
		if w.Proto != Proto {
			return fail(fmt.Errorf("aiplugin: the plugin speaks protocol %d, not %d", w.Proto, Proto))
		}
		c.name, c.version = w.Name, w.Version
	case <-p.Done():
		return fail(fmt.Errorf("aiplugin: the plugin exited before its welcome: %v", p.Err()))
	case <-time.After(c.opts.Timeout):
		return fail(fmt.Errorf("aiplugin: the plugin did not answer in %v", c.opts.Timeout))
	}
	c.peer, c.proc, c.err = p, proc, nil
	return p, nil
}

// pipeConn joins a child's stdin and stdout into one connection.
type pipeConn struct {
	r io.ReadCloser
	w io.WriteCloser
}

func (c *pipeConn) Read(p []byte) (int, error)  { return c.r.Read(p) }
func (c *pipeConn) Write(p []byte) (int, error) { return c.w.Write(p) }

func (c *pipeConn) Close() error {
	err := c.r.Close()
	if werr := c.w.Close(); err == nil {
		err = werr
	}
	return err
}

// process is the spawned plugin, waited for off the main path.
type process struct {
	cmd  *exec.Cmd
	done chan struct{}
}

func newProcess(cmd *exec.Cmd) *process {
	p := &process{cmd: cmd, done: make(chan struct{})}
	go func() {
		_ = cmd.Wait()
		close(p.done)
	}()
	return p
}

// stop waits grace for the plugin to exit, then kills it.
func (p *process) stop(grace time.Duration) {
	select {
	case <-p.done:
		return
	case <-time.After(grace):
	}
	if p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	select {
	case <-p.done:
	case <-time.After(2 * time.Second):
	}
}

// newID makes a request id: unique enough that a cancel cannot hit another.
func newID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprint(time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
