// Package adalink is the library's front door: it puts another MyGo
// application's native UI in your window, in a process of its own, and
// carries typed calls and events between the two.
//
// A host calls Spawn to start a child and Attach to place its frames and
// forward its input:
//
//	sess, err := adalink.Spawn(adalink.SpawnOptions{
//		Command: []string{childExe},
//		W: 720, H: 520, Scale: 2,
//	})
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer sess.Close()
//
//	var panel *adalink.Host
//	win := mygo.NewWindow(mygo.WindowOptions{
//		Content: ui.View(func(c *ui.Context) {
//			panel.View(c)
//		}),
//	})
//	panel = adalink.Attach(win, sess)
//
// A child calls Serve with the ui.View it would give a window:
//
//	err := adalink.Serve(adalink.ServeOptions{
//		View:    app.view,
//		Service: app.call,
//	})
//
// The packages below are the same API, by concern:
//
//   - github.com/learningdog1/ada_link/link: the transport, protocol,
//     frame ring, input and calls, without MyGo types.
//   - github.com/learningdog1/ada_link/mygoembed: the MyGo and ui
//     integration, and the types this package re-exports.
//   - github.com/learningdog1/ada_link/install: per-user install and
//     uninstall for Windows.
//
// The design is in DESIGN.md and the wire protocol in docs/protocol.md.
package adalink

import (
	"github.com/learningdog1/ada_link/link"
	"github.com/learningdog1/ada_link/mygoembed"

	"github.com/egoist/mygo"
)

// Version is the library's version, also what an installed app shows.
const Version = "0.1.0"

// The library's types, re-exported so that an app can import one package.
type (
	// Session is a host's link to a child process.
	Session = link.Session
	// SpawnOptions configure Spawn.
	SpawnOptions = link.SpawnOptions
	// Surface is the size and scale a child draws at.
	Surface = link.Surface
	// Frame is one of a child's frames, as the host receives it.
	Frame = link.Frame
	// FrameInfo names a child's frame ring.
	FrameInfo = link.FrameInfo
	// SizeInfo is a size in DIPs and the scale to draw at.
	SizeInfo = link.SizeInfo
	// InputEvent is one pointer, key, text or focus event for the child.
	InputEvent = link.InputEvent
	// InputKind names what an InputEvent reports.
	InputKind = link.InputKind
	// Handler serves one method for a peer.
	Handler = link.Handler
	// Peer is the message layer under a session.
	Peer = link.Peer
	// Stats counts a session's frames.
	Stats = link.Stats
	// Host places a child's frames in a window.
	Host = mygoembed.Host
	// ServeOptions configure Serve.
	ServeOptions = mygoembed.ServeOptions
	// Child is a running child, and what its app can do with the link.
	Child = mygoembed.Child
)

// The kinds of input, re-exported from link.
const (
	InputDown    = link.InputDown
	InputUp      = link.InputUp
	InputMove    = link.InputMove
	InputScroll  = link.InputScroll
	InputZoom    = link.InputZoom
	InputKeyDown = link.InputKeyDown
	InputKeyUp   = link.InputKeyUp
	InputText    = link.InputText
	InputCompose = link.InputCompose
	InputCommand = link.InputCommand
	InputFocus   = link.InputFocus
	InputDrop    = link.InputDrop
)

// Spawn starts a child process and links to it: link.Spawn.
func Spawn(opts SpawnOptions) (*Session, error) {
	return link.Spawn(opts)
}

// Attach connects a native-UI window to a session: the child's frames in
// the window's view, and the window's input to the child.
func Attach(win *mygo.Window, sess *Session) *Host {
	return mygoembed.Attach(win, sess)
}

// Serve runs the child app until the host closes the link.
func Serve(opts ServeOptions) error {
	return mygoembed.Serve(opts)
}
