package update

import (
	"context"
	"errors"
	"github.com/darkarmy-cyber/darkphish/internal/audit"
	"sync"
	"time"
)

// ListenerReady is installed before listeners start in a supervised child.
var ListenerReady func(string)

var WaitServing func()

func (s *Service) Fail() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status.Applying = false
	s.status.Error = "Update verification failed; no application files were changed"
	s.status.Result = s.status.Error
	s.status.ResultCode = "verification_failed"
}

type Status struct {
	Current     string    `json:"current_version"`
	Latest      string    `json:"latest_version"`
	Notes       string    `json:"release_notes"`
	Published   time.Time `json:"published_at"`
	Checked     time.Time `json:"checked_at"`
	Available   bool      `json:"available"`
	Badge       int       `json:"badge"`
	Unsupported string    `json:"unsupported_reason"`
	Error       string    `json:"error,omitempty"`
	Applying    bool      `json:"applying"`
	Result      string    `json:"result,omitempty"`
	ResultCode  string    `json:"result_code,omitempty"`
}

type Service struct {
	mu      sync.Mutex
	client  *Client
	status  Status
	release Release
	request func(Release) error
}

func NewService(current, unsupported string, request func(Release) error) *Service {
	return &Service{client: NewClient(), status: Status{Current: current, Unsupported: unsupported}, request: request}
}

func (s *Service) Status() Status { s.mu.Lock(); defer s.mu.Unlock(); return s.status }

// SetResult keeps the last transaction outcome separate from release-check
// errors so a bell poll or automatic check cannot erase a rollback notice.
func (s *Service) SetResult(result string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch result {
	case "applied":
		s.status.Result = "Update completed successfully"
	case "rollback":
		s.status.Result = "Update failed; the previous application and database were restored"
	case "backup_failed":
		s.status.Result = "Pre-update backup failed; no application files were changed"
	case "apply_failed":
		s.status.Result = "Update could not start; the backup completed and no application files were changed"
	default:
		return
	}
	s.status.ResultCode = result
}

func (s *Service) Check(ctx context.Context, force bool) (Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !force && !s.status.Checked.IsZero() && time.Since(s.status.Checked) < time.Hour {
		return s.status, nil
	}
	checkResult := "failure"
	if !force {
		defer func() { audit.RecordSystem("update.check", "release", "stable", checkResult) }()
	}
	r, err := s.client.Latest(ctx)
	s.status.Checked = time.Now().UTC()
	s.status.Available = false
	s.status.Badge = 0
	s.status.Error = ""
	if err != nil {
		s.status.Error = err.Error()
		return s.status, err
	}
	cmp, err := Compare(r.Version(), s.status.Current)
	if err != nil {
		s.status.Error = "Current build is not a stable release"
		return s.status, err
	}
	s.release = r
	s.status.Latest = r.Version()
	s.status.Notes = r.Notes
	s.status.Published = r.Published
	s.status.Available = cmp > 0
	if s.status.Available {
		s.status.Badge = 1
	}
	checkResult = "success"
	return s.status, nil
}

func (s *Service) Apply() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status.Unsupported != "" || s.request == nil {
		return errors.New("one-click update is unsupported for this installation")
	}
	if s.status.Applying {
		return errors.New("an update is already in progress")
	}
	if !s.status.Available || s.status.Error != "" || time.Since(s.status.Checked) > 5*time.Minute {
		return errors.New("check for updates again before applying")
	}
	if err := s.request(s.release); err != nil {
		return err
	}
	s.status.Applying = true
	s.status.Result = ""
	s.status.ResultCode = ""
	return nil
}
