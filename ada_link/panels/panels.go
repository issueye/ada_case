// Package panels keeps a host's embedded panels: which sessions run, what
// state each is in, the namespaced events they emit, and broadcasts from the
// host to them.
//
// A registry is one host process's table of panels, keyed by an ID the host
// chooses:
//
//	reg := panels.New()
//	sess, err := adalink.Spawn(...)
//	panel, err := reg.Add("orders", "Orders", sess)
//	win := mygo.NewWindow(...)
//	if _, err := reg.Attach("orders", win); err != nil { ... }
//	off := reg.OnEvent("changed", func(e panels.Event) { ... })
//	reg.Broadcast("theme", Theme{Dark: true}) // every running panel
//
// Events are namespaced by panel, so that two children can both emit "ready"
// without colliding: a listener of OnEvent("ready") gets
// Event{Topic: "panel/<id>/ready", Panel: "<id>", Name: "ready"}.
// Host-to-panel broadcasts are not namespaced: the child sees the name it
// knows, such as "theme".
//
// A registry is not an event bus. Each event is delivered on its session's
// read goroutine, as link does, so listeners must not block; fan-out to many
// consumers, latest-value caching and queues belong to a separate hub on top
// of the registry (see DESIGN.md).
package panels

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/egoist/mygo"

	"github.com/learningdog1/ada_link/link"
	"github.com/learningdog1/ada_link/mygoembed"
)

// State is where a panel is in its life.
type State uint8

// The states of a panel.
const (
	// Running means the session is up.
	Running State = iota + 1
	// Closed means the session ended without an error: the host closed it, or
	// the child left after a bye.
	Closed
	// Failed means the child exited, crashed, or the connection failed.
	Failed
)

// String returns the state's name.
func (s State) String() string {
	switch s {
	case Running:
		return "running"
	case Closed:
		return "closed"
	case Failed:
		return "failed"
	default:
		return fmt.Sprintf("State(%d)", int(s))
	}
}

// Panel is a snapshot of one panel, as Get, List and changes return it.
// Changing its fields changes nothing in the registry.
type Panel struct {
	// ID is the name the host gave the panel.
	ID string
	// Title is the title the host passed, or the child's when it passed none.
	Title string
	// Session is the link to the child.
	Session *link.Session
	// Host is the view that shows the child's frames, nil until Attach.
	Host *mygoembed.Host
	// State is where the panel is.
	State State
	// Err is why it failed, nil otherwise.
	Err error
}

// ChangeKind is what happened to a panel.
type ChangeKind uint8

// The kinds of change.
const (
	// Added means a panel was added.
	Added ChangeKind = iota + 1
	// Updated means its state, host or title changed, as when a view was
	// attached or the child ended.
	Updated
	// Removed means the panel left the registry.
	Removed
)

// String returns the kind's name.
func (k ChangeKind) String() string {
	switch k {
	case Added:
		return "added"
	case Updated:
		return "updated"
	case Removed:
		return "removed"
	default:
		return fmt.Sprintf("ChangeKind(%d)", int(k))
	}
}

// Change is one panel's change, for OnChange.
type Change struct {
	Kind  ChangeKind
	Panel *Panel
}

// Event is one event a child emitted, namespaced by its panel.
type Event struct {
	// Panel is the ID of the panel that emitted it.
	Panel string
	// Name is the event's name, as the child emitted it.
	Name string
	// Topic is panel/<Panel>/<Name>, the namespace the registry assigns.
	Topic string
	// Payload is the event's JSON payload.
	Payload json.RawMessage
}

// BroadcastResult reports one broadcast: which panels took it, and why the
// others failed.
type BroadcastResult struct {
	// Sent are the IDs of the panels sess.Emit reported success for, in the
	// registry's order.
	Sent []string
	// Errs are the failures, by panel ID.
	Errs map[string]error
}

// Err returns the failures joined, naming their panels, or nil.
func (r BroadcastResult) Err() error {
	if len(r.Errs) == 0 {
		return nil
	}
	ids := make([]string, 0, len(r.Errs))
	errs := make([]error, 0, len(r.Errs))
	for id := range r.Errs {
		ids = append(ids, id)
	}
	// Sort the IDs, so the message reads the same twice.
	for i := 1; i < len(ids); i++ {
		for j := i; j > 0 && ids[j] < ids[j-1]; j-- {
			ids[j], ids[j-1] = ids[j-1], ids[j]
		}
	}
	for _, id := range ids {
		errs = append(errs, fmt.Errorf("%s: %w", id, r.Errs[id]))
	}
	return errors.Join(errs...)
}

// BroadcastOptions choose which panels a broadcast reaches. Its zero value
// reaches every running panel.
type BroadcastOptions struct {
	// Include, when not empty, limits the broadcast to these panel IDs.
	Include []string
	// Exclude leaves these panel IDs out, as the panel that changed the state
	// itself often does.
	Exclude []string
	// Closed includes panels whose state is not Running.
	Closed bool
	// OnError takes each failed Emit, as a logger would. Broadcasts run
	// concurrently, but OnError runs after all of them answered.
	OnError func(panelID string, err error)
}

// Registry is a host's table of panels. Its methods are safe from any
// goroutine.
type Registry struct {
	mu     sync.Mutex
	closed bool
	panels map[string]*entry
	order  []string
	// subs are the listeners of every panel's events, by event name.
	subs    map[string]map[uint64]func(Event)
	nextSub uint64
	changes []changeSub
	nextChg uint64
}

// entry is the registry's own record of a panel.
type entry struct {
	id    string
	title string
	sess  *link.Session
	host  *mygoembed.Host
	state State
	err   error
	// eventOffs holds the listener each subscribed event name has on this
	// panel's session.
	eventOffs map[string]func()
}

// changeSub is one change listener, for off.
type changeSub struct {
	id uint64
	fn func(Change)
}

// New returns an empty registry.
func New() *Registry {
	return &Registry{
		panels: map[string]*entry{},
		subs:   map[string]map[uint64]func(Event){},
	}
}

// Add puts a session in the registry under id. title may be empty, for the
// child's own title. The ID must be unique and free of slashes and spaces,
// because it names the panel's event topic.
func (r *Registry) Add(id, title string, sess *link.Session) (*Panel, error) {
	if sess == nil {
		return nil, errors.New("panels: nil session")
	}
	if err := validateID(id); err != nil {
		return nil, err
	}
	if title == "" {
		title = sess.Surface().Title
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil, errors.New("panels: registry closed")
	}
	if _, ok := r.panels[id]; ok {
		r.mu.Unlock()
		return nil, fmt.Errorf("panels: panel %q is already registered", id)
	}
	e := &entry{id: id, title: title, sess: sess, state: Running, eventOffs: map[string]func(){}}
	r.panels[id] = e
	r.order = append(r.order, id)
	names := make([]string, 0, len(r.subs))
	for name := range r.subs {
		names = append(names, name)
	}
	snapshot := r.snapshot(e)
	r.mu.Unlock()

	r.notify(Change{Kind: Added, Panel: snapshot})
	// Attach outside r.mu: a listener may be called at once with an event
	// the session held, and that call dispatches through the registry.
	for _, name := range names {
		if off := r.listen(e, name); off != nil {
			r.mu.Lock()
			if r.panels[id] == e && r.subs[name] != nil && e.eventOffs[name] == nil {
				e.eventOffs[name] = off
				off = nil
			}
			r.mu.Unlock()
			if off != nil {
				off()
			}
		}
	}
	go r.watch(id, sess)
	return snapshot, nil
}

// Attach puts the panel's frames in win, through mygoembed.Attach. Call it
// on the window's main thread, as attaching a view does.
func (r *Registry) Attach(id string, win *mygo.Window) (*Panel, error) {
	if win == nil {
		return nil, errors.New("panels: nil window")
	}
	r.mu.Lock()
	e := r.panels[id]
	if e == nil {
		r.mu.Unlock()
		return nil, fmt.Errorf("panels: no panel %q", id)
	}
	if e.host != nil {
		r.mu.Unlock()
		return nil, fmt.Errorf("panels: panel %q already has a window", id)
	}
	sess := e.sess
	r.mu.Unlock()

	host := mygoembed.Attach(win, sess)

	r.mu.Lock()
	if r.panels[id] != e {
		// Removed while the window attached.
		r.mu.Unlock()
		host.Close()
		return nil, fmt.Errorf("panels: panel %q was removed", id)
	}
	e.host = host
	snapshot := r.snapshot(e)
	r.mu.Unlock()
	r.notify(Change{Kind: Updated, Panel: snapshot})
	return snapshot, nil
}

// Get returns a snapshot of the panel id.
func (r *Registry) Get(id string) (*Panel, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e := r.panels[id]
	if e == nil {
		return nil, false
	}
	return r.snapshot(e), true
}

// List returns snapshots of the panels, in the order they were added.
func (r *Registry) List() []*Panel {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*Panel, 0, len(r.order))
	for _, id := range r.order {
		if e := r.panels[id]; e != nil {
			out = append(out, r.snapshot(e))
		}
	}
	return out
}

// IDs returns the panels' IDs, in the order they were added.
func (r *Registry) IDs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.order...)
}

// Remove takes the panel out of the registry and closes its session unless
// it already ended. Its listenerson that session go.
func (r *Registry) Remove(id string) error {
	r.mu.Lock()
	e := r.panels[id]
	if e == nil {
		r.mu.Unlock()
		return fmt.Errorf("panels: no panel %q", id)
	}
	r.forget(e)
	offs := make([]func(), 0, len(e.eventOffs))
	for name, off := range e.eventOffs {
		offs = append(offs, off)
		delete(e.eventOffs, name)
	}
	snapshot := r.snapshot(e)
	r.mu.Unlock()
	for _, off := range offs {
		off()
	}
	if !e.sess.IsClosed() {
		e.sess.Close()
	}
	r.notify(Change{Kind: Removed, Panel: snapshot})
	return nil
}

// Close removes every panel and closes their sessions. A registry that was
// closed takes no more panels.
func (r *Registry) Close() {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	r.closed = true
	entries := make([]*entry, 0, len(r.order))
	for _, id := range r.order {
		if e := r.panels[id]; e != nil {
			entries = append(entries, e)
			r.forget(e)
		}
	}
	r.mu.Unlock()
	for _, e := range entries {
		offs := make([]func(), 0, len(e.eventOffs))
		for _, off := range e.eventOffs {
			offs = append(offs, off)
		}
		e.eventOffs = map[string]func(){}
		for _, off := range offs {
			off()
		}
		snapshot := &Panel{ID: e.id, Title: e.title, Session: e.sess, Host: e.host,
			State: e.state, Err: e.err}
		if !e.sess.IsClosed() {
			e.sess.Close()
		}
		r.notify(Change{Kind: Removed, Panel: snapshot})
	}
}

// OnChange calls fn for every panel added, changed or removed, now and
// later, on the goroutine that changed it. fn must not block: a session's
// read goroutine is one of its callers.
func (r *Registry) OnChange(fn func(Change)) (off func()) {
	r.mu.Lock()
	r.nextChg++
	id := r.nextChg
	r.changes = append(r.changes, changeSub{id: id, fn: fn})
	r.mu.Unlock()
	return func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		for i, s := range r.changes {
			if s.id == id {
				r.changes = append(r.changes[:i:i], r.changes[i+1:]...)
				return
			}
		}
	}
}

// OnEvent calls fn for events named name from any panel, now and later, with
// the panel's namespace. It runs on the emitting session's read goroutine,
// so fn must not block: start a goroutine for slow work. A name that cannot
// be a topic, empty or with a slash, panics, as MyGo's NewEvent does.
func (r *Registry) OnEvent(name string, fn func(Event)) (off func()) {
	if err := validateEventName(name); err != nil {
		panic(err)
	}
	r.mu.Lock()
	if r.subs[name] == nil {
		r.subs[name] = map[uint64]func(Event){}
	}
	r.nextSub++
	id := r.nextSub
	r.subs[name][id] = fn
	var entries []*entry
	for _, e := range r.panels {
		if e.eventOffs[name] == nil {
			entries = append(entries, e)
		}
	}
	r.mu.Unlock()

	// Attach outside r.mu, for the same reason Add does.
	for _, e := range entries {
		if off := r.listen(e, name); off != nil {
			r.mu.Lock()
			if r.panels[e.id] == e && r.subs[name][id] != nil && e.eventOffs[name] == nil {
				e.eventOffs[name] = off
				off = nil
			}
			r.mu.Unlock()
			if off != nil {
				off()
			}
		}
	}
	return func() {
		r.mu.Lock()
		var offs []func()
		if subs := r.subs[name]; subs != nil {
			delete(subs, id)
			if len(subs) == 0 {
				delete(r.subs, name)
				for _, e := range r.panels {
					if off := e.eventOffs[name]; off != nil {
						offs = append(offs, off)
						delete(e.eventOffs, name)
					}
				}
			}
		}
		r.mu.Unlock()
		for _, off := range offs {
			off()
		}
	}
}

// OnPanelEvent calls fn for events named name from the panels with these
// IDs, and returns its off. Unlike OnEvent, it does not follow panels the
// host adds later. The event's Topic is the panel's namespace.
func (r *Registry) OnPanelEvent(ids []string, name string, fn func(Event)) (off func(), err error) {
	if err := validateEventName(name); err != nil {
		return nil, err
	}
	r.mu.Lock()
	var sessions []*link.Session
	var panelIDs []string
	for _, id := range ids {
		e := r.panels[id]
		if e == nil {
			r.mu.Unlock()
			return nil, fmt.Errorf("panels: no panel %q", id)
		}
		sessions = append(sessions, e.sess)
		panelIDs = append(panelIDs, id)
	}
	r.mu.Unlock()

	offs := make([]func(), 0, len(sessions))
	for i, sess := range sessions {
		panelID := panelIDs[i]
		offs = append(offs, sess.OnEvent(name, func(p json.RawMessage) {
			fn(Event{Panel: panelID, Name: name, Topic: mustTopic(panelID, name), Payload: p})
		}))
	}
	return func() {
		for _, off := range offs {
			off()
		}
	}, nil
}

// Broadcast sends an event to the panels, running ones by default, and
// reports each panel's result. Sends run concurrently, so one slow child
// does not delay the others. The child sees name as given, not namespaced:
// host-to-panel events are commands of the child's, such as "theme".
func (r *Registry) Broadcast(name string, payload any, opts ...BroadcastOptions) BroadcastResult {
	if err := validateEventName(name); err != nil {
		panic(err)
	}
	var o BroadcastOptions
	if len(opts) > 0 {
		o = opts[0]
	}
	type target struct {
		id   string
		sess *link.Session
	}
	r.mu.Lock()
	var targets []target
	for _, id := range r.order {
		e := r.panels[id]
		if e == nil {
			continue
		}
		if !o.Closed && e.state != Running {
			continue
		}
		if len(o.Include) > 0 && !contains(o.Include, id) {
			continue
		}
		if contains(o.Exclude, id) {
			continue
		}
		targets = append(targets, target{id: id, sess: e.sess})
	}
	r.mu.Unlock()

	type outcome struct {
		id  string
		err error
	}
	outcomes := make([]outcome, len(targets))
	var wg sync.WaitGroup
	for i, t := range targets {
		wg.Add(1)
		go func(i int, t target) {
			defer wg.Done()
			outcomes[i] = outcome{id: t.id, err: t.sess.Emit(name, payload)}
		}(i, t)
	}
	wg.Wait()

	res := BroadcastResult{Errs: map[string]error{}}
	for _, out := range outcomes {
		if out.err != nil {
			res.Errs[out.id] = out.err
			continue
		}
		res.Sent = append(res.Sent, out.id)
	}
	if o.OnError != nil {
		for _, out := range outcomes {
			if out.err != nil {
				o.OnError(out.id, out.err)
			}
		}
	}
	return res
}

// BroadcastTo is Broadcast limited to the panels with these IDs.
func (r *Registry) BroadcastTo(ids []string, name string, payload any) BroadcastResult {
	return r.Broadcast(name, payload, BroadcastOptions{Include: ids})
}

// watch marks a panel's state when its session ends.
func (r *Registry) watch(id string, sess *link.Session) {
	<-sess.Done()
	err := sess.Err()
	state := Closed
	if err != nil && !errors.Is(err, link.ErrClosed) {
		state = Failed
	}
	r.mu.Lock()
	e := r.panels[id]
	if e == nil || e.sess != sess {
		r.mu.Unlock()
		return
	}
	e.state, e.err = state, err
	snapshot := r.snapshot(e)
	r.mu.Unlock()
	r.notify(Change{Kind: Updated, Panel: snapshot})
}

// snapshot copies an entry, called under r.mu.
func (r *Registry) snapshot(e *entry) *Panel {
	return &Panel{ID: e.id, Title: e.title, Session: e.sess, Host: e.host,
		State: e.state, Err: e.err}
}

// forget removes an entry from the map and order, called under r.mu.
func (r *Registry) forget(e *entry) {
	delete(r.panels, e.id)
	for i, id := range r.order {
		if id == e.id {
			r.order = append(r.order[:i:i], r.order[i+1:]...)
			break
		}
	}
}

// listen puts the registry's listener for name on e, and returns its off.
// It must not be called under r.mu: the session may call it at once with an
// event it held, and that call dispatches through the registry.
func (r *Registry) listen(e *entry, name string) func() {
	panelID := e.id
	return e.sess.OnEvent(name, func(p json.RawMessage) {
		r.dispatch(panelID, name, p)
	})
}

// dispatch gives one event to the listeners of its name, outside r.mu.
func (r *Registry) dispatch(panelID, name string, payload json.RawMessage) {
	r.mu.Lock()
	if _, ok := r.panels[panelID]; !ok {
		r.mu.Unlock()
		return
	}
	subs := r.subs[name]
	if len(subs) == 0 {
		r.mu.Unlock()
		return
	}
	fns := make([]func(Event), 0, len(subs))
	for _, fn := range subs {
		fns = append(fns, fn)
	}
	r.mu.Unlock()
	e := Event{Panel: panelID, Name: name, Topic: mustTopic(panelID, name), Payload: payload}
	for _, fn := range fns {
		fn(e)
	}
}

// notify calls the change listeners outside r.mu.
func (r *Registry) notify(c Change) {
	r.mu.Lock()
	fns := make([]func(Change), 0, len(r.changes))
	for _, s := range r.changes {
		if s.fn != nil {
			fns = append(fns, s.fn)
		}
	}
	r.mu.Unlock()
	for _, fn := range fns {
		fn(c)
	}
}

// contains reports whether list holds s.
func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// validateID checks a panel ID: it names the panel's topic, so it must not
// hold a slash or whitespace.
func validateID(id string) error {
	if id == "" {
		return errors.New("panels: empty panel ID")
	}
	if strings.ContainsAny(id, "/ \t\r\n") {
		return fmt.Errorf("panels: panel ID %q holds a slash or whitespace", id)
	}
	return nil
}

// validateEventName checks an event name for a topic.
func validateEventName(name string) error {
	if name == "" {
		return errors.New("panels: empty event name")
	}
	if strings.Contains(name, "/") {
		return fmt.Errorf("panels: event name %q holds a slash", name)
	}
	return nil
}

// Topic returns a panel's event topic: panel/<id>/<name>.
func Topic(panelID, name string) (string, error) {
	if err := validateID(panelID); err != nil {
		return "", err
	}
	if err := validateEventName(name); err != nil {
		return "", err
	}
	return "panel/" + panelID + "/" + name, nil
}

// mustTopic is Topic for an ID and name already validated.
func mustTopic(panelID, name string) string {
	return "panel/" + panelID + "/" + name
}

// SplitTopic takes an event topic apart: panel/<id>/<name>.
func SplitTopic(topic string) (panelID, name string, ok bool) {
	const prefix = "panel/"
	if !strings.HasPrefix(topic, prefix) {
		return "", "", false
	}
	rest := topic[len(prefix):]
	i := strings.IndexByte(rest, '/')
	if i <= 0 || i == len(rest)-1 {
		return "", "", false
	}
	return rest[:i], rest[i+1:], true
}

// HostTopic returns a topic for events the host publishes itself, for the
// hub or UI that later listens to them: host/<name>.
func HostTopic(name string) (string, error) {
	if err := validateEventName(name); err != nil {
		return "", err
	}
	return "host/" + name, nil
}
