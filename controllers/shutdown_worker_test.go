package controllers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/darkarmy-cyber/darkphish/models"
)

type shutdownTestWorker struct {
	stop chan struct{}
	once sync.Once
}

func (w *shutdownTestWorker) Start() {}

func (w *shutdownTestWorker) LaunchCampaign(models.Campaign) {}

func (w *shutdownTestWorker) SendTestEmail(*models.EmailRequest) error {
	<-w.stop
	return nil
}

func (w *shutdownTestWorker) Shutdown() { w.once.Do(func() { close(w.stop) }) }

func TestHTTPShutdownUnblocksWorkerDependentHandler(t *testing.T) {
	w := &shutdownTestWorker{stop: make(chan struct{})}
	defer w.Shutdown()
	entered := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		close(entered)
		_ = w.SendTestEmail(nil)
		rw.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		response, err := server.Client().Get(server.URL)
		if err == nil {
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
		}
	}()
	<-entered
	admin := &AdminServer{server: server.Config, worker: w}
	done := make(chan error, 1)
	go func() { done <- admin.Shutdown() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		w.Shutdown()
		t.Fatal("HTTP shutdown waited for a worker that it had not stopped")
	}
	<-requestDone
}
