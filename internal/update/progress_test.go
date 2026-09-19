package update

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestSafeOutcomeCodesSurviveReleaseCheckErrors(t *testing.T) {
	for _, code := range []string{"applied", "rollback", "backup_failed", "apply_failed", "verification_failed"} {
		s := NewService("0.17.0", "", nil)
		if code == "verification_failed" {
			s.Fail()
		} else {
			s.SetResult(code)
		}
		s.client.http.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("release feed offline") })
		status, err := s.Check(context.Background(), true)
		if err == nil || status.ResultCode != code || status.Result == "" || status.Applying {
			t.Fatalf("lost outcome: %+v %v", status, err)
		}
		var payload map[string]interface{}
		encoded, _ := json.Marshal(status)
		if err := json.Unmarshal(encoded, &payload); err != nil || payload["result_code"] != code {
			t.Fatalf("missing public safe code: %s", encoded)
		}
		s.SetResult("unknown or secret text")
		if s.Status().ResultCode != code {
			t.Fatal("unknown outcome replaced allowlisted diagnostic")
		}
	}
}

func TestAcceptedUpdateTargetIsIndependentOfReleaseFeed(t *testing.T) {
	var submitted string
	s := NewService("0.17.0", "", func(r Release) error { submitted = r.Version(); return nil })
	s.release = Release{Tag: "v0.19.0"}
	s.status.Available, s.status.Checked, s.status.Latest = true, time.Now(), "0.19.0"
	target, err := s.ApplyVersion()
	if err != nil || target != "0.19.0" || submitted != target || s.Status().Target != target {
		t.Fatalf("accepted target mismatch: %q %q %+v %v", target, submitted, s.Status(), err)
	}
	// Subsequent feed refreshes must not retarget an in-flight operation.
	s.mu.Lock()
	s.status.Latest, s.release = "0.20.0", Release{Tag: "v0.20.0"}
	s.mu.Unlock()
	if s.Status().Target != "0.19.0" {
		t.Fatal("release feed replaced accepted transaction target")
	}
	if target, err = s.ApplyVersion(); err == nil || target != "" || submitted != "0.19.0" {
		t.Fatal("concurrent apply replaced accepted transaction")
	}
}

func TestRejectedUpdateHasNoAcceptedTarget(t *testing.T) {
	s := NewService("0.17.0", "", func(Release) error { return errors.New("not accepted") })
	s.release = Release{Tag: "v0.19.0"}
	s.status.Available, s.status.Checked = true, time.Now()
	if target, err := s.ApplyVersion(); err == nil || target != "" || s.Status().Target != "" || s.Status().Applying {
		t.Fatal("rejected submission claimed an accepted target")
	}
}
