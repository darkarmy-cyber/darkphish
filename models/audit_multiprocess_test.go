package models

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/darkarmy-cyber/darkphish/config"
	"github.com/darkarmy-cyber/darkphish/internal/audit"
	"github.com/darkarmy-cyber/darkphish/internal/persistence"
)

func auditProcessConfig(t *testing.T) *config.Config {
	t.Helper()
	backend := os.Getenv("DARKPHISH_AUDIT_TEST_BACKEND")
	if backend != "mysql" && backend != "postgres" {
		t.Fatal("network audit backend required")
	}
	return &config.Config{
		DBName: backend, DBPath: os.Getenv("DARKPHISH_AUDIT_TEST_DSN"),
		MigrationsPath: "../db/db_" + backend + "/migrations",
		Audit:          config.AuditConfig{MultiInstance: true, ActiveSigningKeyID: "test-shared", SigningKeys: map[string]string{"test-shared": base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{73}, 32))}, CheckpointInterval: 7},
	}
}

// This helper runs in a separate OS process, with its own globals, connection
// pool and signer. It deliberately does not invoke schema migrations.
func TestAuditProcessHelper(t *testing.T) {
	mode := os.Getenv("DARKPHISH_AUDIT_CHILD")
	if mode == "" {
		t.Skip("subprocess helper")
	}
	conf = auditProcessConfig(t)
	var err error
	db, err = persistence.Open(conf)
	if err != nil {
		t.Fatal(err)
	}
	defer Close()
	if err := configureAuditStore(); err != nil {
		t.Fatal(err)
	}
	directory := os.Getenv("DARKPHISH_AUDIT_BARRIER")
	worker, err := strconv.Atoi(mode)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "ready-"+mode), []byte("ready"), 0600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(directory, "go")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("barrier timeout")
		}
		time.Sleep(10 * time.Millisecond)
	}
	for i := 0; i < 30; i++ {
		if worker < 4 {
			for _, delivery := range []int64{int64(100000 + worker*100 + i), int64(900000 + i)} {
				if _, err := (databaseAuditStore{}).Append(audit.Event{OutboxID: delivery, Timestamp: time.Date(2026, 1, 1, 0, 0, 0, 123456789, time.UTC), Actor: "audit-process-test", ActorType: "system", Action: "audit.concurrent", Result: "success", Metadata: "{}"}); err != nil {
					t.Fatal(err)
				}
			}
		} else {
			if _, err := VerifyAuditChain(); err != nil {
				t.Fatal(err)
			}
			if _, err := CreateAuditCheckpoint(); err != nil {
				t.Fatal(err)
			}
			content, manifest, err := BuildAuditExport("0.6.0", "process-test")
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := VerifyAuditExport(content, encoded); err != nil {
				t.Fatal(err)
			}
			if _, err := (databaseAuditStore{}).DeleteBefore(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestAuditMultiProcess(t *testing.T) {
	if os.Getenv("DARKPHISH_AUDIT_TEST_DSN") == "" {
		t.Skip("dedicated disposable network database required")
	}
	c := auditProcessConfig(t)
	t.Setenv(InitialAdminPassword, "test-only-audit-process-bootstrap-7391!")
	if err := Setup(c); err != nil {
		t.Fatal(err)
	}
	defer Close()
	var count int64
	if err := db.Model(&auditEventRow{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("requires empty audit database: count=%d err=%v", count, err)
	}
	for i := 0; i < 10; i++ {
		if _, err := (databaseAuditStore{}).Append(audit.Event{Timestamp: time.Date(1999, 1, 1, 0, 0, i, 0, time.UTC), Actor: "retention-fixture", Action: "audit.old", Metadata: "{}"}); err != nil {
			t.Fatal(err)
		}
	}
	directory := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	commands := make([]*exec.Cmd, 6)
	outputs := make([]bytes.Buffer, 6)
	for i := range commands {
		commands[i] = exec.CommandContext(ctx, os.Args[0], "-test.run=^TestAuditProcessHelper$", "-test.v")
		commands[i].Env = append(os.Environ(), "DARKPHISH_AUDIT_CHILD="+strconv.Itoa(i), "DARKPHISH_AUDIT_BARRIER="+directory)
		commands[i].Stdout, commands[i].Stderr = &outputs[i], &outputs[i]
		if err := commands[i].Start(); err != nil {
			t.Fatal(err)
		}
	}
	deadline := time.Now().Add(40 * time.Second)
	for {
		ready := 0
		for i := range commands {
			if _, err := os.Stat(filepath.Join(directory, fmt.Sprintf("ready-%d", i))); err == nil {
				ready++
			}
		}
		if ready == len(commands) {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			for i, cmd := range commands {
				_ = cmd.Wait()
				t.Logf("process %d: %s", i, outputs[i].String())
			}
			t.Fatal("workers failed to reach barrier")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := os.WriteFile(filepath.Join(directory, "go"), []byte("go"), 0600); err != nil {
		t.Fatal(err)
	}
	for i, cmd := range commands {
		if err := cmd.Wait(); err != nil {
			t.Errorf("process %d: %v\n%s", i, err, outputs[i].String())
		}
	}
	if t.Failed() {
		return
	}
	report, err := VerifyAuditChain()
	if err != nil || report.RecordCount != 150 {
		t.Fatalf("chain count=%d err=%v", report.RecordCount, err)
	}
	var head auditChainHead
	if err := db.First(&head).Error; err != nil {
		t.Fatal(err)
	}
	if head.Sequence != 160 || head.RetiredSequence != 10 {
		t.Fatalf("head %+v", head)
	}
	if err := initializeAuditChain(); err != nil {
		t.Fatal(err)
	}
	t.Logf("PASS %s: six independent OS processes, four writers, two verify/export/checkpoint/retention workers; 240 delivery attempts, 150 unique events, final sequence 160, retained anchor 10, valid hashes and signatures", c.DBName)
}
