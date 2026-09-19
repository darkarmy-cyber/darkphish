package update

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
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
