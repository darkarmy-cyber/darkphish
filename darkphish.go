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
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/alecthomas/kingpin/v2"

	"github.com/darkarmy-cyber/darkphish/config"
	"github.com/darkarmy-cyber/darkphish/controllers"
	"github.com/darkarmy-cyber/darkphish/dialer"
	"github.com/darkarmy-cyber/darkphish/imap"
	"github.com/darkarmy-cyber/darkphish/internal/audit"
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
	versionCommand = kingpin.Command("version", "Print Darkphish version and build information.")
	rotateSecrets  = kingpin.Flag("rotate-secrets", "Re-encrypt stored secrets with the active key and exit.").Bool()
	commitSHA      = "unknown"
	builtAt        = "unknown"
	releaseVersion = ""
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

	// Provide the option to disable the built-in mailer
	// Setup the global variables and settings
	err = models.Setup(conf)
	if err != nil {
		log.Fatal(err)
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
