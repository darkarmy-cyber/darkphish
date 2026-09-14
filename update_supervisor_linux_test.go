//go:build linux

package main

import (
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"testing"
	"time"

	"github.com/darkarmy-cyber/darkphish/config"
	"github.com/darkarmy-cyber/darkphish/logger"
)

func TestUpdateRuntimePaths(t *testing.T) {
	root := t.TempDir()
	conf := &config.Config{MigrationsPath: filepath.Join(root, "db", "db_sqlite3", "migrations")}
	if err := updateRuntimePaths(conf, root); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(root, "db", "darkphish.log"), filepath.Join(t.TempDir(), "external.log")} {
		conf.Logging = &logger.Config{Filename: path}
		if err := updateRuntimePaths(conf, root); err == nil {
			t.Fatal("custom file logger accepted")
		}
	}
	conf.Logging = nil
	conf.MigrationsPath = filepath.Join(t.TempDir(), "migrations")
	if err := updateRuntimePaths(conf, root); err == nil {
		t.Fatal("external migrations accepted")
	}
}

func TestSupervisorSignalsGracefulShutdown(t *testing.T) {
	if marker := os.Getenv("DARKPHISH_TEST_SHUTDOWN_MARKER"); marker != "" {
		shutdown := make(chan os.Signal, 1)
		signal.Notify(shutdown, os.Interrupt)
		_, _ = os.Stdout.WriteString("ready")
		<-shutdown
		if os.WriteFile(marker, []byte("drained"), 0600) != nil {
			os.Exit(2)
		}
		os.Exit(0)
	}
	marker := filepath.Join(t.TempDir(), "drained")
	cmd := exec.Command(os.Args[0], "-test.run=^TestSupervisorSignalsGracefulShutdown$")
	cmd.Env = append(os.Environ(), "DARKPHISH_TEST_SHUTDOWN_MARKER="+marker)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	feedback, err := os.CreateTemp(t.TempDir(), "feedback")
	if err != nil {
		t.Fatal(err)
	}
	defer feedback.Close()
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill() }()
	child := &supervisedChild{cmd: cmd, done: make(chan error, 1), feedback: feedback}
	go func() { child.done <- cmd.Wait() }()
	ready := make(chan error, 1)
	go func() {
		_, err := io.ReadFull(out, make([]byte, 5))
		ready <- err
	}()
	select {
	case err = <-ready:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("shutdown fixture did not start")
	}
	if err = child.stop(); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "drained" {
		t.Fatal("supervisor bypassed graceful shutdown")
	}
}
