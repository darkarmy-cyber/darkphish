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
	_ = c.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-c.done:
		return nil
	case <-time.After(15 * time.Second):
	}
	if err := c.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	select {
	case <-c.done:
		return nil
	case <-time.After(10 * time.Second):
		return errors.New("application did not stop; refusing update")
	}
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
	if releaseVersion != semanticVersion() {
		return false, nil
	}
	l, err := updateLayout(conf)
	if err != nil {
		return false, nil
	}
	state := filepath.Join(l.Root, ".darkphish-updates")
	if err = os.MkdirAll(state, 0700); err != nil {
		// A read-only native installation remains usable with manual updates.
		// There cannot be a pending transaction in a state directory we could
		// not create; configureUpdates will disable apply without a supervisor.
		if _, stateErr := os.Lstat(state); os.IsNotExist(stateErr) {
			return false, nil
		}
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
	active := filepath.Join(state, "active")
	transaction := update.Transaction{Layout: l, Directory: active}
	recovered := false
	if _, err = os.Lstat(active); err == nil {
		if _, started := os.Stat(filepath.Join(active, "install-started.json")); started == nil {
			if _, completed := os.Stat(filepath.Join(active, "completed.json")); os.IsNotExist(completed) {
				if err = transaction.Rollback(); err != nil {
					return true, err
				}
				recovered = true
			}
		}
		if err = os.Rename(active, filepath.Join(state, "recovered-"+time.Now().UTC().Format("20060102T150405.000000000"))); err != nil {
			return true, err
		}
	}
	binary := filepath.Join(l.Root, "darkphish")
	var child *supervisedChild
	if recovered {
		child, err = startSupervised(binary, "rollback", "interrupted")
	} else {
		child, err = startSupervised(binary)
	}
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
				child, err = startSupervised(binary, "applied", latest.Tag)
				if err == nil {
					err = child.ready()
					if err != nil {
						_ = child.stop()
					}
				}
			}
			outcome := "success"
			if err != nil {
				fmt.Fprintln(os.Stderr, "update failed:", err)
				outcome = "failure"
				if _, started := os.Stat(filepath.Join(active, "install-started.json")); started == nil {
					if rollbackErr := transaction.Rollback(); rollbackErr != nil {
						return true, rollbackErr
					}
				}
				failureKind := "rollback"
				if !backedUp {
					failureKind = "backup_failed"
				}
				child, err = startSupervised(binary, failureKind, latest.Tag)
				if err != nil {
					return true, err
				}
				if err = child.ready(); err != nil {
					_ = child.stop()
					return true, err
				}
			}
			// Outcome remains durable in the retained backup directory, without secrets.
			if err = durableUpdateResult(filepath.Join(active, "completed.json"), outcome); err != nil {
				return true, errors.Join(err, child.stop())
			}
			retained := filepath.Join(state, "backup-"+time.Now().UTC().Format("20060102T150405.000000000"))
			if err = os.Rename(active, retained); err != nil {
				return true, errors.Join(err, child.stop())
			}
			if err = syncUpdateDirectory(state); err != nil {
				return true, errors.Join(err, child.stop())
			}
			if outcome == "success" {
				// Reload the supervisor too, preserving systemd's main PID. A later
				// update must never use the old version's database or backup code.
				if err = child.stop(); err != nil {
					return true, err
				}
				if err = syscall.Exec(binary, os.Args, os.Environ()); err != nil {
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
	if os.Getenv(childEnvironment) == "1" {
		tag := os.Getenv("DARKPHISH_UPDATE_TAG")
		switch os.Getenv("DARKPHISH_UPDATE_RESULT") {
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

func durableUpdateResult(path, result string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, err = f.WriteString("{\"result\":\"" + result + "\"}\n")
	if err = errors.Join(err, f.Sync(), f.Close()); err != nil {
		return err
	}
	return syncUpdateDirectory(filepath.Dir(path))
}
