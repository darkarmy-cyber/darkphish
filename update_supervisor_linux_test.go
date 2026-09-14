//go:build linux

package main

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/darkarmy-cyber/darkphish/config"
	"github.com/darkarmy-cyber/darkphish/internal/audit"
	"github.com/darkarmy-cyber/darkphish/internal/update"
	"github.com/darkarmy-cyber/darkphish/logger"
)

type failingCompletionStore struct {
	audit.Store
	err error
}

func (s *failingCompletionStore) Append(audit.Event) (int64, error) {
	return 0, s.err
}

func TestCompletionAuditRetriesFailedPersistence(t *testing.T) {
	state := t.TempDir()
	saved := updateCompletion{Result: "applied", Tag: "v0.8.0"}
	if err := persistUpdateResult(state, saved); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("audit database unavailable")
	store := &failingCompletionStore{err: failure}
	audit.SetStore(store)
	defer audit.SetStore(nil)
	if err := recordUpdateCompletion(state, saved); !errors.Is(err, failure) {
		t.Fatal("audit failure was not returned", err)
	}
	pending, err := readUpdateCompletion(filepath.Join(state, "last-result.json"))
	if err != nil || pending.AuditRecorded {
		t.Fatal("failed audit was acknowledged", err)
	}
	store.err = nil
	if err := recordUpdateCompletion(state, pending); err != nil {
		t.Fatal("audit retry failed", err)
	}
	completed, err := readUpdateCompletion(filepath.Join(state, "last-result.json"))
	if err != nil || !completed.AuditRecorded {
		t.Fatal("successful retry not acknowledged", err)
	}
}

func TestRecoveryUsesJournalBeforeConfiguration(t *testing.T) {
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
	if err := os.WriteFile(*configPath, []byte("invalid replacement configuration"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := writeRecoveryLayout(active, update.Layout{Database: "darkphish.db"}); err != nil {
		t.Fatal(err)
	}
	layout, err := readRecoveryLayout(active, root)
	if err != nil || layout.Database != "darkphish.db" || layout.Root != root {
		t.Fatal("recovery depended on parsing replacement configuration", err)
	}
	if err = os.WriteFile(filepath.Join(active, "install-started.json"), []byte(`{"started":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	if handled, err := recoverPendingUpdate(); !handled || err == nil {
		t.Fatal("missing backup must block startup before configuration loading")
	}
}

func TestReadinessCancelledBySupervisorStop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	child := &supervisedChild{messages: make(chan supervisorMessage), done: make(chan error)}
	if err := child.readyContext(ctx); err != context.Canceled {
		t.Fatal("shutdown did not cancel readiness", err)
	}
	child.messages = make(chan supervisorMessage, 2)
	child.messages <- supervisorMessage{Kind: "admin-ready"}
	child.messages <- supervisorMessage{Kind: "phish-ready"}
	if err := child.readyContext(ctx); err != context.Canceled {
		t.Fatal("ready messages overrode shutdown", err)
	}
}

func TestReservedDatabaseDoesNotPreventNormalStartup(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv(childEnvironment, "")
	if err := os.WriteFile(".darkphish-updates", []byte("SQLite file"), 0600); err != nil {
		t.Fatal(err)
	}
	if handled, err := recoverPendingUpdate(); handled || err != nil {
		t.Fatal("reserved database name prevented normal startup", err)
	}
}

func TestInactiveStatePermissionsOnlyDisableUpdates(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv(childEnvironment, "")
	state := ".darkphish-updates"
	if err := os.Mkdir(state, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(state, 0755); err != nil {
		t.Fatal(err)
	}
	if handled, err := recoverPendingUpdate(); handled || err != nil {
		t.Fatal("inactive state prevented startup", err)
	}
	if err := os.Mkdir(filepath.Join(state, "active"), 0700); err != nil {
		t.Fatal(err)
	}
	if handled, err := recoverPendingUpdate(); !handled || err == nil {
		t.Fatal("unsafe active state bypassed fail-closed recovery")
	}
}

func TestActiveTransactionPublishesCompleteLayout(t *testing.T) {
	root := t.TempDir()
	previous := *configPath
	*configPath = filepath.Join(root, "config.json")
	defer func() { *configPath = previous }()
	state := t.TempDir()
	active := filepath.Join(state, "active")
	if err := createActiveTransaction(state, active, update.Layout{Database: "original.db"}); err != nil {
		t.Fatal(err)
	}
	if err := createActiveTransaction(state, active, update.Layout{Database: "replacement.db"}); err == nil {
		t.Fatal("existing transaction was overwritten")
	}
	layout, err := readRecoveryLayout(active, root)
	if err != nil || layout.Database != "original.db" {
		t.Fatal("failed journal publication damaged recovery", err)
	}
	entries, err := os.ReadDir(state)
	if err != nil || len(entries) != 1 || entries[0].Name() != "active" {
		t.Fatal("failed preparation was not cleaned up")
	}
}

func TestFailedJournalSyncAllowsRetry(t *testing.T) {
	state := t.TempDir()
	active := filepath.Join(state, "active")
	calls := 0
	err := createActiveTransactionWithSync(state, active, update.Layout{Database: "darkphish.db"}, func(string) error {
		calls++
		if calls == 1 {
			return errors.New("temporary sync failure")
		}
		return nil
	})
	if err == nil {
		t.Fatal("sync failure was ignored")
	}
	if _, err = os.Lstat(active); !os.IsNotExist(err) {
		t.Fatal("failed preparation still occupies active", err)
	}
	if err = createActiveTransaction(state, active, update.Layout{Database: "darkphish.db"}); err != nil {
		t.Fatal("retry blocked by failed preparation", err)
	}
}

func TestPreInstallCancellationDoesNotRequireBackup(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	t.Setenv(childEnvironment, "")
	previous := *configPath
	*configPath = filepath.Join(root, "config.json")
	defer func() { *configPath = previous }()
	state := filepath.Join(root, ".darkphish-updates")
	if err := os.Mkdir(state, 0700); err != nil {
		t.Fatal(err)
	}
	active := filepath.Join(state, "active")
	if err := createActiveTransaction(state, active, update.Layout{Database: "darkphish.db"}); err != nil {
		t.Fatal(err)
	}
	// Simulate cancellation during backup: no install marker or manifest exists.
	// The intentionally absent executable makes exec return instead of replacing
	// the test process, proving recovery reached normal restart without rollback.
	if handled, err := recoverPendingUpdate(); !handled || !errors.Is(err, syscall.ENOENT) {
		t.Fatal("pre-install recovery incorrectly required a backup", err)
	}
	if _, err := os.Lstat(active); !os.IsNotExist(err) {
		t.Fatal("pre-install journal was not retired", err)
	}
	entries, err := os.ReadDir(state)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "recovered-") {
			return
		}
	}
	t.Fatal("retired pre-install journal missing")
}

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
	if completed, err := completedUpdate(active); err != nil || completed.Result != "" {
		t.Fatal("interrupted completion marker bypassed rollback")
	}
}

func TestCompletedRecoveryRetainsResultAndTag(t *testing.T) {
	for _, outcome := range []string{"applied", "rollback", "backup_failed"} {
		active := t.TempDir()
		if err := durableUpdateResult(filepath.Join(active, "completed.json"), outcome, "v0.8.0"); err != nil {
			t.Fatal(err)
		}
		completed, err := completedUpdate(active)
		if err != nil || completed.Result != outcome || completed.Tag != "v0.8.0" {
			t.Fatal("completed crash recovery lost outcome or audit tag")
		}
		state := t.TempDir()
		if err = persistUpdateResult(state, completed); err != nil {
			t.Fatal(err)
		}
		last, err := readUpdateCompletion(filepath.Join(state, "last-result.json"))
		if err != nil || last != completed {
			t.Fatal("outcome did not survive retirement of the active transaction")
		}
	}
}

func TestUpdateResultSurvivesSupervisorReload(t *testing.T) {
	t.Setenv("DARKPHISH_UPDATE_RESULT", "stale")
	t.Setenv("DARKPHISH_UPDATE_TAG", "stale")
	env := strings.Join(updateResultEnvironment("applied", "v0.8.0"), "\n")
	if strings.Contains(env, "DARKPHISH_UPDATE_RESULT=stale") || strings.Count(env, "DARKPHISH_UPDATE_RESULT=") != 1 || !strings.Contains(env, "DARKPHISH_UPDATE_RESULT=applied") || !strings.Contains(env, "DARKPHISH_UPDATE_TAG=v0.8.0") {
		t.Fatal("reload lost or duplicated update outcome")
	}
	for _, result := range []string{"", "rollback", "backup_failed"} {
		env = strings.Join(updateResultEnvironment(result, "v0.8.1"), "\n")
		if strings.Contains(env, "=stale") || strings.Count(env, "DARKPHISH_UPDATE_RESULT=") > 1 {
			t.Fatal("child inherited stale outcome")
		}
		if result != "" && !strings.Contains(env, "DARKPHISH_UPDATE_RESULT="+result) {
			t.Fatal("child lost failure outcome")
		}
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
		notifyShutdown(shutdown)
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
	// systemd's control-group stop also sends SIGTERM directly to the child.
	if err = cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	transaction := update.Transaction{Directory: t.TempDir()}
	if stopped, err := stopReadyUpdate(ctx, child, transaction); !stopped || err != nil {
		t.Fatal("stop request did not prevent commit", err)
	}
	if _, err := os.Stat(filepath.Join(transaction.Directory, "completed.json")); !os.IsNotExist(err) {
		t.Fatal("cancelled ready update was committed")
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "drained" {
		t.Fatal("supervisor bypassed graceful shutdown")
	}
}
