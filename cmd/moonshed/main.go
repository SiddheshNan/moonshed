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
	"github.com/SiddheshNan/moonshed/internal/store"
	"github.com/SiddheshNan/moonshed/internal/version"
)

const usage = `Usage: moonshed <command> [-e ENV] [--runtime DIR]

Commands:
  serve                   run Moonshed
  install                 register as a systemd unit / Windows service and start it
  uninstall               stop and remove the service
  reset-password <user>   set a new random password for a user
  version                 print the version

Flags:
  -e ENV          development or production, selects runtime/config/<ENV>.yaml (or set ENV).
                  Required for serve and install.
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
		if _, err = config.Load(*env, paths); err == nil {
			err = service.Install(*env, root)
		}
	case "uninstall":
		err = service.Uninstall()
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

func resetPassword(paths config.Paths, username string) error {
	if username == "" {
		return errors.New("usage: moonshed reset-password <user>")
	}
	box, err := secret.LoadOrCreate(paths.SecretKey())
	if err != nil {
		return err
	}
	st, err := store.Open(paths.DB(), box)
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
