// Command ada-link is the host app of the demo: a MyGo window whose own
// native UI surrounds another process's native UI.
//
//	go run ./cmd/linkapp                  # the host window
//	go run ./cmd/linkapp -child           # the child, as the host starts it
//	go run ./cmd/linkapp install          # install for the current user
//	go run ./cmd/linkapp uninstall        # remove it again
//
// The host starts AdaPanel.exe next to it when the build has one, and itself
// with -child otherwise, so a single binary is a complete demo too.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	adalink "github.com/learningdog1/ada_link"
	"github.com/learningdog1/ada_link/install"
	"github.com/learningdog1/ada_link/link"
	"github.com/learningdog1/ada_link/mygoembed"
)

// What the demo installs, and what Settings and its shortcuts show.
const (
	appName   = "Ada Link"
	panelName = "Ada Panel"
	publisher = "learningdog1"
)

// appInstall describes the demo for package install: two executables that
// sit together, a Start Menu entry and a desktop shortcut for the host, and
// the flags Settings passes to it to uninstall.
func appInstall() install.Config {
	src, err := os.Executable()
	if err != nil {
		src = "AdaLink.exe"
	}
	dir := filepath.Dir(src)
	return install.Config{
		Name:      appName,
		Version:   adalink.Version,
		Publisher: publisher,
		Dir:       install.DefaultDir("AdaLink"),
		Key:       "AdaLink",
		Exes: map[string]string{
			"AdaLink.exe":  src,
			"AdaPanel.exe": filepath.Join(dir, "AdaPanel.exe"),
		},
		Host: "AdaLink.exe",
		Shortcuts: []install.Shortcut{
			{Name: appName, Exe: "AdaLink.exe", Description: "Embed another MyGo app's native UI", Desktop: true},
			{Name: panelName, Exe: "AdaPanel.exe", Description: "Ada Link's child app, standalone"},
		},
		UninstallArgs:      []string{"--uninstall"},
		QuietUninstallArgs: []string{"--uninstall", "--yes", "--quiet"},
	}
}

func main() {
	// Accept "install" and "uninstall" as commands, as a person types them,
	// with their flags after: the flag package wants the dashes.
	for i := 1; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "install", "uninstall":
			os.Args[i] = "-" + os.Args[i]
		}
	}
	child := flag.Bool("child", false, "run as the child process")
	installCmd := flag.Bool("install", false, "install Ada Link and Ada Panel for this user")
	uninstallCmd := flag.Bool("uninstall", false, "remove Ada Link and Ada Panel for this user")
	yes := flag.Bool("yes", false, "uninstall without asking")
	quiet := flag.Bool("quiet", false, "no dialogs")
	autoClose := flag.Duration("auto-close", 0, "close the window after this long, for a smoke test")
	shot := flag.String("shot", "", "capture the window to this PNG after a moment, then close")
	flag.Parse()
	switch {
	case *child:
		runChild()
	case *installCmd:
		os.Exit(runInstall(*quiet))
	case *uninstallCmd:
		os.Exit(runUninstall(*yes, *quiet))
	default:
		runHost(*autoClose, *shot)
	}
}

// demo is the fallback child: when the build has no AdaPanel.exe next to the
// host, the host starts itself with -child and this is what it shows.
type demo struct {
	n int
}

type state struct {
	N int `json:"n"`
}

func (d *demo) view(c *ui.Context) {
	ui.Column(c).Fill().Center().Gap(16).Children(func() {
		ui.Text(c, "Ada Panel").FontSize(28).Bold()
		ui.Text(c, "Embedded in Ada Link's window, from another process").FontSize(13)
		if ui.PrimaryButton(c, fmt.Sprintf("count %d · click me", d.n)).Size(300, 58).FontSize(20).Clicked() {
			d.n++
		}
	})
}

func (d *demo) call(method string, _ []json.RawMessage) (any, error) {
	switch method {
	case "Panel.Increment":
		d.n++
	case "Panel.State":
	default:
		return nil, fmt.Errorf("no method %q", method)
	}
	return state{N: d.n}, nil
}

func runChild() {
	d := &demo{}
	if err := mygoembed.Serve(mygoembed.ServeOptions{View: d.view, Title: "Ada Panel", Service: d.call}); err != nil {
		log.Fatal(err)
	}
}

// childCommand is AdaPanel.exe next to the host when there is one, else the
// host itself with -child.
func childCommand() ([]string, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	panel := filepath.Join(filepath.Dir(exe), "AdaPanel.exe")
	if _, err := os.Stat(panel); err == nil {
		return []string{panel}, nil
	}
	return []string{exe, "-child"}, nil
}

// install is the install state the window shows, main thread only.
var installState struct {
	installed bool
	dir       string
}

func runInstall(quiet bool) int {
	res, err := install.Install(appInstall())
	if err != nil {
		if !quiet {
			install.Warn("Install failed", err.Error())
		}
		return 1
	}
	if !quiet {
		install.Inform("Ada Link installed", fmt.Sprintf(
			"Ada Link %s and Ada Panel are installed in\n%s\n\nStart Menu: Ada Link, Ada Panel\nSettings → Apps → Installed apps lists them.\n\nThe portable copies you ran from are untouched.",
			adalink.Version, res.Dir))
	}
	return 0
}

func runUninstall(yes, quiet bool) int {
	if !yes && !install.Confirm("Uninstall Ada Link", "Remove Ada Link and Ada Panel from this computer?") {
		return 0
	}
	needQuit, err := install.Uninstall(appInstall())
	if err != nil {
		if !quiet {
			install.Warn("Uninstall failed", err.Error())
		}
		return 1
	}
	// A running installed app cannot delete its own files: the directory goes
	// a moment after this process exits.
	if !needQuit && !quiet {
		install.Inform("Ada Link uninstalled", "The installed files, shortcuts and the Settings entry are gone.")
	}
	return 0
}

// hostState is the window's state, main thread only.
type hostState struct {
	count int
	sess  *link.Session
	err   string
}

func runHost(autoClose time.Duration, shot string) {
	installState.installed, installState.dir = install.Status(appInstall())
	st := &hostState{}
	var panel *mygoembed.Host
	var win *mygo.Window

	mygo.App.WhenReady(func() {
		win = mygo.NewWindow(mygo.WindowOptions{
			Title:           "Ada Link — host and child",
			Width:           1080,
			Height:          700,
			BackgroundColor: "#ffffff",
			Content: ui.View(func(c *ui.Context) {
				ui.Row(c).Fill().Children(func() {
					ui.Column(c).Width(320).Fill().Padding(24).Gap(12).Children(func() {
						ui.Text(c, "Ada Link").FontSize(28).Bold()
						ui.Text(c, "This window is one process's native UI. The panel on the right is another process's native UI, composited from its frames.").FontSize(13)
						ui.Text(c, fmt.Sprintf("Calls made from here: %d", st.count)).FontSize(13)
						if ui.PrimaryButton(c, "Call the child").Clicked() {
							if st.sess != nil {
								st.count++
								go callPanel(st.sess)
							}
						}
						if st.err != "" {
							ui.Text(c, st.err).FontSize(12)
						}
						ui.Box(c).Height(6)
						if installState.installed {
							ui.Text(c, "Installed for this user").FontSize(12)
							if ui.Button(c, "Uninstall").Clicked() {
								go uninstallFromWindow(win, st.sess)
							}
						} else {
							ui.Text(c, "Portable build").FontSize(12)
							if ui.Button(c, "Install…").Clicked() {
								go installFromWindow(win)
							}
						}
					})
					ui.Box(c).Fill().Children(func() {
						if panel != nil {
							panel.View(c)
						} else {
							ui.Text(c, "Waiting for Ada Panel…").FontSize(14)
						}
					})
				})
			}),
		})
		win.OnClosed(func() {
			if st.sess != nil {
				st.sess.Close()
			}
		})

		go func() {
			cmd, err := childCommand()
			if err != nil {
				log.Printf("ada-link: %v", err)
				return
			}
			sess, err := link.Spawn(link.SpawnOptions{
				Command: cmd,
				W:       720,
				H:       520,
				Scale:   1,
				Title:   "Ada Panel",
				Stderr:  os.Stderr,
			})
			if err != nil {
				log.Printf("ada-link: %v", err)
				win.Update(func() { st.err = "Ada Panel did not start: " + err.Error() })
				return
			}
			win.Update(func() {
				st.sess = sess
				panel = mygoembed.Attach(win, sess)
			})
		}()

		if autoClose > 0 {
			time.AfterFunc(autoClose, func() { win.Close() })
		}
		if shot != "" {
			// A moment for the child's first frame and the window's own paint.
			time.AfterFunc(1500*time.Millisecond, func() {
				if png, err := win.CapturePage(); err != nil {
					log.Printf("ada-link: capture: %v", err)
				} else if err := os.WriteFile(shot, png, 0o644); err != nil {
					log.Printf("ada-link: write %s: %v", shot, err)
				} else {
					log.Printf("ada-link: wrote %s", shot)
				}
				win.Close()
			})
		}
	})

	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

func callPanel(sess *link.Session) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := sess.Call(ctx, "Panel.Increment"); err != nil {
		log.Printf("ada-link: %v", err)
	}
}

func installFromWindow(win *mygo.Window) {
	res, err := install.Install(appInstall())
	if err != nil {
		install.Warn("Install failed", err.Error())
		return
	}
	win.Update(func() {
		installState.installed, installState.dir = true, res.Dir
	})
	install.Inform("Ada Link installed", fmt.Sprintf(
		"Ada Link %s and Ada Panel are installed in\n%s\n\nStart Menu: Ada Link, Ada Panel\nSettings → Apps → Installed apps lists them.",
		adalink.Version, res.Dir))
}

func uninstallFromWindow(win *mygo.Window, sess *link.Session) {
	if !install.Confirm("Uninstall Ada Link", "Remove Ada Link and Ada Panel from this computer?") {
		return
	}
	needQuit, err := install.Uninstall(appInstall())
	if err != nil {
		install.Warn("Uninstall failed", err.Error())
		return
	}
	if needQuit {
		// This window runs from the install directory: close it, and the
		// directory goes once the process is gone.
		if sess != nil {
			sess.Close()
		}
		mygo.App.Quit()
		return
	}
	// A portable Ada Link removed the installed copy, and stays.
	win.Update(func() { installState.installed = false })
	install.Inform("Ada Link uninstalled", "The installed files, shortcuts and the Settings entry are gone. This portable copy stays.")
}
