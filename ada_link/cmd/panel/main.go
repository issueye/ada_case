// Command ada-panel is the child app of the demo: the UI that Ada Link
// embeds in a process of its own.
//
// Started with Ada Link's environment (link.EnvSocket), it draws that UI
// headless and serves it to the host. Started on its own, it shows the same
// UI in an ordinary window, which makes the embedding easy to see: the same
// native UI, standalone or inside another app's window.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/learningdog1/ada_link/link"
	"github.com/learningdog1/ada_link/mygoembed"
)

func main() {
	shot := flag.String("shot", "", "capture the standalone window to this PNG, then close")
	autoClose := flag.Duration("auto-close", 0, "close the standalone window after this long")
	flag.Parse()

	app := &panel{embedded: os.Getenv(link.EnvSocket) != ""}
	if app.embedded {
		if err := mygoembed.Serve(mygoembed.ServeOptions{
			View:    app.view,
			Title:   "Ada Panel",
			Service: app.call,
		}); err != nil {
			log.Fatal(err)
		}
		return
	}
	runStandalone(app, *shot, *autoClose)
}

// panel is both apps' UI: a counter, and whether it runs embedded.
type panel struct {
	n        int
	embedded bool
}

type state struct {
	N        int  `json:"n"`
	Embedded bool `json:"embedded"`
}

func (p *panel) view(c *ui.Context) {
	ui.Column(c).Fill().Center().Gap(16).Children(func() {
		ui.Text(c, "Ada Panel").FontSize(28).Bold()
		mode := "Standalone window — start Ada Link to embed this UI"
		if p.embedded {
			mode = "Embedded in Ada Link's window, from another process"
		}
		ui.Text(c, mode).FontSize(13)
		if ui.PrimaryButton(c, fmt.Sprintf("count %d · click me", p.n)).Size(300, 58).FontSize(20).Clicked() {
			p.n++
		}
	})
}

func (p *panel) call(method string, _ []json.RawMessage) (any, error) {
	switch method {
	case "Panel.Increment":
		p.n++
	case "Panel.State":
	default:
		return nil, fmt.Errorf("no method %q", method)
	}
	return state{N: p.n, Embedded: p.embedded}, nil
}

func runStandalone(app *panel, shot string, autoClose time.Duration) {
	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{
			Title:           "Ada Panel",
			Width:           720,
			Height:          520,
			BackgroundColor: "#ffffff",
			Content: ui.View(func(c *ui.Context) {
				app.view(c)
			}),
		})
		if autoClose > 0 {
			time.AfterFunc(autoClose, func() { win.Close() })
		}
		if shot != "" {
			time.AfterFunc(900*time.Millisecond, func() {
				if png, err := win.CapturePage(); err != nil {
					log.Printf("panel: capture: %v", err)
				} else if err := os.WriteFile(shot, png, 0o644); err != nil {
					log.Printf("panel: write %s: %v", shot, err)
				} else {
					log.Printf("panel: wrote %s", shot)
				}
				win.Close()
			})
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
