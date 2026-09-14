package imap

import (
	"bufio"
	"context"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/darkarmy-cyber/darkphish/dialer"
)

func TestIMAPDeadlineCannotBeCleared(t *testing.T) {
	client, peer := net.Pipe()
	defer peer.Close()
	conn, err := boundIMAPConnection(context.Background(), client, 30*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err = conn.SetDeadline(time.Time{}); err != nil {
		t.Fatal(err)
	}
	_, err = conn.Read(make([]byte, 1))
	if timeout, ok := err.(net.Error); !ok || !timeout.Timeout() {
		t.Fatal("IMAP library cleared session deadline", err)
	}
}

func TestIMAPCancellationClosesBlockedRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, peer := net.Pipe()
	defer peer.Close()
	conn, err := boundIMAPConnection(ctx, client, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	done := make(chan error, 1)
	go func() { _, err := conn.Read(make([]byte, 1)); done <- err }()
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled read succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown did not interrupt IMAP")
	}
}

func TestFailedFetchLeavesMessagesUnread(t *testing.T) {
	allowed := dialer.DefaultDialer.AllowedHosts()
	if err := dialer.SetAllowedHosts([]string{"127.0.0.1"}); err != nil {
		t.Fatal(err)
	}
	defer dialer.SetAllowedHosts(allowed)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	commands := make(chan string, 16)
	go func() {
		defer close(commands)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
		_, _ = io.WriteString(conn, "* OK [CAPABILITY IMAP4rev1] ready\r\n")
		reader := bufio.NewReader(conn)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			commands <- line
			tag := strings.Fields(line)[0]
			switch {
			case strings.Contains(line, "SELECT"):
				_, _ = io.WriteString(conn, "* 1 EXISTS\r\n* OK [UIDVALIDITY 1] stable\r\n"+tag+" OK selected\r\n")
			case strings.Contains(line, "UID SEARCH"):
				_, _ = io.WriteString(conn, "* SEARCH 42\r\n"+tag+" OK searched\r\n")
			case strings.Contains(line, "UID FETCH"):
				_, _ = io.WriteString(conn, tag+" NO interrupted fetch\r\n")
			case strings.Contains(line, "LOGOUT"):
				_, _ = io.WriteString(conn, "* BYE closing\r\n"+tag+" OK logout\r\n")
				return
			default:
				_, _ = io.WriteString(conn, tag+" OK done\r\n")
			}
		}
	}()
	mailbox := &Mailbox{Host: listener.Addr().String(), User: "user", Pwd: "password", Folder: "INBOX"}
	if messages, err := mailbox.GetUnread(true, false); err == nil || len(messages) != 0 {
		t.Fatal("failed fetch was treated as completed", err)
	}
	peek := false
	for command := range commands {
		peek = peek || strings.Contains(command, "BODY.PEEK[]")
		if strings.Contains(command, "STORE") {
			t.Fatal("failed fetch changed message flags")
		}
	}
	if !peek {
		t.Fatal("fetch could mark reports seen before persistence")
	}
}
