package link

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The environment a host gives its child, which mygoembed.Serve reads.
const (
	EnvSocket = "ADALINK_SOCK"
	EnvToken  = "ADALINK_TOKEN"
	EnvDir    = "ADALINK_DIR"
	EnvProto  = "ADALINK_PROTO"
	EnvW      = "ADALINK_W"
	EnvH      = "ADALINK_H"
	EnvScale  = "ADALINK_SCALE"
	EnvTitle  = "ADALINK_TITLE"
)

// SpawnOptions configure Spawn.
type SpawnOptions struct {
	// Command is the child's executable and arguments, and is required.
	Command []string
	// Dir is the child's working directory.
	Dir string
	// Env adds to the host's environment, before the link's own.
	Env []string
	// W, H and Scale are the size the child draws at until the host
	// resizes it: DIPs, and device pixels per DIP.
	W, H  int
	Scale float32
	// Title is the child's title, for logs and the host's chrome.
	Title string
	// Stderr receives the child's standard error; nil discards it.
	Stderr io.Writer
	// Stdout receives the child's standard output; nil discards it.
	Stdout io.Writer
	// Timeout bounds the child's start, handshake included. Zero means 20
	// seconds.
	Timeout time.Duration
}

// Spawn starts a child process and links to it. The host listens on a
// socket in a directory of its own, and gives the child its path and a
// random token; the child dials, sends its hello with the token, and the
// host answers with its welcome.
//
// A child that fails to start, or to say hello, is killed and its directory
// removed.
func Spawn(opts SpawnOptions) (*Session, error) {
	if len(opts.Command) == 0 {
		return nil, fmt.Errorf("link: Spawn needs a Command")
	}
	if opts.W <= 0 || opts.H <= 0 {
		return nil, fmt.Errorf("link: Spawn needs a size")
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	dir, err := os.MkdirTemp("", "adalink-")
	if err != nil {
		return nil, fmt.Errorf("link: session directory: %w", err)
	}
	ok := false
	defer func() {
		if !ok {
			_ = removeDir(dir)
		}
	}()
	sock := filepath.Join(dir, "l.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		return nil, fmt.Errorf("link: listen on %s: %w", sock, err)
	}
	// The socket is the user's: no other user of the machine may hand a
	// child frames, or calls, over it.
	_ = os.Chmod(sock, 0o600)
	defer ln.Close()

	token, err := randomToken()
	if err != nil {
		return nil, err
	}
	scale := opts.Scale
	if scale <= 0 {
		scale = 1
	}
	env := []string{
		EnvSocket + "=" + sock,
		EnvToken + "=" + token,
		EnvDir + "=" + dir,
		EnvProto + "=" + fmt.Sprint(Proto),
		EnvW + "=" + fmt.Sprint(opts.W),
		EnvH + "=" + fmt.Sprint(opts.H),
		EnvScale + "=" + fmt.Sprint(scale),
		EnvTitle + "=" + opts.Title,
	}
	cmd := exec.Command(opts.Command[0], opts.Command[1:]...)
	cmd.Dir = opts.Dir
	cmd.Env = append(append(os.Environ(), opts.Env...), env...)
	cmd.Stderr = opts.Stderr
	cmd.Stdout = opts.Stdout
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("link: start the child: %w", err)
	}
	proc := newChildProcess(cmd, opts.Title)

	if err := ln.(*net.UnixListener).SetDeadline(time.Now().Add(timeout)); err != nil {
		proc.stop(2 * time.Second)
		return nil, err
	}
	type accepted struct {
		conn net.Conn
		err  error
	}
	ach := make(chan accepted, 1)
	go func() {
		conn, err := ln.Accept()
		ach <- accepted{conn, err}
	}()
	var conn net.Conn
	select {
	case a := <-ach:
		if a.err != nil {
			proc.stop(2 * time.Second)
			return nil, fmt.Errorf("link: the child did not connect in %v: %w", timeout, a.err)
		}
		conn = a.conn
	case <-proc.waited:
		ln.Close()
		<-ach
		return nil, fmt.Errorf("link: the child exited before it connected: %v", proc.errOf())
	case <-time.After(timeout):
		ln.Close()
		<-ach
		proc.stop(2 * time.Second)
		return nil, fmt.Errorf("link: the child did not connect in %v", timeout)
	}
	_ = conn.SetDeadline(time.Now().Add(timeout))
	hello, err := readHandshake(conn)
	if err == nil {
		err = checkHello(hello, token)
	}
	if err != nil {
		conn.Close()
		proc.stop(2 * time.Second)
		return nil, err
	}
	_ = conn.SetDeadline(time.Time{})
	ring, err := OpenRing(dir, *hello.Frame)
	if err != nil {
		conn.Close()
		proc.stop(2 * time.Second)
		return nil, err
	}
	scale2 := ring.Scale
	if scale2 <= 0 {
		scale2 = 1
	}
	s := &Session{
		dir:  dir,
		cmd:  proc,
		ring: ring,
		pool: newFramePool(ring.W, ring.H, ring.Slots),
		done: make(chan struct{}),
		surface: Surface{
			W:     int(float32(ring.W)/scale2 + 0.5),
			H:     int(float32(ring.H)/scale2 + 0.5),
			Scale: scale2,
			Title: hello.Title,
		},
	}
	s.startPeer(NewPeer(conn))
	if err := s.peer.Send(&Message{T: "welcome", Proto: Proto, Size: &SizeInfo{W: opts.W, H: opts.H, Scale: scale}, Title: hello.Title}); err != nil {
		s.shutdown(err)
		return nil, err
	}
	ok = true
	return s, nil
}

// DialOptions are what a child needs to reach its host: what DialFromEnv
// reads.
type DialOptions struct {
	Socket, Token, Dir string
	W, H               int
	Scale              float32
	Title              string
}

// DialFromEnv reads the link's environment in a child process and dials
// the host. It is what Spawn sets up; a child started by other means can
// pass DialOptions to Dial instead.
func DialFromEnv() (*Child, error) {
	opts := DialOptions{
		Socket: os.Getenv(EnvSocket),
		Token:  os.Getenv(EnvToken),
		Dir:    os.Getenv(EnvDir),
		Title:  os.Getenv(EnvTitle),
	}
	opts.W = atoi(os.Getenv(EnvW))
	opts.H = atoi(os.Getenv(EnvH))
	opts.Scale = atof(os.Getenv(EnvScale), 1)
	if opts.Socket == "" || opts.Token == "" || opts.Dir == "" {
		return nil, fmt.Errorf("link: not started by link.Spawn (%s, %s, %s are not set)", EnvSocket, EnvToken, EnvDir)
	}
	return Dial(opts)
}

// Child is a child's end of the link before it has a peer: the connection
// to the host, and what the host asked for.
type Child struct {
	// Dir is the host's session directory, where the child writes rings.
	Dir string
	// W, H and Scale are the size the host asked for, in DIPs.
	W, H  int
	Scale float32
	// Title is the title the host knows the child by.
	Title string

	conn    net.Conn
	token   string
	peer    *Peer
	helloed bool
}

// Dial connects to the host at opts.Socket.
func Dial(opts DialOptions) (*Child, error) {
	if opts.Socket == "" || opts.Token == "" {
		return nil, fmt.Errorf("link: Dial needs a socket and a token")
	}
	conn, err := net.DialTimeout("unix", opts.Socket, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("link: dial %s: %w", opts.Socket, err)
	}
	scale := opts.Scale
	if scale <= 0 {
		scale = 1
	}
	return &Child{Dir: opts.Dir, W: opts.W, H: opts.H, Scale: scale, Title: opts.Title, conn: conn, token: opts.Token}, nil
}

// Hello sends the child's hello: its frame ring and the surface it drew.
// The host answers with Welcome.
func (c *Child) Hello(info FrameInfo, size SizeInfo, title string) error {
	if c.helloed {
		return fmt.Errorf("link: hello already sent")
	}
	c.helloed = true
	if err := writeHandshake(c.conn, &Message{T: "hello", Proto: Proto, Token: c.token,
		Frame: &info, Size: &size, Title: title}); err != nil {
		c.conn.Close()
		return err
	}
	return nil
}

// Welcome reads the host's answer to Hello, and starts the child's peer.
// After it returns, Peer serves calls and events.
func (c *Child) Welcome() (Surface, error) {
	_ = c.conn.SetDeadline(time.Now().Add(20 * time.Second))
	m, err := readHandshake(c.conn)
	if err != nil {
		c.conn.Close()
		return Surface{}, err
	}
	_ = c.conn.SetDeadline(time.Time{})
	if m.T != "welcome" {
		c.conn.Close()
		return Surface{}, fmt.Errorf("link: the host answered %q, not welcome", m.T)
	}
	if m.Proto != Proto {
		c.conn.Close()
		return Surface{}, fmt.Errorf("link: the host speaks protocol %d, not %d", m.Proto, Proto)
	}
	s := Surface{W: c.W, H: c.H, Scale: c.Scale, Title: c.Title}
	if m.Size != nil {
		s.W, s.H, s.Scale = m.Size.W, m.Size.H, m.Size.Scale
	}
	c.peer = newPeer(c.conn)
	return s, nil
}

// Start starts the child's message loop, after setup registered the
// handlers the host's first messages go to: its resize and frame
// acknowledgments, its calls and its events. Without Start nothing reads.
func (c *Child) Start(setup func(*Peer)) {
	if setup != nil {
		setup(c.peer)
	}
	c.peer.start()
}

// Peer returns the child's message layer, after Welcome.
func (c *Child) Peer() *Peer { return c.peer }

// Close ends the child's connection.
func (c *Child) Close() {
	if c.peer != nil {
		c.peer.Close()
	} else if c.conn != nil {
		c.conn.Close()
	}
}

// readHandshake reads one message before a peer exists.
func readHandshake(conn net.Conn) (*Message, error) {
	payload, err := readMessage(conn)
	if err != nil {
		return nil, fmt.Errorf("link: handshake: %w", err)
	}
	var m Message
	if err := json.Unmarshal(payload, &m); err != nil {
		return nil, fmt.Errorf("link: handshake: %w", err)
	}
	return &m, nil
}

// writeHandshake writes one message before a peer exists.
func writeHandshake(conn net.Conn, m *Message) error {
	payload, err := json.Marshal(m)
	if err != nil {
		return err
	}
	buf := make([]byte, 4+len(payload))
	buf[0] = byte(len(payload))
	buf[1] = byte(len(payload) >> 8)
	buf[2] = byte(len(payload) >> 16)
	buf[3] = byte(len(payload) >> 24)
	copy(buf[4:], payload)
	_, err = conn.Write(buf)
	return err
}

func randomToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("link: token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func removeDir(dir string) error {
	if dir == "" {
		return nil
	}
	return os.RemoveAll(dir)
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func atof(s string, def float32) float32 {
	f, err := strconv.ParseFloat(s, 32)
	if err != nil || f <= 0 {
		return def
	}
	return float32(f)
}

// childProcess is the child the host started, and waits for.
type childProcess struct {
	cmd    *exec.Cmd
	waited chan struct{}
	mu     sync.Mutex
	err    error
	name   string
}

func newChildProcess(cmd *exec.Cmd, name string) *childProcess {
	p := &childProcess{cmd: cmd, waited: make(chan struct{}), name: name}
	go func() {
		err := cmd.Wait()
		p.mu.Lock()
		p.err = err
		p.mu.Unlock()
		close(p.waited)
	}()
	return p
}

// errOf returns why the child exited, once waited.
func (p *childProcess) errOf() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.err
}

// wait waits up to timeout for the child to exit; timeout 0 only checks.
func (p *childProcess) wait(timeout time.Duration) error {
	if timeout <= 0 {
		select {
		case <-p.waited:
			p.mu.Lock()
			defer p.mu.Unlock()
			return p.err
		default:
			return fmt.Errorf("link: the child still runs")
		}
	}
	select {
	case <-p.waited:
		p.mu.Lock()
		defer p.mu.Unlock()
		return p.err
	case <-time.After(timeout):
		return fmt.Errorf("link: the child still runs after %v", timeout)
	}
}

// stop waits grace for the child to exit, then kills it.
func (p *childProcess) stop(grace time.Duration) {
	if err := p.wait(grace); err == nil {
		return
	}
	if p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	select {
	case <-p.waited:
	case <-time.After(2 * time.Second):
		log.Printf("link: the child %q did not die", strings.TrimSpace(p.name))
	}
}
