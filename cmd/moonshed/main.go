package main

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/crypto/bcrypt"

	"github.com/SiddheshNan/moonshed/internal/config"
	"github.com/SiddheshNan/moonshed/internal/secret"
	"github.com/SiddheshNan/moonshed/internal/server"
	"github.com/SiddheshNan/moonshed/internal/service"
	"github.com/SiddheshNan/moonshed/internal/setup"
	"github.com/SiddheshNan/moonshed/internal/store"
	"github.com/SiddheshNan/moonshed/internal/version"
)

const usage = `Usage: moonshed <command> [-e ENV] [--runtime DIR]

Commands:
  serve                   run Moonshed
  install                 register as a systemd unit / Windows service and start it
  uninstall               stop and remove the service
  setup-link              print the link to create the first admin
  reset-password <user>   set a new random password for a user
  version                 print the version

Flags:
  -e ENV          development or production, selects runtime/config/<ENV>.yaml (or set ENV).
                  Required for serve, install and setup-link.
  --runtime DIR   runtime directory (or set RUNTIME_DIR). Defaults to runtime/ next to the
                  binary, then ./runtime.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(2)
	}
	cmd := os.Args[1]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	fs.Usage = func() { fmt.Print(usage) }
	env := fs.String("e", os.Getenv("ENV"), "environment")
	runtimeDir := fs.String("runtime", envOr("RUNTIME_DIR", config.DefaultRoot()), "runtime directory")
	args := parseAnywhere(fs, os.Args[2:])

	root, err := filepath.Abs(*runtimeDir)
	if err != nil {
		fatal(err)
	}
	paths := config.Paths{Root: root}

	switch cmd {
	case "serve":
		err = serve(*env, paths)
	case "install":
		err = install(*env, paths)
	case "uninstall":
		err = service.Uninstall()
	case "setup-link":
		err = setupLink(*env, paths)
	case "reset-password":
		err = resetPassword(paths, firstArg(args))
	case "version":
		fmt.Println(version.Full())
	default:
		fmt.Print(usage)
		os.Exit(2)
	}
	if err != nil {
		fatal(err)
	}
}

func serve(env string, paths config.Paths) error {
	cfg, err := config.Load(env, paths)
	if err != nil {
		return err
	}
	srv, err := server.New(cfg, paths)
	if err != nil {
		return err
	}
	defer srv.Close()
	return service.Run(srv.Run)
}

// install creates the setup token before the service starts, so the link printed
// here is the one the service accepts.
func install(env string, paths config.Paths) error {
	cfg, err := config.Load(env, paths)
	if err != nil {
		return err
	}
	token, err := pendingSetup(paths)
	if err != nil {
		return err
	}
	if err := service.Install(env, paths.Root); err != nil {
		return err
	}
	fmt.Println()
	if token == "" {
		fmt.Println("Moonshed is running:", setup.PanelURL(cfg.PanelAddr))
		return nil
	}
	fmt.Println("Moonshed is running. Create the admin account:")
	fmt.Println(setup.Link(cfg.PanelAddr, token))
	return nil
}

func setupLink(env string, paths config.Paths) error {
	cfg, err := config.Load(env, paths)
	if err != nil {
		return err
	}
	token, err := pendingSetup(paths)
	if err != nil {
		return err
	}
	if token == "" {
		fmt.Println("Setup is already done. Sign in at", setup.PanelURL(cfg.PanelAddr))
		fmt.Println("Forgot the password? Run: moonshed reset-password <user>")
		os.Exit(1)
	}
	fmt.Println(setup.Link(cfg.PanelAddr, token))
	return nil
}

func pendingSetup(paths config.Paths) (string, error) {
	if err := paths.Init(); err != nil {
		return "", err
	}
	st, err := openStore(paths)
	if err != nil {
		return "", err
	}
	defer st.Close()
	return setup.Pending(st, paths)
}

func openStore(paths config.Paths) (*store.Store, error) {
	box, err := secret.LoadOrCreate(paths.SecretKey())
	if err != nil {
		return nil, err
	}
	return store.Open(paths.DB(), box)
}

func resetPassword(paths config.Paths, username string) error {
	if username == "" {
		return errors.New("usage: moonshed reset-password <user>")
	}
	st, err := openStore(paths)
	if err != nil {
		return err
	}
	defer st.Close()
	u, err := st.UserByName(username)
	if err != nil {
		return fmt.Errorf("user %s: %w", username, err)
	}
	raw := make([]byte, 12)
	rand.Read(raw)
	password := base64.RawURLEncoding.EncodeToString(raw)
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := st.SetPassword(u.ID, string(hash)); err != nil {
		return err
	}
	fmt.Printf("New password for %s: %s\n", username, password)
	return nil
}

// parseAnywhere lets flags come before or after positional arguments,
// e.g. both "reset-password --runtime x admin" and "reset-password admin --runtime x".
func parseAnywhere(fs *flag.FlagSet, args []string) []string {
	var positional []string
	for len(args) > 0 {
		fs.Parse(args)
		args = fs.Args()
		if len(args) > 0 {
			positional = append(positional, args[0])
			args = args[1:]
		}
	}
	return positional
}

func firstArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
