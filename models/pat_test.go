package models

import (
	"encoding/json"
	"strings"
	"time"

	"gopkg.in/check.v1"
)

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
