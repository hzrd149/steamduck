// Command steamduck integrates the kwakore napplet runtime into SteamOS:
// it installs kwakore, runs the napplet store, launches napplets as Steam
// games and keeps Steam's shortcuts in step with installed napplets.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/hzrd149/steamduck/internal/flatpak"
	"github.com/hzrd149/steamduck/internal/hostlib"
	"github.com/hzrd149/steamduck/internal/installer"
	"github.com/hzrd149/steamduck/internal/kwakrpc"
	"github.com/hzrd149/steamduck/internal/launcher"
	"github.com/hzrd149/steamduck/internal/library"
	"github.com/hzrd149/steamduck/internal/paths"
	"github.com/hzrd149/steamduck/internal/steam"
	"github.com/hzrd149/steamduck/internal/store"
)

var version = "development"

const usage = `steamduck - kwakore napplets on the Steam Deck

Usage:
  steamduck install [--kwakore-version TAG] [--kwakore-archive FILE] [--kwakore-installer FILE] [--skip-kwakore]
  steamduck uninstall [--kwakore] [--keep-shortcuts] [--restart-steam]
  steamduck doctor [--json]
  steamduck store [--debug]
  steamduck launch --napplet ADDRESS [--appid ID] [--try]
  steamduck sync [--watch] [--force] [--restart-steam] [--json]
  steamduck shortcuts
  steamduck runtime install|status
  steamduck version

Commands:
  install    install kwakore and steamduck for this user (no root needed)
  uninstall  remove steamduck, its Steam shortcuts, files and service
  doctor     check the runtime libraries, services and display
  store      open the napplet store
  launch     run a napplet as a Steam game (what Steam shortcuts run)
  sync       write Steam shortcuts for installed napplets
  shortcuts  print the wanted shortcuts as JSON (for the Decky plugin)
  runtime    install or check the Flatpak runtime used when the system
             has no WebKitGTK 4.1 (SteamOS)
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]
	log := newLogger(cmd)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()

	var err error
	switch cmd {
	case "install":
		err = runInstall(ctx, log, args)
	case "uninstall":
		err = runUninstall(ctx, log, args)
	case "doctor":
		err = runDoctor(ctx, args)
	case "store":
		err = runStore(ctx, log, args)
	case "launch":
		err = runLaunch(ctx, log, args)
	case "sync":
		err = runSync(ctx, log, args)
	case "shortcuts":
		err = runShortcuts(ctx, log)
	case "runtime":
		err = runRuntime(ctx, log, args)
	case "version", "--version":
		fmt.Println(version)
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Error(cmd+" failed", "err", err)
		os.Exit(1)
	}
}

// logFile is steamduck.log, open when the command was started by Steam.
var logFile *os.File

// newLogger logs to stderr. Steam discards a game's output (often by
// pointing it at /dev/null, which looks like a terminal), so the commands
// Steam runs always log to steamduck.log as well.
func newLogger(cmd string) *slog.Logger {
	var w io.Writer = os.Stderr
	if cmd == "launch" || cmd == "store" {
		if err := os.MkdirAll(paths.DataDir(), 0o700); err == nil {
			if f, err := os.OpenFile(filepath.Join(paths.DataDir(), "steamduck.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
				logFile = f
				w = io.MultiWriter(os.Stderr, f)
			}
		}
	}
	level := slog.LevelInfo
	if os.Getenv("STEAMDUCK_DEBUG") != "" {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level})).With("cmd", cmd)
}

func client() (*kwakrpc.Client, error) {
	return kwakrpc.New(os.Getenv("STEAMDUCK_KWAKORE_SOCKET"))
}

func runInstall(ctx context.Context, log *slog.Logger, args []string) error {
	fs := flag.NewFlagSet("install", flag.ExitOnError)
	opts := installer.Options{Log: log}
	fs.StringVar(&opts.KwakoreVersion, "kwakore-version", "", "kwakore release tag to install (default: latest)")
	fs.StringVar(&opts.KwakoreArchive, "kwakore-archive", "", "install a local kwakore-linux-ARCH.tar.gz")
	fs.StringVar(&opts.KwakoreInstaller, "kwakore-installer", "", "use a local copy of kwakore's scripts/install.sh")
	fs.BoolVar(&opts.SkipKwakore, "skip-kwakore", false, "leave the kwakore installation alone")
	_ = fs.Parse(args)
	if err := installer.Install(ctx, opts); err != nil {
		return err
	}
	// first shortcut sync; deferred while Steam runs
	if err := runSync(ctx, log, nil); err != nil {
		log.Warn("initial shortcut sync", "err", err)
	}
	fmt.Println("steamduck is installed. Run `steamduck doctor` to check the runtime.")
	return nil
}

func runUninstall(ctx context.Context, log *slog.Logger, args []string) error {
	fs := flag.NewFlagSet("uninstall", flag.ExitOnError)
	kwakore := fs.Bool("kwakore", false, "also remove kwakore (its settings and napplets are kept)")
	keep := fs.Bool("keep-shortcuts", false, "leave the Steam shortcuts in place")
	restart := fs.Bool("restart-steam", false, "close Steam, remove the shortcuts, then start Steam again")
	_ = fs.Parse(args)

	root, rootErr := steam.Root()
	removeShortcuts := !*keep && rootErr == nil
	if removeShortcuts && steam.Running() {
		if !*restart {
			return errors.New("Steam is running and would write the shortcuts back; close Steam first, " +
				"or pass --restart-steam (or --keep-shortcuts)")
		}
		log.Info("closing Steam to remove the shortcuts")
		if err := steam.Shutdown(ctx); err != nil {
			return err
		}
		defer func() {
			if steam.InGameMode() {
				return // the gamescope session starts Steam again itself
			}
			log.Info("starting Steam")
			if err := steam.Start(); err != nil {
				log.Error("start Steam", "err", err)
			}
		}()
	}
	// stop the sync service before it can put shortcuts back
	_ = installer.StopSync(ctx)
	if removeShortcuts {
		changes, err := library.RemoveAll(root)
		if err != nil {
			return fmt.Errorf("remove Steam shortcuts: %w", err)
		}
		for user, ch := range changes {
			log.Info("removed Steam shortcuts", "steam_account", user, "removed", len(ch.Removed))
		}
	} else if !*keep {
		log.Info("no Steam installation found; no shortcuts to remove", "err", rootErr)
	}
	if err := installer.Uninstall(ctx, installer.UninstallOptions{Kwakore: *kwakore, Log: log}); err != nil {
		return err
	}
	if *kwakore {
		fmt.Println("steamduck and kwakore are uninstalled.")
	} else {
		fmt.Println("steamduck is uninstalled. kwakore is still installed; to remove it too, run:\n" +
			"  curl -fsSL https://raw.githubusercontent.com/" + installer.KwakoreRepo + "/master/scripts/uninstall.sh | bash")
	}
	return nil
}

func runDoctor(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "print JSON")
	_ = fs.Parse(args)
	checks := installer.Doctor(ctx)
	if c, err := client(); err == nil {
		if st, err := c.Status(ctx); err != nil {
			checks = append(checks, installer.Check{Name: "kwakore daemon", Detail: err.Error()})
		} else {
			checks = append(checks, installer.Check{Name: "kwakore daemon", OK: st.Health.Ready && st.ProtocolVersion == kwakrpc.ProtocolVersion,
				Detail: fmt.Sprintf("version %s, protocol %d, %d windows", st.Health.Version, st.ProtocolVersion, st.Health.ActiveWindows)})
		}
		c.Close()
	}
	if root, err := steam.Root(); err != nil {
		checks = append(checks, installer.Check{Name: "steam", Detail: err.Error()})
	} else {
		users, _ := steam.Users(root)
		checks = append(checks, installer.Check{Name: "steam", OK: len(users) > 0, Detail: fmt.Sprintf("%s, %d account(s), running=%v", root, len(users), steam.Running())})
	}
	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(checks)
	}
	failed := false
	for _, c := range checks {
		mark := "ok  "
		if !c.OK {
			mark, failed = "FAIL", true
		}
		fmt.Printf("[%s] %-28s %s\n", mark, c.Name, c.Detail)
		if !c.OK && c.Fix != "" {
			fmt.Printf("       fix: %s\n", c.Fix)
		}
	}
	if failed {
		return errors.New("some checks failed")
	}
	return nil
}

func newSyncer(log *slog.Logger, c *kwakrpc.Client) (*library.Syncer, error) {
	cfg, err := paths.LoadConfig()
	if err != nil {
		return nil, err
	}
	root, err := steam.Root()
	if err != nil {
		return nil, err
	}
	return &library.Syncer{Client: c, Config: cfg, Exe: shortcutExe(), SteamRoot: root, Log: log}, nil
}

// shortcutExe is the binary Steam shortcuts run: the installed copy when
// there is one, so shortcuts survive deleting a downloaded binary.
func shortcutExe() string {
	installed := filepath.Join(paths.LibDir(), "steamduck")
	if _, err := os.Stat(installed); err == nil {
		return installed
	}
	exe, _ := os.Executable()
	exe, _ = filepath.EvalSymlinks(exe)
	return exe
}

func runSync(ctx context.Context, log *slog.Logger, args []string) error {
	fs := flag.NewFlagSet("sync", flag.ExitOnError)
	watch := fs.Bool("watch", false, "keep running and sync after every install or uninstall")
	force := fs.Bool("force", false, "write shortcuts.vdf even while Steam runs")
	asJSON := fs.Bool("json", false, "print the result as JSON")
	restart := fs.Bool("restart-steam", false, "close Steam, write the shortcuts, then start Steam again")
	_ = fs.Parse(args)

	c, err := client()
	if err != nil {
		return err
	}
	defer c.Close()
	s, err := newSyncer(log, c)
	if err != nil {
		return err
	}
	s.Force = *force
	report := func(res library.Result) {
		if *asJSON {
			_ = json.NewEncoder(os.Stdout).Encode(res)
			return
		}
		if res.Deferred {
			log.Info("Steam is running; shortcuts will be written after it exits (or use --force)")
			return
		}
		for user, ch := range res.Changes {
			log.Info("shortcuts synced", "steam_account", user, "added", len(ch.Added), "updated", len(ch.Updated), "removed", len(ch.Removed))
		}
	}
	if *watch {
		return s.Watch(ctx, report)
	}
	if *restart && steam.Running() {
		log.Info("closing Steam to write shortcuts")
		if err := steam.Shutdown(ctx); err != nil {
			return err
		}
		defer func() {
			if steam.InGameMode() {
				return // the gamescope session starts Steam again itself
			}
			log.Info("starting Steam")
			if err := steam.Start(); err != nil {
				log.Error("start Steam", "err", err)
			}
		}()
	}
	res, err := s.Sync(ctx)
	if err != nil {
		return err
	}
	report(res)
	return nil
}

func runShortcuts(ctx context.Context, log *slog.Logger) error {
	c, err := client()
	if err != nil {
		return err
	}
	defer c.Close()
	s, err := newSyncer(log, c)
	if err != nil {
		return err
	}
	want, err := s.Wanted(ctx)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(want)
}

func runLaunch(ctx context.Context, log *slog.Logger, args []string) error {
	fs := flag.NewFlagSet("launch", flag.ExitOnError)
	address := fs.String("napplet", "", "canonical napplet address")
	appID := fs.Uint64("appid", 0, "Steam app id to tag windows with (default: from Steam's environment)")
	try := fs.Bool("try", false, "open without installing")
	_ = fs.Parse(args)
	if *address == "" {
		return errors.New("launch needs --napplet ADDRESS")
	}
	cfg, err := paths.LoadConfig()
	if err != nil {
		return err
	}
	// No WebKitGTK on the host (SteamOS): become the window inside the
	// Flatpak runtime.
	if flatpak.Use(cfg) {
		c, err := client()
		if err != nil {
			return err
		}
		log.Info("launching in the Flatpak runtime", "napplet", *address, "runtime", cfg.FlatpakRuntime)
		launchCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
		defer cancel()
		return flatpak.ExecWindow(launchCtx, c, *address, *try, cfg.FlatpakRuntime, logFile)
	}
	// Preferred: become the window ourselves (kwakore attached launch).
	if !*try && launcher.SupportsRun() {
		log.Info("launching attached", "napplet", *address)
		return launcher.ExecRun(*address, logFile)
	}

	id := uint32(*appID)
	if id == 0 {
		id = launcher.AppIDFromEnv()
	}
	if id == 0 {
		// started outside Steam, or a client that exports no id: assume the
		// shortcut steamduck itself wrote
		id = steam.NappletShortcut(shortcutExe(), *address, "").AppID()
	}
	c, err := client()
	if err != nil {
		return err
	}
	defer c.Close()
	log.Info("launching through the daemon (kwak run unavailable)", "napplet", *address, "app_id", id)
	return launcher.Run(ctx, c, launcher.Options{Address: *address, AppID: id, Try: *try, Log: log})
}

func runStore(ctx context.Context, log *slog.Logger, args []string) error {
	fs := flag.NewFlagSet("store", flag.ExitOnError)
	debug := fs.Bool("debug", false, "enable the webview inspector")
	_ = fs.Parse(args)

	if flatpak.InSandbox() {
		return fmt.Errorf("the native store broker must run on the host; start steamduck store outside Flatpak")
	}

	c, err := client()
	if err != nil {
		return err
	}
	defer c.Close()
	native := c.WithoutRetry()
	defer native.Close()
	c = native
	api := &store.API{Client: c, Log: log, AppID: launcher.AppIDFromEnv(), Stderr: logFile}
	api.OnSync = func(ctx context.Context) (any, error) {
		s, err := newSyncer(log, c)
		if err != nil {
			return nil, err
		}
		return s.Sync(ctx)
	}
	cfg, err := paths.LoadConfig()
	if err != nil {
		return err
	}
	return store.RunBuiltin(ctx, api, cfg, *debug)
}
func runRuntime(ctx context.Context, log *slog.Logger, args []string) error {
	if len(args) != 1 || (args[0] != "install" && args[0] != "status") {
		return errors.New("usage: steamduck runtime install|status")
	}
	cfg, err := paths.LoadConfig()
	if err != nil {
		return err
	}
	switch args[0] {
	case "install":
		log.Info("installing the Flatpak runtime (about 1 GB on disk)", "runtime", cfg.FlatpakRuntime)
		if err := flatpak.Install(ctx, cfg.FlatpakRuntime, os.Stderr); err != nil {
			return err
		}
		fmt.Println("installed", cfg.FlatpakRuntime)
	case "status":
		fmt.Printf("window runtime: %s (using Flatpak: %v)\n", cfg.WindowRuntime, flatpak.Use(cfg))
		fmt.Printf("host WebKitGTK 4.1: %s\n", orNone(hostlib.Find(hostlib.WebKit)))
		fmt.Printf("flatpak runtime %s installed: %v\n", cfg.FlatpakRuntime, flatpak.Available() && flatpak.Installed(ctx, cfg.FlatpakRuntime))
	}
	return nil
}

func orNone(s string) string {
	if s == "" {
		return "not found"
	}
	return s
}
