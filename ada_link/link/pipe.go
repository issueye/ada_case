package link

import "io"

// PipeConn joins a reader and a writer into the connection a Peer reads
// and writes. It is for two processes that share private pipes — a
// spawned plugin's standard streams — rather than a socket; the peer only
// needs Read, Write and Close.
type PipeConn struct {
	R io.Reader
	W io.Writer
}

// NewPipeConn makes a connection over r and w, in that direction.
func NewPipeConn(r io.Reader, w io.Writer) *PipeConn {
	return &PipeConn{R: r, W: w}
}

// Read reads from the connection's reader.
func (c *PipeConn) Read(p []byte) (int, error) { return c.R.Read(p) }

// Write writes to the connection's writer.
func (c *PipeConn) Write(p []byte) (int, error) { return c.W.Write(p) }

// Close closes the reader and the writer when they are closers; the
// process that opened them may close them anyway.
func (c *PipeConn) Close() error {
	var err error
	if cl, ok := c.R.(io.Closer); ok {
		err = cl.Close()
	}
	if cl, ok := c.W.(io.Closer); ok && err == nil {
		err = cl.Close()
	}
	return err
}
