// Command multi shows two embedded panels in one window, kept by a
// panels.Registry: namespaced events from each child, and a broadcast from
// the host to both.
//
//	go run ./examples/multi
//	go run ./examples/multi -child      # one child, as the host starts them
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	adalink "github.com/learningdog1/ada_link"
	"github.com/learningdog1/ada_link/panels"
)

type childApp struct {
	c     *adalink.Child
	n     int
	pings atomic.Int64
}

type state struct {
	N int `json:"n"`
}

func (a *childApp) view(c *ui.Context) {
	ui.Column(c).Fill().Center().Gap(10).Children(func() {
		ui.Text(c, "Panel").FontSize(22).Bold()
		ui.Text(c, fmt.Sprintf("count %d", a.n)).FontSize(16)
		ui.Text(c, fmt.Sprintf("pings %d", a.pings.Load())).FontSize(13)
		if ui.PrimaryButton(c, "Increment").Size(200, 44).Clicked() {
			a.n++
		}
	})
}

func (a *childApp) call(method string, _ []json.RawMessage) (any, error) {
	if method != "Panel.Increment" {
		return nil, fmt.Errorf("no method %q", method)
	}
	a.n++
	return state{N: a.n}, nil
}

func main() {
	child := flag.Bool("child", false, "run as a child process")
	shot := flag.String("shot", "", "capture the window to this PNG, then close")
	shots := flag.String("shots", "", "resize through several sizes, capturing a PNG in this directory, then close")
	drag := flag.String("drag", "", "resize the window continuously, capturing PNGs in this directory")
	probe := flag.String("probe", "", "capture the window every moment into this directory, for a live resize")
	autoClose := flag.Duration("auto-close", 0, "close the window after this long")
	flag.Parse()
	if *child {
		runChild()
		return
	}
	runHost(*shot, *shots, *drag, *probe, *autoClose)
}

func runChild() {
	a := &childApp{}
	err := adalink.Serve(adalink.ServeOptions{
		View:    a.view,
		Title:   "Multi Panel",
		Service: a.call,
		OnReady: func(c *adalink.Child) {
			a.c = c
			// The host's broadcasts reach every panel; each answers with its
			// own pong, namespaced by the registry on the host side.
			c.OnEvent("ping", func(json.RawMessage) {
				a.pings.Add(1)
				c.Refresh()
				_ = c.Emit("pong", state{N: int(a.pings.Load())})
			})
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}

func runHost(shot, shots, drag, probe string, autoClose time.Duration) {
	exe, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}

	reg := panels.New()
	defer reg.Close()

	for _, spec := range []struct{ id, title string }{{"left", "Left"}, {"right", "Right"}} {
		sess, err := adalink.Spawn(adalink.SpawnOptions{
			Command: []string{exe, "-child"},
			W:       320,
			H:       260,
			Scale:   1,
			Title:   spec.title,
			Stderr:  os.Stderr,
		})
		if err != nil {
			log.Fatal(err)
		}
		if _, err := reg.Add(spec.id, spec.title, sess); err != nil {
			sess.Close()
			log.Fatal(err)
		}
	}

	var win *mygo.Window
	var pongs atomic.Int64
	mygo.App.WhenReady(func() {
		win = mygo.NewWindow(mygo.WindowOptions{
			Title:           "Multi-panel host",
			Width:           900,
			Height:          480,
			BackgroundColor: "#ffffff",
			Content: ui.View(func(c *ui.Context) {
				ui.Row(c).Fill().Children(func() {
					ui.Column(c).Width(240).Padding(20).Gap(10).Children(func() {
						ui.Text(c, "Host").FontSize(24).Bold()
						ui.Text(c, "Two panels, two processes, one registry.").FontSize(12)
						ui.Text(c, fmt.Sprintf("pongs received: %d", pongs.Load())).FontSize(12)
						if ui.PrimaryButton(c, "Broadcast ping").Clicked() {
							go func() {
								res := reg.Broadcast("ping", map[string]string{"from": "host"})
								if err := res.Err(); err != nil {
									log.Printf("multi: %v", err)
								}
							}()
						}
					})
					for _, id := range reg.IDs() {
						snap, ok := reg.Get(id)
						if !ok || snap.Host == nil {
							continue
						}
						ui.Box(c).Fill().Children(func() {
							snap.Host.View(c)
						})
					}
				})
			}),
		})
		reg.OnEvent("pong", func(panels.Event) {
			pongs.Add(1)
			win.Invalidate()
		})
		win.OnClosed(func() { reg.Close() })
		for _, id := range reg.IDs() {
			if _, err := reg.Attach(id, win); err != nil {
				log.Printf("multi: attach %s: %v", id, err)
			}
		}
		win.Invalidate()
		if autoClose > 0 {
			time.AfterFunc(autoClose, func() { win.Close() })
		}
		if shot != "" {
			time.AfterFunc(1800*time.Millisecond, func() {
				if png, err := win.CapturePage(); err != nil {
					log.Printf("multi: capture: %v", err)
				} else if err := os.WriteFile(shot, png, 0o644); err != nil {
					log.Printf("multi: write %s: %v", shot, err)
				} else {
					log.Printf("multi: wrote %s", shot)
				}
				win.Close()
			})
		}
		if shots != "" {
			go resizeShots(win, shots)
		}
		if drag != "" {
			go dragProbe(win, drag)
		}
		if probe != "" {
			go probeShots(win, probe)
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// dragProbe resizes the window continuously, as dragging its edge does, and
// captures frames meanwhile: a diagnostic for the live-resize path.
func dragProbe(win *mygo.Window, dir string) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("multi: %v", err)
		return
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for w := 900; w <= 1220; w += 8 {
			win.SetSize(w, 480+(w-900)/2)
			time.Sleep(10 * time.Millisecond)
		}
		for w := 1220; w >= 900; w -= 8 {
			win.SetSize(w, 480+(w-900)/2)
			time.Sleep(10 * time.Millisecond)
		}
	}()
	for i := 0; i < 16; i++ {
		time.Sleep(90 * time.Millisecond)
		select {
		case <-done:
			win.Close()
			return
		default:
		}
		png, err := win.CapturePage()
		if err != nil {
			log.Printf("multi: capture: %v", err)
			continue
		}
		path := filepath.Join(dir, fmt.Sprintf("drag-%02d.png", i))
		if err := os.WriteFile(path, png, 0o644); err != nil {
			log.Printf("multi: write: %v", err)
			continue
		}
		log.Printf("multi: wrote %s", path)
	}
	win.Close()
}

// probeShots captures the window's content, not the screen, every quarter
// second for a while: a live resize can then be seen in the host's own
// frames rather than through DWM's window shadow.
func probeShots(win *mygo.Window, dir string) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("multi: %v", err)
		return
	}
	for i := 0; i < 32; i++ {
		time.Sleep(250 * time.Millisecond)
		png, err := win.CapturePage()
		if err != nil {
			log.Printf("multi: capture: %v", err)
			continue
		}
		path := filepath.Join(dir, fmt.Sprintf("probe-%02d.png", i))
		if err := os.WriteFile(path, png, 0o644); err != nil {
			log.Printf("multi: write: %v", err)
			continue
		}
		log.Printf("multi: wrote %s", path)
	}
	win.Close()
}

// resizeShots steps the window through sizes, capturing each once it settled:
// a diagnostic for the resize path.
func resizeShots(win *mygo.Window, dir string) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("multi: %v", err)
		return
	}
	sizes := []struct{ w, h int }{
		{1200, 700}, {760, 420}, {1100, 620}, {900, 480},
	}
	for i, s := range sizes {
		win.SetSize(s.w, s.h)
		time.Sleep(1100 * time.Millisecond)
		path := filepath.Join(dir, fmt.Sprintf("size-%d-%dx%d.png", i+1, s.w, s.h))
		png, err := win.CapturePage()
		if err != nil {
			log.Printf("multi: capture: %v", err)
			continue
		}
		if err := os.WriteFile(path, png, 0o644); err != nil {
			log.Printf("multi: write: %v", err)
			continue
		}
		log.Printf("multi: wrote %s", path)
	}
	win.Close()
}
