package main

import (
	tea "charm.land/bubbletea/v2"
	"diskcleaner/internal/config"
	"diskcleaner/internal/policy"
	"diskcleaner/internal/ui"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

var buildVersion = "0.1.0-dev"

func main() {
	if e := run(); e != nil {
		fmt.Fprintf(os.Stderr, "DiskCleaner: %q\n", e.Error())
		os.Exit(1)
	}
}
func run() error {
	root := flag.String("root", "", "initial directory (default: home)")
	version := flag.Bool("version", false, "print version")
	flag.Parse()
	if *version {
		fmt.Println("DiskCleaner " + buildVersion)
		return nil
	}
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("macOS is required")
	}
	home, e := os.UserHomeDir()
	if e != nil {
		return e
	}
	if *root == "" {
		*root = home
	}
	resolved, e := filepath.EvalSymlinks(*root)
	if e != nil {
		return e
	}
	resolved, e = filepath.Abs(resolved)
	if e != nil {
		return e
	}
	if _, e = os.ReadDir(resolved); e != nil {
		return e
	}
	p, e := config.Path()
	if e != nil {
		return e
	}
	settings, e := config.Load(p)
	if e != nil {
		return fmt.Errorf("load settings: %w", e)
	}
	g, e := policy.NewGuard(home, filepath.Dir(p), resolved, settings.Masks)
	if e != nil {
		return fmt.Errorf("initialize protection: %w", e)
	}
	_, e = tea.NewProgram(ui.New(resolved, settings, g)).Run()
	return e
}
