package models

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"gopkg.in/check.v1"
	"gorm.io/gorm"
)

func (s *ModelsSuite) TestPATAdmissionCreationDoesNotRescan(c *check.C) {
	_, raw, err := CreatePersonalAccessToken(1, "warm", []string{"landing-pages:read"}, time.Now().UTC().Add(time.Hour))
	c.Assert(err, check.IsNil)
	c.Assert(RecognizedPATForAdmission(raw), check.Equals, true)
	queries := 0
	name := "test:pat-admission-rescans"
	c.Assert(db.Callback().Query().Before("gorm:query").Register(name, func(tx *gorm.DB) {
		if len(tx.Statement.Selects) == 3 && tx.Statement.Selects[0] == "prefix" {
			queries++
		}
	}), check.IsNil)
	defer db.Callback().Query().Remove(name)
	for i := 0; i < 8; i++ {
		_, raw, err := CreatePersonalAccessToken(1, "incremental", []string{"landing-pages:read"}, time.Now().UTC().Add(time.Hour))
		c.Assert(err, check.IsNil)
		c.Assert(RecognizedPATForAdmission(raw), check.Equals, true)
	}
	c.Assert(queries, check.Equals, 0)
}

func (s *ModelsSuite) TestPersonalAccessTokenCapacity(c *check.C) {
	var first PersonalAccessToken
	for i := 0; i < MaxActivePersonalAccessTokens; i++ {
		pat, _, err := CreatePersonalAccessToken(1, "capacity", []string{"landing-pages:read"}, time.Now().UTC().Add(time.Hour))
		c.Assert(err, check.IsNil)
		if i == 0 {
			first = pat
		}
	}
	_, raw, err := CreatePersonalAccessToken(1, "overflow", []string{"landing-pages:read"}, time.Now().UTC().Add(time.Hour))
	c.Assert(err, check.Equals, ErrPATCapacity)
	c.Assert(raw, check.Equals, "")
	c.Assert(RevokePersonalAccessToken(first.ID, 1), check.IsNil)
	_, _, err = CreatePersonalAccessToken(1, "replacement", []string{"landing-pages:read"}, time.Now().UTC().Add(time.Hour))
	c.Assert(err, check.IsNil)
}

func (s *ModelsSuite) TestPATAdmissionRetainsSnapshotOnTransientFailure(c *check.C) {
	_, raw, err := CreatePersonalAccessToken(1, "admission", []string{"landing-pages:read"}, time.Now().UTC().Add(time.Hour))
	c.Assert(err, check.IsNil)
	c.Assert(RecognizedPATForAdmission(raw), check.Equals, true)
	invalidatePATAdmission()
	queries := 0
	name := "test:pat-admission-query-failure"
	c.Assert(db.Callback().Query().Before("gorm:query").Register(name, func(tx *gorm.DB) {
		queries++
		tx.AddError(errors.New("temporary database failure"))
	}), check.IsNil)
	defer db.Callback().Query().Remove(name)
	c.Assert(RecognizedPATForAdmission(raw), check.Equals, true)
	c.Assert(RecognizedPATForAdmission(raw), check.Equals, true)
	c.Assert(queries, check.Equals, 1)
	c.Assert(db.Callback().Query().Remove(name), check.IsNil)
	_, err = AuthenticatePersonalAccessToken(raw)
	c.Assert(err, check.IsNil)
}

func (s *ModelsSuite) TestPersonalAccessTokenLifecycle(c *check.C) {
	pat, raw, err := CreatePersonalAccessToken(1, "automation", []string{"campaigns:read", "reports:read"}, time.Now().UTC().Add(time.Hour))
	c.Assert(err, check.IsNil)
	c.Assert(strings.HasPrefix(raw, "darkphish_pat_"), check.Equals, true)
	c.Assert(pat.TokenHash == raw, check.Equals, false)

	stored := PersonalAccessToken{}
	c.Assert(db.Where("id=?", pat.ID).First(&stored).Error, check.IsNil)
	c.Assert(strings.Contains(stored.TokenHash, raw), check.Equals, false)
	_, secret, ok := parsePAT(raw)
	c.Assert(ok, check.Equals, true)
	c.Assert(stored.TokenHash, check.Equals, hashPATSecret(secret))
	encoded, err := json.Marshal(pat)
	c.Assert(err, check.IsNil)
	c.Assert(strings.Contains(string(encoded), raw), check.Equals, false)

	authentication, err := AuthenticatePersonalAccessToken(raw)
	c.Assert(err, check.IsNil)
	c.Assert(authentication.User.Id, check.Equals, int64(1))
	_, hasReports := authentication.Scopes["reports:read"]
	c.Assert(hasReports, check.Equals, true)

	c.Assert(RevokePersonalAccessToken(pat.ID, 1), check.IsNil)
	_, err = AuthenticatePersonalAccessToken(raw)
	c.Assert(err, check.Equals, ErrRevokedPAT)
}

func (s *ModelsSuite) TestPersonalAccessTokenExpiryAndScopeValidation(c *check.C) {
	_, _, err := CreatePersonalAccessToken(1, "bad scope", []string{"everything"}, time.Now().UTC().Add(time.Hour))
	c.Assert(err, check.NotNil)
	_, _, err = CreatePersonalAccessToken(1, "expired", []string{"campaigns:read"}, time.Now().UTC().Add(-time.Hour))
	c.Assert(err, check.NotNil)
}
