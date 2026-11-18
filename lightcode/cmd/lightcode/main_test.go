package main

import (
	"os"
	"testing"
	"time"

	"lightcode/internal/aiplugin"
)

// TestAIProviderSubcommand runs the real dispatch: argv says
// "ai-provider", so the process becomes the plugin and answers a hello
// instead of opening a window.
func TestAIProviderSubcommand(t *testing.T) {
	oldArgs, oldIn, oldOut := os.Args, os.Stdin, os.Stdout
	defer func() { os.Args, os.Stdin, os.Stdout = oldArgs, oldIn, oldOut }()

	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer outR.Close()
	os.Args = []string{"lightcode", "ai-provider"}
	os.Stdin, os.Stdout = inR, outW
	done := make(chan struct{})
	go func() {
		defer close(done)
		main()
	}()
	if err := aiplugin.WriteMessage(inW, &aiplugin.Message{T: "hello", Proto: aiplugin.Proto}); err != nil {
		t.Fatal(err)
	}
	// Closing stdin ends the plugin after its welcome, as the host's bye
	// would.
	if err := inW.Close(); err != nil {
		t.Fatal(err)
	}
	type read struct {
		m   *aiplugin.Message
		err error
	}
	got := make(chan read, 1)
	go func() {
		m, err := aiplugin.ReadMessage(outR)
		got <- read{m, err}
	}()
	select {
	case r := <-got:
		if r.err != nil {
			t.Fatalf("welcome: %v", r.err)
		}
		if r.m.T != "welcome" || r.m.Proto != aiplugin.Proto || r.m.Name != aiplugin.Name {
			t.Fatalf("welcome = %+v", r.m)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the subcommand did not answer a hello")
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the plugin did not stop after stdin closed")
	}
}
