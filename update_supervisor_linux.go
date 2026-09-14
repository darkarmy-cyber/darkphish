//go:build linux

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/darkarmy-cyber/darkphish/config"
	"github.com/darkarmy-cyber/darkphish/internal/audit"
	"github.com/darkarmy-cyber/darkphish/internal/update"
	"github.com/darkarmy-cyber/darkphish/models"
)

const childEnvironment = "DARKPHISH_UPDATE_CHILD"

type supervisorMessage struct {
	Kind    string `json:"kind"`
	Version string `json:"version"`
	Source  string `json:"source"`
}
type supervisedChild struct {
	cmd      *exec.Cmd
	done     chan error
	messages chan supervisorMessage
	pipe     *os.File
	feedback *os.File
}

func startSupervised(binary string, outcome ...string) (*supervisedChild, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(binary, os.Args[1:]...)
	feedbackReader, feedbackWriter, err := os.Pipe()
	if err != nil {
		r.Close()
		w.Close()
		return nil, err
	}
	cmd.Env = append(os.Environ(), childEnvironment+"=1")
	if len(outcome) == 2 {
		cmd.Env = append(cmd.Env, "DARKPHISH_UPDATE_RESULT="+outcome[0], "DARKPHISH_UPDATE_TAG="+outcome[1])
	}
	cmd.ExtraFiles = []*os.File{w, feedbackReader}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err = cmd.Start(); err != nil {
		r.Close()
		w.Close()
		feedbackReader.Close()
		feedbackWriter.Close()
		return nil, err
	}
	w.Close()
	feedbackReader.Close()
	c := &supervisedChild{cmd: cmd, done: make(chan error, 1), messages: make(chan supervisorMessage, 4), pipe: r, feedback: feedbackWriter}
	go func() { c.done <- cmd.Wait() }()
	go func() {
		defer r.Close()
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 1024), 4096)
		for scanner.Scan() {
			var msg supervisorMessage
			if json.Unmarshal(scanner.Bytes(), &msg) == nil {
				select {
				case c.messages <- msg:
				default:
				}
			}
		}
	}()
	return c, nil
}

func (c *supervisedChild) stop() error {
	defer c.feedback.Close()
	// The application's shutdown loop handles os.Interrupt and drains HTTP/IMAP.
	_ = c.cmd.Process.Signal(os.Interrupt)
	// Never kill an in-flight SMTP delivery to advance an update. Its result
	// must be persisted before a snapshot; a hung delivery keeps apply waiting.
	return <-c.done
}

func (c *supervisedChild) ready() error {
	timer := time.NewTimer(90 * time.Second)
	defer timer.Stop()
	ready := map[string]bool{}
	for {
		select {
		case msg := <-c.messages:
			if msg.Kind == "admin-ready" || msg.Kind == "phish-ready" {
				ready[msg.Kind] = true
			}
			if len(ready) == 2 {
				return nil
			}
		case err := <-c.done: // Keep termination observable by stop.
			c.done <- err
			return errors.New("replacement exited before readiness")
		case <-timer.C:
			return errors.New("replacement readiness timed out")
		}
	}
}

func updateLayout(conf *config.Config) (update.Layout, error) {
	root, err := os.Getwd()
	if err != nil {
		return update.Layout{}, err
	}
	binary, err := os.Executable()
	if err != nil {
		return update.Layout{}, err
	}
	if binary != filepath.Join(root, "darkphish") {
		return update.Layout{}, errors.New("one-click update requires a native release directory")
	}
	if err = syscall.Access(root, 2); err != nil {
		return update.Layout{}, errors.New("one-click update requires a writable native release directory")
	}
	if conf.DBName != "sqlite3" {
		return update.Layout{}, errors.New("one-click update is unsupported for MySQL/PostgreSQL; a consistent backup is not guaranteed")
	}
	if conf.Audit.MultiInstance || *mode != "all" || *disableMailer {
		return update.Layout{}, errors.New("one-click update requires a single supervised all-mode instance")
	}
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		return update.Layout{}, errors.New("one-click update supports Linux amd64/arm64 only")
	}
	if err := updateRuntimePaths(conf, root); err != nil {
		return update.Layout{}, err
	}
	if err := update.AttestationSupport(); err != nil {
		return update.Layout{}, err
	}
	configAbs, err := filepath.Abs(*configPath)
	if err != nil {
		return update.Layout{}, err
	}
	dbAbs, err := filepath.Abs(conf.DBPath)
	if err != nil {
		return update.Layout{}, err
	}
	if filepath.Dir(configAbs) != root || filepath.Dir(dbAbs) != root {
		return update.Layout{}, errors.New("one-click update requires config and SQLite beside the binary")
	}
	l := update.Layout{Root: root, Config: filepath.Base(configAbs), Database: filepath.Base(dbAbs)}
	if info, err := os.Lstat(dbAbs); err != nil || !info.Mode().IsRegular() {
		return l, errors.New("complete initial SQLite bootstrap and restart before enabling one-click updates")
	}
	for _, secret := range []string{conf.Session.AuthKeyFile, conf.Session.EncryptionKeyFile, conf.Secrets.EncryptionKeyFile, conf.Secrets.Vault.TokenFile, conf.Secrets.Vault.CACert, conf.Audit.SigningKeyFile, conf.AdminConf.CertPath, conf.AdminConf.KeyPath, conf.PhishConf.CertPath, conf.PhishConf.KeyPath} {
		if secret == "" {
			continue
		}
		p, err := filepath.Abs(secret)
		if err != nil {
			return l, err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return l, err
		}
		if filepath.IsLocal(rel) {
			if filepath.Base(rel) != rel || rel == l.Database || rel == l.Config {
				return l, errors.New("external secrets must be outside managed runtime directories")
			}
			l.Excluded = append(l.Excluded, rel)
		}
	}
	if err = l.Validate(); err != nil {
		return l, err
	}
	return l, nil
}

// superviseUpdates keeps the systemd main PID alive across replacement. All
// child processes and their workers are reaped before snapshot or rollback.
func superviseUpdates(conf *config.Config) (bool, error) {
	if os.Getenv(childEnvironment) == "1" {
		return false, nil
	}
	root, err := os.Getwd()
	if err != nil {
		return true, err
	}
	state := filepath.Join(root, ".darkphish-updates")
	active := filepath.Join(state, "active")
	_, activeErr := os.Lstat(active)
	pending := activeErr == nil
	if activeErr != nil && !os.IsNotExist(activeErr) {
		return true, activeErr
	}
	var l update.Layout
	if pending {
		// Local recovery must precede ALL new-update eligibility checks. The
		// executable/runtime may be partially replaced and gh may be unavailable.
		l, err = recoveryLayout(conf, root)
		if err != nil {
			return true, err
		}
	} else {
		if releaseVersion != semanticVersion() {
			return false, nil
		}
		l, err = updateLayout(conf)
		if err != nil {
			return false, nil
		}
	}
	if err = os.MkdirAll(state, 0700); err != nil {
		// A read-only native installation remains usable with manual updates.
		// There cannot be a pending transaction in a state directory we could
		// not create; configureUpdates will disable apply without a supervisor.
		if _, stateErr := os.Lstat(state); os.IsNotExist(stateErr) {
			return false, nil
		}
		return true, err
	}
	if err = syncUpdateDirectory(root); err != nil {
		return true, err
	}
	info, err := os.Lstat(state)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return true, errors.New("unsafe update state directory")
	}
	lock, err := os.OpenFile(filepath.Join(state, "lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return true, err
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return true, errors.New("another supervisor holds the update lock")
	}
	transaction := update.Transaction{Layout: l, Directory: active}
	recovered := false
	var saved updateCompletion
	if _, err = os.Lstat(active); err == nil {
		completed, completionErr := completedUpdate(active)
		if completionErr != nil {
			return true, completionErr
		}
		saved = completed
		if _, started := os.Lstat(filepath.Join(active, "install-started.json")); started == nil {
			if completed.Result == "" {
				if err = transaction.Rollback(); err != nil {
					return true, err
				}
				recovered = true
			}
		} else if !os.IsNotExist(started) {
			return true, started
		}
		if err = os.Rename(active, filepath.Join(state, "recovered-"+time.Now().UTC().Format("20060102T150405.000000000"))); err != nil {
			return true, err
		}
		if err = syncUpdateDirectory(state); err != nil {
			return true, err
		}
	}
	binary := filepath.Join(l.Root, "darkphish")
	if pending {
		result, tag := saved.Result, saved.Tag
		if recovered {
			result, tag = "rollback", "interrupted"
		}
		// Execute the restored supervisor even if future updates are unsupported.
		return true, syscall.Exec(binary, os.Args, updateResultEnvironment(result, tag))
	}
	child, err := startSupervised(binary)
	if err != nil {
		return true, err
	}
	if err = child.ready(); err != nil {
		_ = child.stop()
		return true, err
	}
	if _, err = cWrite(child, "resume"); err != nil {
		_ = child.stop()
		return true, err
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	for {
		select {
		case <-signals:
			return true, child.stop()
		case err := <-child.done:
			if err == nil {
				err = errors.New("supervised application stopped")
			}
			return true, err
		case request := <-child.messages:
			if request.Kind != "apply" {
				continue
			}
			comparison, compareErr := update.Compare(request.Version, semanticVersion())
			if compareErr != nil || comparison <= 0 {
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			client := update.NewClient()
			latest, checkErr := client.Latest(ctx)
			var archive []byte
			if checkErr == nil && latest.Version() == request.Version && latest.Source == request.Source {
				archive, checkErr = client.VerifiedArchive(ctx, latest, runtime.GOARCH)
			} else if checkErr == nil {
				checkErr = errors.New("requested release changed")
			}
			cancel()
			if checkErr != nil {
				fmt.Fprintln(os.Stderr, "update verification failed; application was not changed:", checkErr)
				_, _ = cWrite(child, "failed")
				continue
			}
			if err = os.Mkdir(active, 0700); err != nil {
				return true, errors.Join(err, child.stop())
			}
			if err = syncUpdateDirectory(state); err != nil {
				return true, errors.Join(err, child.stop())
			}
			stage := filepath.Join(active, "stage")
			if err = os.Mkdir(stage, 0700); err == nil {
				err = update.Extract(archive, stage, latest.Version(), runtime.GOARCH)
			}
			if err == nil {
				probeContext, stopProbe := context.WithTimeout(context.Background(), 10*time.Second)
				output, probeErr := exec.CommandContext(probeContext, filepath.Join(stage, "darkphish"), "version").Output()
				stopProbe()
				if probeErr != nil || !strings.Contains(string(output), "version "+latest.Version()+",") || !strings.Contains(string(output), "commit "+latest.Source+",") {
					err = errors.New("native binary build identity does not match release evidence")
				}
			}
			if err != nil {
				fmt.Fprintln(os.Stderr, "update archive rejected:", err)
				_, _ = cWrite(child, "failed")
				_ = os.Rename(active, filepath.Join(state, "rejected-"+time.Now().UTC().Format("20060102T150405.000000000")))
				continue
			}
			if err = child.stop(); err != nil {
				return true, err
			}
			// No application process remains; setup only the old database/audit store.
			if err = models.Setup(conf); err != nil {
				return true, err
			}
			audit.RecordSystem("update.backup", "release", latest.Tag, "started")
			if err = models.Close(); err != nil {
				return true, err
			}
			ctx, cancel = context.WithTimeout(context.Background(), 5*time.Minute)
			err = update.Backup(ctx, l, filepath.Join(active, "backup"))
			cancel()
			backedUp := err == nil
			if err == nil {
				err = transaction.Install(stage)
			}
			if err == nil {
				child, err = startSupervised(binary)
				if err == nil {
					err = child.ready()
					if err != nil {
						_ = child.stop()
					}
				}
			}
			outcome := "applied"
			if err != nil {
				fmt.Fprintln(os.Stderr, "update failed:", err)
				outcome = "rollback"
				if _, started := os.Stat(filepath.Join(active, "install-started.json")); started == nil {
					if rollbackErr := transaction.Rollback(); rollbackErr != nil {
						return true, rollbackErr
					}
				}
				if !backedUp {
					outcome = "backup_failed"
				}
				child, err = startSupervised(binary, outcome, latest.Tag)
				if err != nil {
					return true, err
				}
				if err = child.ready(); err != nil {
					_ = child.stop()
					return true, err
				}
			}
			// Outcome remains durable in the retained backup directory, without secrets.
			if err = durableUpdateResult(filepath.Join(active, "completed.json"), outcome, latest.Tag); err != nil {
				return true, errors.Join(err, child.stop())
			}
			retained := filepath.Join(state, "backup-"+time.Now().UTC().Format("20060102T150405.000000000"))
			if err = os.Rename(active, retained); err != nil {
				return true, errors.Join(err, child.stop())
			}
			if err = syncUpdateDirectory(state); err != nil {
				return true, errors.Join(err, child.stop())
			}
			if outcome == "applied" {
				// Reload the supervisor too, preserving systemd's main PID. A later
				// update must never use the old version's database or backup code.
				if err = child.stop(); err != nil {
					return true, err
				}
				if err = syscall.Exec(binary, os.Args, updateResultEnvironment("applied", latest.Tag)); err != nil {
					transaction.Directory = retained
					return true, errors.Join(err, transaction.Rollback())
				}
			}
			if _, err = cWrite(child, "resume"); err != nil {
				return true, errors.Join(err, child.stop())
			}
		}
	}
}

func configureUpdates(conf *config.Config) *update.Service {
	_, err := updateLayout(conf)
	reason := ""
	if err != nil {
		reason = err.Error()
	}
	if releaseVersion != semanticVersion() {
		reason = "One-click update requires an official stable native build"
	}
	if os.Getenv(childEnvironment) != "1" && reason == "" {
		reason = "One-click update requires the supervised native process"
	}
	var request func(update.Release) error
	var feedback *os.File
	gate := make(chan struct{})
	if os.Getenv(childEnvironment) == "1" {
		pipe := os.NewFile(3, "update-supervisor")
		info, err := pipe.Stat()
		if err != nil || info.Mode()&os.ModeNamedPipe == 0 {
			reason = "Update supervisor pipe is unavailable"
		} else {
			feedback = os.NewFile(4, "update-supervisor-feedback")
			encoder := json.NewEncoder(pipe)
			var mu sync.Mutex
			update.ListenerReady = func(name string) {
				mu.Lock()
				_ = encoder.Encode(supervisorMessage{Kind: name + "-ready"})
				mu.Unlock()
				<-gate
			}
			update.WaitServing = func() { <-gate }
			request = func(r update.Release) error {
				mu.Lock()
				defer mu.Unlock()
				return encoder.Encode(supervisorMessage{Kind: "apply", Version: r.Version(), Source: r.Source})
			}
		}
	}
	result := os.Getenv("DARKPHISH_UPDATE_RESULT")
	if result != "" {
		tag := os.Getenv("DARKPHISH_UPDATE_TAG")
		switch result {
		case "applied":
			audit.RecordSystem("update.backup", "release", tag, "success")
			audit.RecordSystem("update.apply", "release", tag, "success")
		case "rollback":
			audit.RecordSystem("update.apply", "release", tag, "failure")
			audit.RecordSystem("update.rollback", "release", tag, "success")
		case "backup_failed":
			audit.RecordSystem("update.backup", "release", tag, "failure")
			audit.RecordSystem("update.apply", "release", tag, "failure")
		}
		_ = os.Unsetenv("DARKPHISH_UPDATE_RESULT")
		_ = os.Unsetenv("DARKPHISH_UPDATE_TAG")
	}
	service := update.NewService(semanticVersion(), reason, request)
	service.SetResult(result)
	if feedback != nil {
		go func() {
			defer feedback.Close()
			scanner := bufio.NewScanner(feedback)
			var once sync.Once
			for scanner.Scan() {
				switch scanner.Text() {
				case "resume":
					once.Do(func() { close(gate) })
				case "failed":
					service.Fail()
					audit.RecordSystem("update.apply", "release", "verification", "failure")
				}
			}
			// A child must not outlive the process that owns its update transaction.
			os.Exit(1)
		}()
	}
	return service
}

func updateRuntimePaths(conf *config.Config, root string) error {
	if conf.Logging != nil && conf.Logging.Filename != "" {
		return errors.New("one-click update does not support custom file logging; use standard output or update manually")
	}
	migrations, err := filepath.Abs(conf.MigrationsPath)
	if err != nil || migrations != filepath.Join(root, "db", "db_sqlite3", "migrations") {
		return errors.New("one-click update requires the bundled SQLite migration directory")
	}
	return nil
}

func recoveryLayout(conf *config.Config, root string) (update.Layout, error) {
	configAbs, err := filepath.Abs(*configPath)
	if err != nil {
		return update.Layout{}, err
	}
	dbAbs, err := filepath.Abs(conf.DBPath)
	if err != nil || conf.DBName != "sqlite3" || filepath.Dir(configAbs) != root || filepath.Base(configAbs) != "config.json" || filepath.Dir(dbAbs) != root || filepath.Base(dbAbs) == "config.json" {
		return update.Layout{}, errors.New("pending update requires its original local SQLite/config layout; refusing normal startup")
	}
	for _, entry := range []string{"darkphish", "VERSION", "LICENSE", "NOTICE.md", "README.md", "CHANGELOG.md", "db", "templates", "static", ".darkphish-updates"} {
		if filepath.Base(dbAbs) == entry {
			return update.Layout{}, errors.New("invalid recovery database path")
		}
	}
	return update.Layout{Root: root, Config: "config.json", Database: filepath.Base(dbAbs)}, nil
}

func updateResultEnvironment(result, tag string) []string {
	var env []string
	for _, value := range os.Environ() {
		if strings.HasPrefix(value, "DARKPHISH_UPDATE_RESULT=") || strings.HasPrefix(value, "DARKPHISH_UPDATE_TAG=") {
			continue
		}
		env = append(env, value)
	}
	if result != "" {
		env = append(env, "DARKPHISH_UPDATE_RESULT="+result, "DARKPHISH_UPDATE_TAG="+tag)
	}
	return env
}

type updateCompletion struct {
	Result string `json:"result"`
	Tag    string `json:"tag"`
}

func completedUpdate(active string) (updateCompletion, error) {
	path := filepath.Join(active, "completed.json")
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return updateCompletion{}, nil
	}
	if err != nil {
		return updateCompletion{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > 4096 {
		return updateCompletion{}, errors.New("invalid update completion marker")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return updateCompletion{}, err
	}
	var result updateCompletion
	// An interrupted marker write is not a committed update; restore the backup.
	if json.Unmarshal(data, &result) != nil {
		return updateCompletion{}, nil
	}
	if result.Result != "applied" && result.Result != "rollback" && result.Result != "backup_failed" {
		return updateCompletion{}, nil
	}
	if !strings.HasPrefix(result.Tag, "v") {
		return updateCompletion{}, nil
	}
	if _, err := update.Compare(strings.TrimPrefix(result.Tag, "v"), strings.TrimPrefix(result.Tag, "v")); err != nil {
		return updateCompletion{}, nil
	}
	return result, nil
}

func cWrite(c *supervisedChild, message string) (int, error) {
	return c.feedback.WriteString(message + "\n")
}
func syncUpdateDirectory(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	return errors.Join(f.Sync(), f.Close())
}

func durableUpdateResult(path, result, tag string) error {
	data, err := json.Marshal(updateCompletion{Result: result, Tag: tag})
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(append(data, '\n'))
	if err = errors.Join(err, f.Sync(), f.Close()); err != nil {
		return err
	}
	return syncUpdateDirectory(filepath.Dir(path))
}
