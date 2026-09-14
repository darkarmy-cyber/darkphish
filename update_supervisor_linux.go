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
	cmd.Env = updateResultEnvironment("", "")
	if len(outcome) == 2 {
		cmd.Env = updateResultEnvironment(outcome[0], outcome[1])
	}
	cmd.Env = append(cmd.Env, childEnvironment+"=1")
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

func (c *supervisedChild) readyContext(ctx context.Context) error {
	timer := time.NewTimer(90 * time.Second)
	defer timer.Stop()
	ready := map[string]bool{}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg := <-c.messages:
			if msg.Kind == "admin-ready" || msg.Kind == "phish-ready" {
				ready[msg.Kind] = true
			}
			if len(ready) == 2 {
				return ctx.Err()
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

// recoverPendingUpdate runs before replacement-version configuration parsing.
func recoverPendingUpdate() (bool, error) {
	if os.Getenv(childEnvironment) == "1" {
		return false, nil
	}
	pending, err := pendingUpdate(".")
	if err != nil {
		return true, err
	}
	if !pending {
		return false, nil
	}
	return superviseUpdates(nil)
}

func pendingUpdate(root string) (bool, error) {
	state := filepath.Join(root, ".darkphish-updates")
	info, err := os.Lstat(state)
	if os.IsNotExist(err) || (err == nil && info.Mode().IsRegular()) {
		// An existing SQLite file with the reserved name only disables apply.
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.IsDir() {
		return false, nil
	}
	_, err = os.Lstat(filepath.Join(state, "active"))
	if os.IsNotExist(err) {
		return false, nil
	}
	if info.Mode().Perm()&0077 != 0 {
		return false, errors.New("unsafe pending update state directory")
	}
	return err == nil, err
}

// superviseUpdates keeps the systemd main PID alive across replacement. All
// child processes and their workers are reaped before snapshot or rollback.
func superviseUpdates(conf *config.Config) (bool, error) {
	if os.Getenv(childEnvironment) == "1" {
		return false, nil
	}
	stopContext, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	root, err := os.Getwd()
	if err != nil {
		return true, err
	}
	state := filepath.Join(root, ".darkphish-updates")
	active := filepath.Join(state, "active")
	pending, activeErr := pendingUpdate(root)
	if activeErr != nil {
		return true, activeErr
	}
	var l update.Layout
	if pending {
		// Local recovery must precede ALL new-update eligibility checks. The
		// executable/runtime may be partially replaced and gh may be unavailable.
		l, err = readRecoveryLayout(active, root)
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
		if recovered {
			saved = updateCompletion{Result: "rollback", Tag: "interrupted"}
		}
		if saved.Result != "" {
			if err = persistUpdateResult(state, saved); err != nil {
				return true, err
			}
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
		if stopContext.Err() != nil {
			return true, nil
		}
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
	if err = child.readyContext(stopContext); err != nil {
		_ = child.stop()
		return true, err
	}
	if _, err = cWrite(child, "resume"); err != nil {
		_ = child.stop()
		return true, err
	}
	for {
		select {
		case <-stopContext.Done():
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
			ctx, cancel := context.WithTimeout(stopContext, 3*time.Minute)
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
			if err = createActiveTransaction(state, active, l); err != nil {
				fmt.Fprintln(os.Stderr, "update preparation failed; application was not changed:", err)
				_, _ = cWrite(child, "failed")
				continue
			}
			stage := filepath.Join(active, "stage")
			if err = os.Mkdir(stage, 0700); err == nil {
				err = update.ExtractContext(stopContext, archive, stage, latest.Version(), runtime.GOARCH)
			}
			if err == nil {
				probeContext, stopProbe := context.WithTimeout(stopContext, 10*time.Second)
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
			if stopContext.Err() != nil {
				return true, nil
			}
			// No application process remains; setup only the old database/audit store.
			if err = models.Setup(conf); err != nil {
				return true, err
			}
			audit.RecordSystem("update.backup", "release", latest.Tag, "started")
			if err = models.Close(); err != nil {
				return true, err
			}
			ctx, cancel = context.WithTimeout(stopContext, 5*time.Minute)
			err = update.Backup(ctx, l, filepath.Join(active, "backup"))
			cancel()
			backedUp := err == nil
			if stopContext.Err() != nil {
				return true, nil
			}
			if err == nil {
				err = transaction.InstallContext(stopContext, stage)
			}
			if err == nil {
				child, err = startSupervised(binary)
				if err == nil {
					err = child.readyContext(stopContext)
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
				if stopContext.Err() != nil {
					return true, nil
				}
				child, err = startSupervised(binary, outcome, latest.Tag)
				if err != nil {
					return true, err
				}
				if err = child.readyContext(stopContext); err != nil {
					_ = child.stop()
					return true, err
				}
			}
			if stopped, stopErr := stopReadyUpdate(stopContext, child, transaction); stopped {
				return true, stopErr
			}
			// Outcome remains durable in the retained backup directory, without secrets.
			if err = durableUpdateResult(filepath.Join(active, "completed.json"), outcome, latest.Tag); err != nil {
				return true, errors.Join(err, child.stop())
			}
			if err = persistUpdateResult(state, updateCompletion{Result: outcome, Tag: latest.Tag}); err != nil {
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
				if stopContext.Err() != nil {
					return true, nil
				}
				if err = syscall.Exec(binary, os.Args, updateResultEnvironment("applied", latest.Tag)); err != nil {
					// Reactivate the journal before rollback so a second interruption
					// cannot leave a completed marker over a partial restoration.
					if journalErr := os.Rename(retained, active); journalErr != nil {
						return true, errors.Join(err, journalErr)
					}
					if journalErr := os.Remove(filepath.Join(active, "completed.json")); journalErr != nil {
						return true, errors.Join(err, journalErr)
					}
					if journalErr := errors.Join(syncUpdateDirectory(active), syncUpdateDirectory(state)); journalErr != nil {
						return true, errors.Join(err, journalErr)
					}
					return true, errors.Join(err, transaction.Rollback())
				}
			}
			if stopContext.Err() != nil {
				return true, child.stop()
			}
			if _, err = cWrite(child, "resume"); err != nil {
				return true, errors.Join(err, child.stop())
			}
		}
	}
}

func stopReadyUpdate(ctx context.Context, child *supervisedChild, transaction update.Transaction) (bool, error) {
	if ctx.Err() == nil {
		return false, nil
	}
	if err := child.stop(); err != nil {
		return true, err
	}
	if _, err := os.Lstat(filepath.Join(transaction.Directory, "install-started.json")); err == nil {
		return true, transaction.Rollback()
	} else if !os.IsNotExist(err) {
		return true, err
	}
	return true, nil
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
	_ = os.Unsetenv("DARKPHISH_UPDATE_RESULT")
	_ = os.Unsetenv("DARKPHISH_UPDATE_TAG")
	service := update.NewService(semanticVersion(), reason, request)
	if result == "" {
		if saved, err := readUpdateCompletion(filepath.Join(".darkphish-updates", "last-result.json")); err == nil {
			result = saved.Result
		}
	}
	service.SetResult(result)
	recordResult := func() {
		state := ".darkphish-updates"
		saved, err := readUpdateCompletion(filepath.Join(state, "last-result.json"))
		if err != nil || saved.Result == "" || saved.AuditRecorded {
			return
		}
		// Only a resumed child records completion; standby validation cannot
		// report success. The durable record also covers a crash before exec.
		if err := recordUpdateCompletion(state, saved); err != nil {
			fmt.Fprintln(os.Stderr, "update audit acknowledgement failed:", err)
		}
	}
	if feedback != nil {
		go func() {
			defer feedback.Close()
			scanner := bufio.NewScanner(feedback)
			var once sync.Once
			for scanner.Scan() {
				switch scanner.Text() {
				case "resume":
					once.Do(func() {
						recordResult()
						close(gate)
					})
				case "failed":
					service.Fail()
					audit.RecordSystem("update.apply", "release", "verification", "failure")
				}
			}
			// A child must not outlive the process that owns its update transaction.
			os.Exit(1)
		}()
	} else {
		recordResult()
	}
	return service
}

func recordUpdateCompletion(state string, saved updateCompletion) error {
	var events [][2]string
	switch saved.Result {
	case "applied":
		events = [][2]string{{"update.backup", "success"}, {"update.apply", "success"}}
	case "rollback":
		events = [][2]string{{"update.backup", "success"}, {"update.apply", "failure"}, {"update.rollback", "success"}}
	case "backup_failed":
		events = [][2]string{{"update.backup", "failure"}, {"update.apply", "failure"}}
	default:
		return errors.New("invalid update audit result")
	}
	for _, event := range events {
		if err := audit.RecordSystemChecked(event[0], "release", saved.Tag, event[1]); err != nil {
			return err
		}
	}
	saved.AuditRecorded = true
	return persistUpdateResult(state, saved)
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

func createActiveTransaction(state, active string, layout update.Layout) error {
	return createActiveTransactionWithSync(state, active, layout, syncUpdateDirectory)
}

func createActiveTransactionWithSync(state, active string, layout update.Layout, syncState func(string) error) error {
	preparing, err := os.MkdirTemp(state, "preparing-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(preparing)
	if err = writeRecoveryLayout(preparing, layout); err != nil {
		return err
	}
	// The visible recovery journal always contains a complete, synced layout.
	// A crash before this rename leaves only an ignored preparation directory.
	if err = os.Rename(preparing, active); err != nil {
		return err
	}
	if err = syncState(state); err != nil {
		// No runtime mutation has begun. Retire this visible preparation so a
		// transient sync failure does not prevent another attempt while serving.
		retireErr := os.Rename(active, preparing)
		return errors.Join(err, retireErr, syncState(state))
	}
	return nil
}

func writeRecoveryLayout(active string, layout update.Layout) error {
	data, err := json.Marshal(struct {
		Database string `json:"database"`
	}{Database: layout.Database})
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(active, "layout.json"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err = errors.Join(err, f.Sync(), f.Close()); err != nil {
		return err
	}
	return syncUpdateDirectory(active)
}

func readRecoveryLayout(active, root string) (update.Layout, error) {
	path := filepath.Join(active, "layout.json")
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 4096 {
		return update.Layout{}, errors.New("invalid local recovery layout")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return update.Layout{}, err
	}
	var layout struct {
		Database string `json:"database"`
	}
	if err = json.Unmarshal(data, &layout); err != nil {
		return update.Layout{}, err
	}
	if !filepath.IsLocal(layout.Database) || filepath.Base(layout.Database) != layout.Database {
		return update.Layout{}, errors.New("invalid recovery database path")
	}
	return recoveryLayout(&config.Config{DBName: "sqlite3", DBPath: filepath.Join(root, layout.Database)}, root)
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
	Result        string `json:"result"`
	Tag           string `json:"tag"`
	AuditRecorded bool   `json:"audit_recorded,omitempty"`
}

func completedUpdate(active string) (updateCompletion, error) {
	return readUpdateCompletion(filepath.Join(active, "completed.json"))
}

func readUpdateCompletion(path string) (updateCompletion, error) {
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
	if result.Result == "rollback" && result.Tag == "interrupted" {
		return result, nil
	}
	if !strings.HasPrefix(result.Tag, "v") {
		return updateCompletion{}, nil
	}
	if _, err := update.Compare(strings.TrimPrefix(result.Tag, "v"), strings.TrimPrefix(result.Tag, "v")); err != nil {
		return updateCompletion{}, nil
	}
	return result, nil
}

func persistUpdateResult(state string, result updateCompletion) error {
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(state, "result-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(append(data, '\n'))
	if err = errors.Join(err, f.Sync(), f.Close()); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), filepath.Join(state, "last-result.json")); err != nil {
		return err
	}
	return syncUpdateDirectory(state)
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
