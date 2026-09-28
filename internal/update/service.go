package update

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/darkarmy-cyber/darkphish/internal/audit"
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

type ReleaseOption struct {
	Version        string    `json:"version"`
	Published      time.Time `json:"published_at"`
	Notes          string    `json:"release_notes"`
	Current        bool      `json:"current"`
	Latest         bool      `json:"latest"`
	Compatible     bool      `json:"compatible"`
	Action         string    `json:"action"`
	DisabledReason string    `json:"disabled_reason,omitempty"`
}

type Status struct {
	Current     string          `json:"current_version"`
	Latest      string          `json:"latest_version"`
	Selected    string          `json:"selected_version,omitempty"`
	Notes       string          `json:"release_notes"`
	Published   time.Time       `json:"published_at"`
	Checked     time.Time       `json:"checked_at"`
	Available   bool            `json:"available"`
	Badge       int             `json:"badge"`
	Releases    []ReleaseOption `json:"releases,omitempty"`
	Unsupported string          `json:"unsupported_reason"`
	Error       string          `json:"error,omitempty"`
	Applying    bool            `json:"applying"`
	Target      string          `json:"target_version,omitempty"`
	Result      string          `json:"result,omitempty"`
	ResultCode  string          `json:"result_code,omitempty"`
}

type Service struct {
	mu       sync.Mutex
	client   *Client
	status   Status
	release  Release
	releases map[string]Release
	request  func(Release) error
}

func NewService(current, unsupported string, request func(Release) error) *Service {
	return &Service{client: NewClient(), status: Status{Current: current, Unsupported: unsupported}, releases: map[string]Release{}, request: request}
}

func (s *Service) Status() Status { s.mu.Lock(); defer s.mu.Unlock(); return s.status }

// SetResult keeps the last transaction outcome separate from release-check
// errors so a bell poll or automatic check cannot erase a rollback notice.
func (s *Service) SetResult(result string) {
	s.SetResultTarget(result, "")
}

// SetResultTarget restores the durable supervisor outcome and, when available,
// the exact semantic-version target. This lets a browser distinguish a
// successful reinstall from a stale success message after process restart.
func (s *Service) SetResultTarget(result, target string) {
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
	version := target
	if len(version) > 0 && version[0] == 'v' {
		version = version[1:]
	}
	if _, err := Compare(version, version); err == nil {
		s.status.Target = version
	}
}

func optionFor(r Release, current, latest string) (ReleaseOption, error) {
	cmp, err := Compare(r.Version(), current)
	if err != nil {
		return ReleaseOption{}, err
	}
	option := ReleaseOption{
		Version: r.Version(), Published: r.Published, Notes: r.Notes,
		Current: cmp == 0, Latest: r.Version() == latest, Compatible: cmp >= 0,
	}
	switch {
	case cmp > 0:
		option.Action = "upgrade"
	case cmp == 0:
		option.Action = "reinstall"
	default:
		option.Action = "downgrade"
		option.DisabledReason = "Downgrade is unavailable until database and configuration compatibility is explicitly verified."
	}
	return option, nil
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
	stable, err := s.client.Stable(ctx)
	s.status.Checked = time.Now().UTC()
	s.status.Available = false
	s.status.Badge = 0
	s.status.Error = ""
	s.status.Releases = nil
	s.status.Selected = ""
	s.releases = map[string]Release{}
	if err != nil {
		s.status.Error = err.Error()
		return s.status, err
	}
	if _, err = Compare(s.status.Current, s.status.Current); err != nil {
		s.status.Error = "Current build is not a stable release"
		return s.status, err
	}
	latest := stable[0]
	s.release = latest
	s.status.Latest = latest.Version()
	for _, r := range stable {
		option, optionErr := optionFor(r, s.status.Current, s.status.Latest)
		if optionErr != nil {
			continue
		}
		s.status.Releases = append(s.status.Releases, option)
		s.releases[r.Version()] = r
	}
	cmp, err := Compare(s.status.Latest, s.status.Current)
	if err != nil {
		s.status.Error = "Current build is not a stable release"
		return s.status, err
	}
	s.status.Available = cmp > 0
	if s.status.Available {
		s.status.Badge = 1
		s.status.Selected = s.status.Latest
	} else if _, ok := s.releases[s.status.Current]; ok {
		s.status.Selected = s.status.Current
	} else {
		s.status.Selected = s.status.Latest
	}
	selected := s.releases[s.status.Selected]
	s.status.Notes = selected.Notes
	s.status.Published = selected.Published
	checkResult = "success"
	return s.status, nil
}

func (s *Service) Apply() error {
	_, err := s.ApplyVersion()
	return err
}

// ApplyVersion returns the version accepted by the supervisor under the same
// lock as submission. A browser's earlier release check may already be stale.
// The optional target is a semantic version only; release URLs, commit IDs and
// filesystem paths are always resolved by the server-side trusted catalogue.
func (s *Service) ApplyVersion(target ...string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status.Unsupported != "" || s.request == nil {
		return "", errors.New("one-click update is unsupported for this installation")
	}
	if s.status.Applying {
		return "", errors.New("an update is already in progress")
	}
	if s.status.Error != "" || s.status.Checked.IsZero() || time.Since(s.status.Checked) > 5*time.Minute {
		return "", errors.New("check for updates again before applying")
	}
	version := s.status.Latest
	if len(target) > 1 {
		return "", errors.New("invalid update target")
	}
	if len(target) == 1 && target[0] != "" {
		version = target[0]
	}
	if _, err := Compare(version, version); err != nil {
		return "", errors.New("invalid update target")
	}
	r, ok := s.releases[version]
	if !ok {
		return "", errors.New("selected version is not in the trusted stable release catalogue")
	}
	cmp, err := Compare(version, s.status.Current)
	if err != nil {
		return "", errors.New("current build is not a stable release")
	}
	if cmp < 0 {
		return "", errors.New("downgrade is unavailable until compatibility is explicitly verified")
	}
	if err := s.request(r); err != nil {
		return "", err
	}
	s.release = r
	s.status.Applying = true
	s.status.Target = r.Version()
	s.status.Selected = r.Version()
	s.status.Notes = r.Notes
	s.status.Published = r.Published
	s.status.Result = ""
	s.status.ResultCode = ""
	return s.status.Target, nil
}
