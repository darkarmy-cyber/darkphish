package models

import (
	"bufio"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/gophish/gomail"
)

func TestSMTPGreetingHasDeadline(t *testing.T) {
	client, peer := net.Pipe()
	defer peer.Close()
	d := gomail.NewDialer("localhost", 25, "", "")
	_, err := newBoundedSMTPSender(client, d, 30*time.Millisecond)
	if timeout, ok := err.(net.Error); !ok || !timeout.Timeout() {
		t.Fatal("silent SMTP greeting did not time out", err)
	}
}

func TestSMTPDataAcknowledgementHasDeadline(t *testing.T) {
	client, peer := net.Pipe()
	defer peer.Close()
	release := make(chan struct{})
	defer close(release)
	go func() {
		_, _ = io.WriteString(peer, "220 localhost SMTP\r\n")
		reader := bufio.NewReader(peer)
		data := false
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			if data {
				if line == ".\r\n" {
					<-release
					return
				}
				continue
			}
			if strings.HasPrefix(line, "DATA") {
				data = true
				_, _ = io.WriteString(peer, "354 send data\r\n")
			} else {
				_, _ = io.WriteString(peer, "250 localhost\r\n")
			}
		}
	}()
	d := gomail.NewDialer("localhost", 25, "", "")
	sender, err := newBoundedSMTPSender(client, d, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	sender.timeout = 30 * time.Millisecond
	err = sender.Send("from@example.org", []string{"to@example.org"}, strings.NewReader("Subject: test\r\n\r\nbody\r\n"))
	if timeout, ok := err.(net.Error); !ok || !timeout.Timeout() {
		t.Fatal("silent SMTP DATA acknowledgement did not time out", err)
	}
}
