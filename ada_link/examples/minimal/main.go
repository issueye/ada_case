// Command minimal is the smallest host and child of the library: a window
// of its own on the left, and another process's native UI on the right.
//
//	go run ./examples/minimal
//	go run ./examples/minimal -child   # the child, as the host starts it
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	adalink "github.com/learningdog1/ada_link"
)

// counter is both ends' app: the child draws it, the host calls it.
type counter struct {
	n int
}

type state struct {
	N int `json:"n"`
}

func (a *counter) view(c *ui.Context) {
	ui.Column(c).Fill().Center().Gap(12).Children(func() {
		ui.Text(c, "Minimal Panel").FontSize(22).Bold()
		ui.Text(c, fmt.Sprintf("count %d", a.n)).FontSize(16)
		if ui.PrimaryButton(c, "Increment").Size(220, 48).Clicked() {
			a.n++
		}
	})
}

func (a *counter) call(method string, _ []json.RawMessage) (any, error) {
	if method != "Counter.Increment" {
		return nil, fmt.Errorf("no method %q", method)
	}
	a.n++
	return state{N: a.n}, nil
}

func main() {
	child := flag.Bool("child", false, "run as the child process")
	flag.Parse()
	if *child {
		runChild()
		return
	}
	runHost()
}

func runChild() {
	a := &counter{}
	err := adalink.Serve(adalink.ServeOptions{
		View:    a.view,
		Title:   "Minimal Panel",
		Service: a.call,
	})
	if err != nil {
		log.Fatal(err)
	}
}

func runHost() {
	exe, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}
	sess, err := adalink.Spawn(adalink.SpawnOptions{
		Command: []string{exe, "-child"},
		W:       520,
		H:       320,
		Scale:   1,
		Title:   "Minimal Panel",
	})
	if err != nil {
		log.Fatal(err)
	}

	var panel *adalink.Host
	var calls int
	var win *mygo.Window
	mygo.App.WhenReady(func() {
		win = mygo.NewWindow(mygo.WindowOptions{
			Title:           "Minimal Host",
			Width:           820,
			Height:          460,
			BackgroundColor: "#ffffff",
			Content: ui.View(func(c *ui.Context) {
				ui.Row(c).Fill().Children(func() {
					ui.Column(c).Width(240).Padding(20).Gap(10).Children(func() {
						ui.Text(c, "Host process").FontSize(20).Bold()
						ui.Text(c, "The right side is another process's ui.").FontSize(12)
						ui.Text(c, fmt.Sprintf("Calls: %d", calls)).FontSize(12)
						if ui.PrimaryButton(c, "Call the child").Clicked() {
							calls++
							go callChild(sess)
						}
					})
					if panel != nil {
						panel.View(c)
					} else {
						ui.Text(c, "Waiting…")
					}
				})
			}),
		})
		win.OnClosed(func() { sess.Close() })
		panel = adalink.Attach(win, sess)
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

func callChild(sess *adalink.Session) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := sess.Call(ctx, "Counter.Increment"); err != nil {
		log.Printf("minimal: %v", err)
	}
}
