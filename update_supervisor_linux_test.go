//go:build linux

package main

import (
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darkarmy-cyber/darkphish/config"
	"github.com/darkarmy-cyber/darkphish/logger"
)

func TestRecoveryLayoutDoesNotRequireNewUpdateEligibility(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	previous := *configPath
	*configPath = filepath.Join(root, "config.json")
	defer func() { *configPath = previous }()
	conf := &config.Config{DBName: "sqlite3", DBPath: filepath.Join(root, "darkphish.db"), MigrationsPath: "/unavailable", Logging: &logger.Config{Filename: "custom.log"}}
	if _, err = recoveryLayout(conf, root); err != nil {
		t.Fatal("recovery was coupled to new-update eligibility:", err)
	}
	if updateRuntimePaths(conf, root) == nil {
		t.Fatal("fixture must be ineligible for a new update")
	}
}

func TestPendingRecoveryFailsClosedBeforeEligibility(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	t.Setenv(childEnvironment, "")
	previous := *configPath
	*configPath = filepath.Join(root, "config.json")
	defer func() { *configPath = previous }()
	active := filepath.Join(root, ".darkphish-updates", "active")
	if err := os.MkdirAll(active, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(active, "install-started.json"), []byte(`{"started":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	conf := &config.Config{DBName: "sqlite3", DBPath: filepath.Join(root, "darkphish.db"), MigrationsPath: "/unavailable"}
	// This is not a release layout and has no backup. Recovery must fail closed,
	// not fall through to normal startup because update eligibility fails.
	if handled, err := superviseUpdates(conf); !handled || err == nil {
		t.Fatal("pending transaction bypassed recovery")
	}
	if err := os.WriteFile(filepath.Join(active, "completed.json"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if completed, err := completedUpdate(active); err != nil || completed {
		t.Fatal("interrupted completion marker bypassed rollback")
	}
}

func TestUpdateResultSurvivesSupervisorReload(t *testing.T) {
	t.Setenv("DARKPHISH_UPDATE_RESULT", "stale")
	t.Setenv("DARKPHISH_UPDATE_TAG", "stale")
	env := strings.Join(updateResultEnvironment("applied", "v0.8.0"), "\n")
	if strings.Contains(env, "DARKPHISH_UPDATE_RESULT=stale") || strings.Count(env, "DARKPHISH_UPDATE_RESULT=") != 1 || !strings.Contains(env, "DARKPHISH_UPDATE_RESULT=applied") || !strings.Contains(env, "DARKPHISH_UPDATE_TAG=v0.8.0") {
		t.Fatal("reload lost or duplicated update outcome")
	}
}

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
