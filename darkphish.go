package main

/*
darkphish - Open-Source Phishing Framework

The MIT License (MIT)

Copyright (c) 2013 Jordan Wright

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.
*/
import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/alecthomas/kingpin/v2"

	"github.com/darkarmy-cyber/darkphish/config"
	"github.com/darkarmy-cyber/darkphish/controllers"
	"github.com/darkarmy-cyber/darkphish/dialer"
	"github.com/darkarmy-cyber/darkphish/imap"
	"github.com/darkarmy-cyber/darkphish/internal/audit"
	"github.com/darkarmy-cyber/darkphish/internal/migrationcheck"
	log "github.com/darkarmy-cyber/darkphish/logger"
	"github.com/darkarmy-cyber/darkphish/middleware"
	"github.com/darkarmy-cyber/darkphish/models"
	"github.com/darkarmy-cyber/darkphish/webhook"
)

const (
	modeAll   string = "all"
	modeAdmin string = "admin"
	modePhish string = "phish"
)

var (
	configPath    = kingpin.Flag("config", "Location of config.json.").Default("./config.json").String()
	disableMailer = kingpin.Flag("disable-mailer", "Disable the mailer (for use with multi-system deployments)").Bool()
	mode          = kingpin.Flag("mode", fmt.Sprintf("Run the binary in one of the modes (%s, %s or %s)", modeAll, modeAdmin, modePhish)).
			Default("all").Enum(modeAll, modeAdmin, modePhish)
	versionCommand           = kingpin.Command("version", "Print Darkphish version and build information.")
	auditCommand             = kingpin.Command("audit", "Verify and export the tamper-evident audit trail.")
	auditVerifyCommand       = auditCommand.Command("verify", "Verify the database audit chain and signed checkpoints.")
	auditExportCommand       = auditCommand.Command("export", "Write an audit JSON export and signed companion manifest.")
	auditExportPath          = auditExportCommand.Flag("output", "Audit JSON output path.").Default("darkphish-audit.json").String()
	auditVerifyExportCommand = auditCommand.Command("verify-export", "Verify an audit export and signed manifest.")
	auditVerifyExportFile    = auditVerifyExportCommand.Arg("export", "Audit JSON export path.").Required().String()
	auditVerifyManifestFile  = auditVerifyExportCommand.Arg("manifest", "Signed manifest path.").Required().String()
	secretsCommand           = kingpin.Command("secrets", "Inspect and migrate encrypted application secrets.")
	secretsStatusCommand     = secretsCommand.Command("status", "Report secret envelope providers and migration status without plaintext.")
	secretsMigrateCommand    = secretsCommand.Command("migrate", "Re-encrypt secrets with the configured active provider/key.")
	migrateCommand           = kingpin.Command("migrate", "Inspect compatibility and migration readiness.")
	migrateCheckCommand      = migrateCommand.Command("check", "Report legacy configuration and migration hazards without changing files.")
	rotateSecrets            = kingpin.Flag("rotate-secrets", "Re-encrypt stored secrets with the active key and exit.").Bool()
	commitSHA                = "unknown"
	builtAt                  = "unknown"
	releaseVersion           = ""
)

// versionFile is the single authoritative release version. Release builds set
// releaseVersion, commitSHA, and builtAt through -ldflags.
//
//go:embed VERSION
var versionFile string

func semanticVersion() string { return strings.TrimSpace(versionFile) }

func displayVersion() string {
	parts := strings.Split(semanticVersion(), ".")
	if len(parts) < 2 {
		return semanticVersion() + "-dev"
	}
	value := strings.Join(parts[:2], ".")
	if strings.TrimSpace(releaseVersion) != semanticVersion() {
		value += "-dev"
	}
	return value
}

func versionSummary() string {
	return fmt.Sprintf("Darkphish %s (version %s, commit %s, built %s)", displayVersion(), semanticVersion(), commitSHA, builtAt)
}

func writeExclusive(path string, value []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.Remove(path)
		}
	}()
	if _, err = file.Write(value); err != nil {
		_ = file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	complete = true
	return nil
}

func main() {
	kingpin.Version(versionSummary())

	// Parse the CLI flags and load the config
	kingpin.CommandLine.HelpFlag.Short('h')
	command := kingpin.Parse()
	if command == versionCommand.FullCommand() {
		fmt.Println(versionSummary())
		return
	}

	// Load the config
	conf, err := config.LoadConfig(*configPath)
	// Just warn if a contact address hasn't been configured
	if err != nil {
		log.Fatal(err)
	}
	if conf.ContactAddress == "" {
		log.Warnf("No contact address has been configured.")
		log.Warnf("Please consider adding a contact_address entry in your config.json")
	}
	config.Version = displayVersion()
	if err = middleware.ConfigureSession(
		conf.Session.AuthKey,
		conf.Session.EncryptionKey,
		conf.AdminConf.UseTLS,
		conf.Session.LifetimeHours,
		conf.ProductionMode,
	); err != nil {
		log.Fatal(err)
	}

	// Configure our various upstream clients to make sure that we restrict
	// outbound connections as needed.
	if err = dialer.SetAllowedHosts(conf.AdminConf.AllowedInternalHosts); err != nil {
		log.Fatal(err)
	}
	webhook.SetTransport(&http.Transport{
		DialContext: dialer.Dialer().DialContext,
	})

	err = log.Setup(conf.Logging)
	if err != nil {
		log.Fatal(err)
	}
	if command == migrateCheckCommand.FullCommand() {
		legacyAPIKeys, plaintextSecrets, inspectErr := models.InspectLegacyDatabase(conf)
		if inspectErr != nil {
			log.Fatal(inspectErr)
		}
		findings := migrationcheck.Inspect(*configPath, conf, legacyAPIKeys, plaintextSecrets)
		if len(findings) == 0 {
			fmt.Println("No known Darkphish compatibility hazards detected.")
			return
		}
		for _, finding := range findings {
			fmt.Printf("[%s] %s: %s\n", finding.Severity, finding.Code, finding.Recommendation)
		}
		return
	}

	// Provide the option to disable the built-in mailer
	// Setup the global variables and settings
	err = models.Setup(conf)
	if err != nil {
		log.Fatal(err)
	}
	switch command {
	case auditVerifyCommand.FullCommand():
		report, verifyErr := models.VerifyAuditChain()
		if verifyErr != nil {
			log.Fatal(verifyErr)
		}
		fmt.Printf("Verified %d audit events (%d through %d); final hash %s; latest checkpoint %d.\n", report.RecordCount, report.FirstEventID, report.LastEventID, report.FinalHash, report.CheckpointID)
		return
	case auditExportCommand.FullCommand():
		content, manifest, exportErr := models.BuildAuditExport(semanticVersion(), commitSHA)
		if exportErr != nil {
			log.Fatal(exportErr)
		}
		manifestContent, marshalErr := json.MarshalIndent(manifest, "", "  ")
		if marshalErr != nil {
			log.Fatal(marshalErr)
		}
		manifestContent = append(manifestContent, '\n')
		manifestPath := strings.TrimSuffix(*auditExportPath, filepath.Ext(*auditExportPath)) + ".manifest.json"
		if writeErr := writeExclusive(*auditExportPath, content); writeErr != nil {
			log.Fatal(writeErr)
		}
		if writeErr := writeExclusive(manifestPath, manifestContent); writeErr != nil {
			_ = os.Remove(*auditExportPath)
			log.Fatal(writeErr)
		}
		fmt.Printf("Wrote %d signed audit records to %s and %s.\n", manifest.RecordCount, *auditExportPath, manifestPath)
		return
	case auditVerifyExportCommand.FullCommand():
		content, readErr := os.ReadFile(*auditVerifyExportFile)
		if readErr != nil {
			log.Fatal(readErr)
		}
		manifestContent, readErr := os.ReadFile(*auditVerifyManifestFile)
		if readErr != nil {
			log.Fatal(readErr)
		}
		manifest, verifyErr := models.VerifyAuditExport(content, manifestContent)
		if verifyErr != nil {
			log.Fatal(verifyErr)
		}
		fmt.Printf("Verified audit export with %d records (%d through %d), signed by %s.\n", manifest.RecordCount, manifest.FirstEventID, manifest.LastEventID, manifest.KeyID)
		return
	case secretsStatusCommand.FullCommand():
		status, statusErr := models.SecretsStatus()
		if statusErr != nil {
			log.Fatal(statusErr)
		}
		encoded, _ := json.MarshalIndent(status, "", "  ")
		fmt.Println(string(encoded))
		return
	case secretsMigrateCommand.FullCommand():
		rotated, rotateErr := models.RotateSecrets()
		if rotateErr != nil {
			log.Fatal(rotateErr)
		}
		audit.RecordSystem("secret.migrate", "secrets", "active-provider", "success")
		fmt.Printf("Migrated %d protected values to the active provider/key. The operation is idempotent and may be resumed.\n", rotated)
		return
	}
	if _, _, err = models.CleanupSecurityRetention(time.Now().UTC()); err != nil {
		log.Errorf("initial security retention cleanup failed: %v", err)
	}
	if *rotateSecrets {
		rotated, rotateErr := models.RotateSecrets()
		if rotateErr != nil {
			log.Fatal(rotateErr)
		}
		audit.RecordSystem("secret.rotate", "secrets", "active-key", "success")
		fmt.Printf("Re-encrypted %d protected values with the active key.\n", rotated)
		return
	}

	// Unlock any maillogs that may have been locked for processing
	// when Darkphish was last shutdown.
	err = models.UnlockAllMailLogs()
	if err != nil {
		log.Fatal(err)
	}

	// Create our servers
	adminOptions := []controllers.AdminServerOption{}
	if *disableMailer {
		adminOptions = append(adminOptions, controllers.WithWorker(nil))
	}
	adminConfig := conf.AdminConf
	adminServer := controllers.NewAdminServer(adminConfig, adminOptions...)

	phishConfig := conf.PhishConf
	phishServer := controllers.NewPhishingServer(phishConfig)

	imapMonitor := imap.NewMonitor()
	if *mode == "admin" || *mode == "all" {
		go adminServer.Start()
		go imapMonitor.Start()
	}
	if *mode == "phish" || *mode == "all" {
		go phishServer.Start()
	}

	// Handle graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c
	log.Info("CTRL+C Received... Gracefully shutting down servers")
	if *mode == modeAdmin || *mode == modeAll {
		adminServer.Shutdown()
		imapMonitor.Shutdown()
	}
	if *mode == modePhish || *mode == modeAll {
		phishServer.Shutdown()
	}

}
