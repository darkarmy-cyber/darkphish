package imap

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jordan-wright/email"
)

func TestReportRIDWorkIsBounded(t *testing.T) {
	em := &email.Email{Text: []byte(strings.Repeat("?rid=AbC1234 ", 1000000))}
	if rids, err := matchEmail(context.Background(), em); err == nil || len(rids) != 0 {
		t.Fatal("unbounded RID input reached persistence", len(rids), err)
	}
	em = &email.Email{}
	for i := 0; i < maxReportRIDs; i++ {
		em.Text = append(em.Text, []byte(fmt.Sprintf("?rid=%07d ", i))...)
	}
	if rids, err := matchEmail(context.Background(), em); err != nil || len(rids) != maxReportRIDs {
		t.Fatal("valid bounded report was rejected", len(rids), err)
	}
	em.HTML = []byte("?rid=Zyx9876")
	if _, err := matchEmail(context.Background(), em); err == nil {
		t.Fatal("separate MIME parts bypassed the shared RID budget")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := matchEmail(ctx, &email.Email{Text: []byte("?rid=AbC1234")}); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled report was parsed", err)
	}
}

func TestReportProcessingStopsBetweenDatabaseOperations(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	err := processReportRIDs(ctx, map[string]bool{"AbC1234": true, "DeF5678": true}, func(string) error {
		calls++
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatal("cancellation did not stop in-memory RID processing", calls, err)
	}
}

func TestUnreadBatchesRotatePastPoisonMessages(t *testing.T) {
	seqs := make([]uint32, maxUnreadBatch+2)
	for i := range seqs {
		seqs[i] = uint32(i + 1)
	}
	first := unreadBatch(seqs, 0)
	if len(first) != maxUnreadBatch {
		t.Fatal("unbounded unread batch", len(first))
	}
	next := unreadBatch(seqs, first[len(first)-1])
	if len(next) != 2 || next[0] != maxUnreadBatch+1 {
		t.Fatal("unread poison messages starved later UIDs", next)
	}
	if again := unreadBatch(seqs, next[len(next)-1]); again[0] != 1 {
		t.Fatal("failed reports never retried", again)
	}
}
