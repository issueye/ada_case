// Command lightcode-ai is the editor's assistant as its own project. It
// draws its own native UI with MyGo's ui package, renders it offscreen and
// hands the frames to the editor through ada_link; the editor gives it a
// rectangle, forwards input, and does the file work the panel asks for.
//
// It has its own go.mod and builds on its own, so the editor's window never
// links the panel's UI or the network client.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/learningdog1/ada_link/mygoembed"

	"lightcode.ai/client/assistant"
	"lightcode.ai/client/protocol"
)

func main() {
	version := flag.Bool("version", false, "print the panel's name and version, then exit")
	flag.Parse()
	if *version {
		fmt.Println(protocol.Name, protocol.Version)
		return
	}
	log.SetFlags(0)
	log.SetOutput(os.Stderr)

	bridge := &childBridge{}
	state := assistant.New(bridge, func() {})
	bridge.refresh = state.Refresh
	err := mygoembed.Serve(mygoembed.ServeOptions{
		View:    state.View,
		Service: state.Service,
		OnReady: func(child *mygoembed.Child) {
			bridge.child = child
			state.Refresh = child.Refresh
			// The editor reports where the mount sits after every move, so
			// a drag can compute window coordinates.
			child.OnEvent(protocol.EventOrigin, func(raw json.RawMessage) {
				var o protocol.Origin
				if err := json.Unmarshal(raw, &o); err == nil {
					state.Origin(o.X, o.Y)
				}
			})
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}

// childBridge is the panel's way to the editor: the link's calls and events.
type childBridge struct {
	child   *mygoembed.Child
	refresh func()
}

// call runs a host method with a timeout, so a stuck editor cannot hang the
// panel's goroutine for good.
func (b *childBridge) call(ctx context.Context, method string, args ...any) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return b.child.Call(ctx, method, args...)
}

// Context asks the editor for the system prompt and the open file's text.
func (b *childBridge) Context(ctx context.Context) ([]protocol.ChatMessage, error) {
	raw, err := b.call(ctx, protocol.MethodContext, protocol.ContextRequest{})
	if err != nil {
		return nil, err
	}
	var res protocol.Context
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &res); err != nil {
			return nil, err
		}
	}
	return res.Messages, nil
}

// Answer hands an answer's text to the editor, which parses the diffs and
// reads the files.
func (b *childBridge) Answer(ctx context.Context, requestID, text string) (protocol.AnswerResult, error) {
	raw, err := b.call(ctx, protocol.MethodAnswer, protocol.AnswerRequest{RequestID: requestID, Text: text})
	if err != nil {
		return protocol.AnswerResult{}, err
	}
	var res protocol.AnswerResult
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &res); err != nil {
			return protocol.AnswerResult{}, err
		}
	}
	return res, nil
}

// Apply writes one card's change into its file.
func (b *childBridge) Apply(ctx context.Context, requestID string, index int) (string, error) {
	raw, err := b.call(ctx, protocol.MethodApply, protocol.ApplyRequest{RequestID: requestID, Index: index})
	if err != nil {
		return "", err
	}
	var res protocol.ApplyResult
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &res); err != nil {
			return "", err
		}
	}
	return res.Err, nil
}

// SetMode asks the editor for another mount; it does not wait, since the
// click should not stall a frame.
func (b *childBridge) SetMode(mode int) {
	go func() { _, _ = b.call(context.Background(), protocol.MethodSetMode, protocol.ModeRequest{Mode: mode}) }()
}

// Settings opens the editor's endpoint/key dialog.
func (b *childBridge) Settings() {
	go func() { _, _ = b.call(context.Background(), protocol.MethodSettings, struct{}{}) }()
}

// MoveTo asks the editor to move a floating mount; an event, since a drag
// sends one per frame.
func (b *childBridge) MoveTo(target int, x, y float32) {
	_ = b.child.Emit(protocol.EventMove, protocol.MoveRequest{Target: target, X: x, Y: y})
}
