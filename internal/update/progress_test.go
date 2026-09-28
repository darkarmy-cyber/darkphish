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
	s.releases["0.19.0"] = s.release
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
	s.releases["0.19.0"] = s.release
	s.status.Available, s.status.Checked, s.status.Latest = true, time.Now(), "0.19.0"
	if target, err := s.ApplyVersion(); err == nil || target != "" || s.Status().Target != "" || s.Status().Applying {
		t.Fatal("rejected submission claimed an accepted target")
	}
}

func TestSelectedVersionPolicyAllowsUpgradeAndReinstallButBlocksDowngrade(t *testing.T) {
	for _, tc := range []struct {
		name, current, target string
		wantErr               bool
	}{
		{name: "upgrade", current: "0.20.0", target: "0.21.0"},
		{name: "reinstall", current: "0.20.0", target: "0.20.0"},
		{name: "downgrade", current: "0.20.0", target: "0.19.0", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var submitted string
			s := NewService(tc.current, "", func(r Release) error { submitted = r.Version(); return nil })
			for _, version := range []string{"0.21.0", "0.20.0", "0.19.0"} {
				s.releases[version] = Release{Tag: "v" + version}
			}
			s.status.Latest, s.status.Checked = "0.21.0", time.Now()
			got, err := s.ApplyVersion(tc.target)
			if tc.wantErr {
				if err == nil || got != "" || submitted != "" || s.Status().Applying {
					t.Fatalf("unsafe target accepted: %q %q %+v %v", got, submitted, s.Status(), err)
				}
				return
			}
			if err != nil || got != tc.target || submitted != tc.target || !s.Status().Applying {
				t.Fatalf("target not accepted: %q %q %+v %v", got, submitted, s.Status(), err)
			}
		})
	}
}

func TestSetResultTargetRestoresReinstallReceipt(t *testing.T) {
	s := NewService("0.21.0", "", nil)
	s.SetResultTarget("applied", "v0.21.0")
	status := s.Status()
	if status.ResultCode != "applied" || status.Target != "0.21.0" {
		t.Fatalf("missing durable target receipt: %+v", status)
	}
	s = NewService("0.21.0", "", nil)
	s.SetResultTarget("applied", "not-a-version")
	if s.Status().Target != "" {
		t.Fatal("accepted invalid durable target")
	}
}
