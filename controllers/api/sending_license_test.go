package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/darkarmy-cyber/darkphish/internal/licensetest"
	"github.com/darkarmy-cyber/darkphish/internal/licensing"
	"github.com/darkarmy-cyber/darkphish/models"
)

type licenseTestWorker struct {
	deny  bool
	calls int
}

func (*licenseTestWorker) Start()                         {}
func (*licenseTestWorker) LaunchCampaign(models.Campaign) {}
func (w *licenseTestWorker) SendTestEmail(*models.EmailRequest) error {
	w.calls++
	if w.deny {
		return models.ErrSendingLicenseRequired
	}
	return nil
}

func TestLicensedTestEmailAPIAndLateDenial(t *testing.T) {
	ctx := setupTest(t)
	models.ConfigureLicenseManager(licensetest.Manager(t, licensing.StateActive))
	t.Cleanup(func() { models.ConfigureLicenseManager(nil) })
	w := &licenseTestWorker{}
	ctx.apiServer.worker = w
	for _, deny := range []bool{false, true} {
		w.deny = deny
		r := httptest.NewRequest(http.MethodPost, "/api/util/send_test_email", strings.NewReader(`{"email":"recipient@example.test","smtp":{"name":"local fake","from_address":"sender@example.test","host":"smtp.example.test:587"}}`))
		r.Header.Set("Authorization", "Bearer "+ctx.apiKey)
		response := httptest.NewRecorder()
		ctx.apiServer.ServeHTTP(response, r)
		want := http.StatusOK
		if deny {
			want = http.StatusForbidden
		}
		if response.Code != want {
			t.Fatalf("late denial=%v status=%d body=%s", deny, response.Code, response.Body.String())
		}
	}
	if w.calls != 2 {
		t.Fatal("licensed request did not reach fake worker")
	}
}

func TestSendingAPIsDenyBeforeParsingOrWorkerAccess(t *testing.T) {
	ctx := setupTest(t)
	ctx.apiServer.worker = nil // Any unauthorized dispatch would panic.
	t.Cleanup(func() { models.ConfigureLicenseManager(nil) })
	for _, state := range []licensing.State{"unconfigured", licensing.StateMissing, licensing.StateInvalid, licensing.StateExpired, licensing.StateActive, licensing.StateGrace} {
		t.Run(string(state), func(t *testing.T) {
			models.ConfigureLicenseManager(nil)
			if state != "unconfigured" {
				models.ConfigureLicenseManager(licensetest.Manager(t, state))
			}
			for _, path := range []string{"/api/campaigns/", "/api/util/send_test_email"} {
				r := httptest.NewRequest(http.MethodPost, path, strings.NewReader("invalid JSON"))
				r.Header.Set("Authorization", "Bearer "+ctx.apiKey)
				w := httptest.NewRecorder()
				ctx.apiServer.ServeHTTP(w, r)
				want := http.StatusForbidden
				if state.AllowsExpansion() {
					want = http.StatusBadRequest
				}
				if w.Code != want {
					t.Fatalf("%s state=%s status=%d want=%d", path, state, w.Code, want)
				}
				if !state.AllowsExpansion() && !strings.Contains(w.Body.String(), "Activate a valid DarkPhish license") {
					t.Fatal("missing actionable warning")
				}
			}
		})
	}
}
