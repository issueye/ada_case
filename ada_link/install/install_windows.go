//go:build windows

// Package install installs a MyGo application for the current Windows user
// and removes it again: its executables under %LOCALAPPDATA%\Programs, Start
// Menu and desktop shortcuts, and the entry Settings shows under Installed
// apps. No administrator is needed, and the portable copies are untouched.
//
// An app describes itself with Config. The app's host executable must
// accept the uninstall arguments, usually --uninstall, because that is what
// Settings runs.
package install

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Shortcut is one Start Menu entry, and optionally a desktop one.
type Shortcut struct {
	// Name is the .lnk file's name, and what the Start Menu and the desktop
	// show.
	Name string
	// Exe is a file name in Config.Dir, one of Config.Exes' targets.
	Exe string
	// Description is the shortcut's comment, as Explorer's tooltip shows it.
	Description string
	// Desktop adds the shortcut to the desktop too.
	Desktop bool
}

// Config describes an app for Install and Uninstall. Both take the same
// Config, so a program builds it once.
type Config struct {
	// Name is what Settings shows, and the default directory's name.
	Name string
	// Version and Publisher are what Settings shows too.
	Version   string
	Publisher string
	// Dir is the install directory; empty for
	// %LOCALAPPDATA%\Programs\<Name>.
	Dir string
	// Exes are the executables to copy, as target file name in Dir to the
	// source path. An app with several binaries lists them all.
	Exes map[string]string
	// Host is the executable Settings runs to uninstall, a name in Exes;
	// empty for the first shortcut's Exe.
	Host string
	// UninstallArgs are the arguments Settings passes to Host, such as
	// --uninstall. Empty for none.
	UninstallArgs []string
	// QuietUninstallArgs are the arguments for an uninstall without
	// dialogs; empty for UninstallArgs.
	QuietUninstallArgs []string
	// Shortcuts are the Start Menu entries; those with Desktop also get a
	// desktop shortcut.
	Shortcuts []Shortcut
	// Key is the uninstall registry key's name under
	// HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall; empty for
	// Name.
	Key string
}

// Result is what Install did.
type Result struct {
	// Dir is where the files went.
	Dir string
	// Shortcuts are the .lnk files made.
	Shortcuts []string
}

// DefaultDir returns where an app named name installs by default.
func DefaultDir(name string) string {
	if base := os.Getenv("LOCALAPPDATA"); base != "" {
		return filepath.Join(base, "Programs", name)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), name)
	}
	return filepath.Join(home, "Programs", name)
}

// Dir returns the configured directory, or its default.
func (c Config) dir() string {
	if c.Dir != "" {
		return c.Dir
	}
	return DefaultDir(c.Name)
}

// key returns the uninstall key's name.
func (c Config) key() string {
	if c.Key != "" {
		return c.Key
	}
	return c.Name
}

// host returns the executable Settings runs, checked against Exes.
func (c Config) host() (string, error) {
	if c.Host != "" {
		if _, ok := c.Exes[c.Host]; !ok {
			return "", fmt.Errorf("install: Host %q is not one of Exes", c.Host)
		}
		return c.Host, nil
	}
	for _, sc := range c.Shortcuts {
		if _, ok := c.Exes[sc.Exe]; ok && filepath.Base(sc.Exe) == sc.Exe {
			return sc.Exe, nil
		}
	}
	if len(c.Exes) == 1 {
		for target := range c.Exes {
			return target, nil
		}
	}
	return "", errors.New("install: Config.Host is empty and no shortcut names one")
}

// validate checks the parts Install and Uninstall need.
func (c Config) validate() error {
	if c.Name == "" {
		return errors.New("install: Config.Name is empty")
	}
	if len(c.Exes) == 0 {
		return errors.New("install: Config.Exes is empty")
	}
	for target := range c.Exes {
		if target == "" || filepath.Base(target) != target {
			return fmt.Errorf("install: Exes target %q is not a file name", target)
		}
	}
	for _, sc := range c.Shortcuts {
		if sc.Name == "" || filepath.Base(sc.Name) != sc.Name {
			return fmt.Errorf("install: Shortcut.Name %q is not a name", sc.Name)
		}
		if _, ok := c.Exes[sc.Exe]; !ok {
			return fmt.Errorf("install: shortcut %q runs %q, which is not one of Exes", sc.Name, sc.Exe)
		}
	}
	if _, err := c.host(); err != nil {
		return err
	}
	return nil
}

// Status reports whether the app is installed, and where.
func Status(cfg Config) (installed bool, dir string) {
	h, err := cfg.host()
	if err != nil {
		return false, cfg.dir()
	}
	dir = cfg.dir()
	_, err = os.Stat(filepath.Join(dir, h))
	return err == nil, dir
}

// Install copies the executables into the directory, makes the shortcuts,
// and writes the uninstall entry.
func Install(cfg Config) (Result, error) {
	if err := cfg.validate(); err != nil {
		return Result{}, err
	}
	host, _ := cfg.host()
	dir := cfg.dir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Result{}, fmt.Errorf("install: %w", err)
	}
	for target, src := range cfg.Exes {
		if _, err := os.Stat(src); err != nil {
			return Result{}, fmt.Errorf("install: %s: %w", src, err)
		}
		if err := copyUnlessSame(src, filepath.Join(dir, target)); err != nil {
			return Result{}, fmt.Errorf("install: %w", err)
		}
	}

	var result Result
	result.Dir = dir
	menu := startMenuDir()
	for _, sc := range cfg.Shortcuts {
		if menu != "" {
			lnk := filepath.Join(menu, sc.Name+".lnk")
			if err := makeShortcut(lnk, filepath.Join(dir, sc.Exe), dir, sc.Description); err != nil {
				return result, err
			}
			result.Shortcuts = append(result.Shortcuts, lnk)
		}
		if desk := desktopDir(); sc.Desktop && desk != "" {
			lnk := filepath.Join(desk, sc.Name+".lnk")
			if err := makeShortcut(lnk, filepath.Join(dir, sc.Exe), dir, sc.Description); err != nil {
				return result, err
			}
			result.Shortcuts = append(result.Shortcuts, lnk)
		}
	}
	if err := writeUninstallEntry(cfg, dir, host); err != nil {
		return result, err
	}
	return result, nil
}

// Uninstall removes the shortcuts and the uninstall entry, then the files.
// needQuit is true when the running executable lives in the install
// directory: Windows cannot delete a running executable, so the directory
// goes a moment after the process exits, and the caller must quit.
func Uninstall(cfg Config) (needQuit bool, err error) {
	if err := cfg.validate(); err != nil {
		return false, err
	}
	dir := cfg.dir()
	var errs []error
	for _, lnk := range shortcutPaths(cfg) {
		if err := os.Remove(lnk); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	_ = registry.DeleteKey(registry.CURRENT_USER, uninstallKey(cfg))
	if exe, err := os.Executable(); err == nil && inDir(exe, dir) {
		scheduleRemoveDir(dir)
		return true, errors.Join(errs...)
	}
	if err := os.RemoveAll(dir); err != nil {
		errs = append(errs, err)
	}
	return false, errors.Join(errs...)
}

// Confirm asks a yes/no question in a native dialog, and reports whether
// the user chose Yes.
func Confirm(caption, text string) bool {
	return messageBox(text, caption, windows.MB_YESNO|windows.MB_ICONQUESTION|windows.MB_DEFBUTTON2) == idYes
}

// Inform shows a plain message.
func Inform(caption, text string) {
	messageBox(text, caption, windows.MB_OK|windows.MB_ICONINFORMATION)
}

// Warn shows a message with the warning icon.
func Warn(caption, text string) {
	messageBox(text, caption, windows.MB_OK|windows.MB_ICONWARNING)
}

// idYes is what MessageBox returns for IDYES, which x/sys does not name.
const idYes = 6

func messageBox(text, caption string, flags uint32) int32 {
	t, err := windows.UTF16PtrFromString(text)
	if err != nil {
		return 0
	}
	c, err := windows.UTF16PtrFromString(caption)
	if err != nil {
		return 0
	}
	r, err := windows.MessageBox(0, t, c, flags)
	if err != nil {
		return 0
	}
	return r
}

// shortcutPaths returns the shortcuts Install makes for cfg.
func shortcutPaths(cfg Config) []string {
	var out []string
	menu := startMenuDir()
	for _, sc := range cfg.Shortcuts {
		if menu != "" {
			out = append(out, filepath.Join(menu, sc.Name+".lnk"))
		}
		if sc.Desktop {
			if desk := desktopDir(); desk != "" {
				out = append(out, filepath.Join(desk, sc.Name+".lnk"))
			}
		}
	}
	return out
}

// writeUninstallEntry writes Settings' entry: what it shows, and how it
// uninstalls.
func writeUninstallEntry(cfg Config, dir, host string) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, uninstallKey(cfg), registry.WRITE)
	if err != nil {
		return fmt.Errorf("install: uninstall entry: %w", err)
	}
	defer key.Close()
	exe := filepath.Join(dir, host)
	var sizeKB uint32
	for target := range cfg.Exes {
		if fi, err := os.Stat(filepath.Join(dir, target)); err == nil {
			sizeKB += uint32(fi.Size() / 1024)
		}
	}
	quietArgs := cfg.QuietUninstallArgs
	if len(quietArgs) == 0 {
		quietArgs = cfg.UninstallArgs
	}
	values := map[string]string{
		"DisplayName":     cfg.Name,
		"DisplayVersion":  cfg.Version,
		"Publisher":       cfg.Publisher,
		"DisplayIcon":     exe + ",0",
		"InstallLocation": dir,
		"UninstallString": uninstallCommand(exe, cfg.UninstallArgs),
		"InstallDate":     time.Now().Format("20060102"),
	}
	if len(quietArgs) > 0 {
		values["QuietUninstallString"] = uninstallCommand(exe, quietArgs)
	}
	for name, v := range values {
		if err := key.SetStringValue(name, v); err != nil {
			return fmt.Errorf("install: uninstall entry %s: %w", name, err)
		}
	}
	// Settings then offers no Modify and no Repair.
	if err := key.SetDWordValue("NoModify", 1); err != nil {
		return err
	}
	if err := key.SetDWordValue("NoRepair", 1); err != nil {
		return err
	}
	return key.SetDWordValue("EstimatedSize", sizeKB)
}

// uninstallKey is where Settings' Installed apps reads cfg from.
func uninstallKey(cfg Config) string {
	return `Software\Microsoft\Windows\CurrentVersion\Uninstall\` + cfg.key()
}

// uninstallCommand is the command line Settings runs: the quoted executable
// and its arguments.
func uninstallCommand(exe string, args []string) string {
	out := `"` + exe + `"`
	if len(args) > 0 {
		out += " " + strings.Join(args, " ")
	}
	return out
}

// makeShortcut writes a .lnk through Windows' own shortcut object; the shell
// is the only portable way to make one without cgo.
func makeShortcut(lnk, target, workdir, desc string) error {
	script := fmt.Sprintf(
		`$s=(New-Object -ComObject WScript.Shell).CreateShortcut(%s);`+
			`$s.TargetPath=%s;$s.WorkingDirectory=%s;$s.Description=%s;`+
			`$s.IconLocation=%s;$s.Save()`,
		psQuote(lnk), psQuote(target), psQuote(workdir), psQuote(desc), psQuote(target+",0"))
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("install: shortcut %s: %w: %s", lnk, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// psQuote quotes s for PowerShell, where ” is a quote inside a quote.
func psQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// copyUnlessSame copies src to dst, unless they are the same file.
func copyUnlessSame(src, dst string) error {
	if samePath(src, dst) {
		return nil
	}
	tmp := dst + ".tmp"
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func samePath(a, b string) bool {
	aa, err := filepath.Abs(a)
	if err != nil {
		return false
	}
	bb, err := filepath.Abs(b)
	if err != nil {
		return false
	}
	return strings.EqualFold(filepath.Clean(aa), filepath.Clean(bb))
}

// inDir reports whether exe is a file of dir.
func inDir(exe, dir string) bool {
	e := strings.ToLower(filepath.Clean(exe))
	d := strings.ToLower(filepath.Clean(dir)) + string(os.PathSeparator)
	return strings.HasPrefix(e, d)
}

// scheduleRemoveDir removes dir once the running executable exited: a
// PowerShell script, started without a console, outlives this process and
// deletes the directory and itself.
//
// Not DETACHED_PROCESS: PowerShell started detached never ran the script,
// while CREATE_NO_WINDOW does and survives its parent.
func scheduleRemoveDir(dir string) {
	f, err := os.CreateTemp("", "install-uninstall-*.ps1")
	if err != nil {
		return
	}
	path := f.Name()
	script := fmt.Sprintf("Start-Sleep -Seconds 3\r\n"+
		"Remove-Item -LiteralPath %s -Recurse -Force -ErrorAction SilentlyContinue\r\n"+
		"Remove-Item -LiteralPath $PSCommandPath -Force -ErrorAction SilentlyContinue\r\n",
		psQuote(filepath.ToSlash(dir)))
	if _, err := f.WriteString(script); err != nil {
		f.Close()
		_ = os.Remove(path)
		return
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return
	}
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", path)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	if err := cmd.Start(); err != nil {
		_ = os.Remove(path)
	}
}

// startMenuDir is the user's Start Menu programs folder.
func startMenuDir() string {
	if base := os.Getenv("APPDATA"); base != "" {
		return filepath.Join(base, "Microsoft", "Windows", "Start Menu", "Programs")
	}
	return ""
}

// desktopDir returns the user's desktop, following a OneDrive redirection,
// or "" when there is none.
func desktopDir() string {
	if base := os.Getenv("OneDrive"); base != "" {
		if d := filepath.Join(base, "Desktop"); dirExists(d) {
			return d
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	if d := filepath.Join(home, "Desktop"); dirExists(d) {
		return d
	}
	return ""
}

func dirExists(dir string) bool {
	fi, err := os.Stat(dir)
	return err == nil && fi.IsDir()
}
