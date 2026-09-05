package models

import (
	"encoding/json"
	"strings"
	"time"

	check "gopkg.in/check.v1"
)

func (s *ModelsSuite) TestNullableLastLogin(c *check.C) {
	role, err := GetRoleBySlug(RoleSecurityReviewer)
	c.Assert(err, check.IsNil)
	user := User{Username: "never-logged-in", ApiKey: "disabled-nullable-test", RoleID: role.ID}
	c.Assert(PutUser(&user), check.IsNil)
	loaded, err := GetUser(user.Id)
	c.Assert(err, check.IsNil)
	c.Assert(loaded.LastLogin, check.IsNil)
	encoded, err := json.Marshal(loaded)
	c.Assert(err, check.IsNil)
	c.Assert(strings.Contains(string(encoded), `"last_login":null`), check.Equals, true)
	var absent int
	c.Assert(db.Model(&User{}).Where("id=? AND last_login IS NULL", user.Id).Count(&absent).Error, check.IsNil)
	c.Assert(absent, check.Equals, 1)
	instant := time.Now().UTC().Truncate(time.Second)
	user.LastLogin = &instant
	c.Assert(PutUser(&user), check.IsNil)
	loaded, err = GetUser(user.Id)
	c.Assert(err, check.IsNil)
	c.Assert(loaded.LastLogin.Equal(instant), check.Equals, true)
	zero := time.Time{}
	user.LastLogin = &zero
	c.Assert(PutUser(&user), check.IsNil)
	loaded, err = GetUser(user.Id)
	c.Assert(err, check.IsNil)
	c.Assert(loaded.LastLogin, check.IsNil)
}

func (s *ModelsSuite) TestOptionalCampaignDatesAreNull(c *check.C) {
	campaign := s.createCampaign(c)
	c.Assert(campaign.SendByDate, check.IsNil)
	c.Assert(campaign.CompletedDate, check.IsNil)
	var absent int
	c.Assert(db.Model(&Campaign{}).Where("id=? AND send_by_date IS NULL AND completed_date IS NULL", campaign.Id).Count(&absent).Error, check.IsNil)
	c.Assert(absent, check.Equals, 1)
	c.Assert(CompleteCampaign(campaign.Id, campaign.UserId), check.IsNil)
	loaded, err := GetCampaign(campaign.Id, campaign.UserId)
	c.Assert(err, check.IsNil)
	c.Assert(loaded.CompletedDate, check.NotNil)
}
