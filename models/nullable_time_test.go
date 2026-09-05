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

func (s *ModelsSuite) TestRequiredModificationDates(c *check.C) {
	group := Group{Name: "timestamp group", UserId: 1, Targets: []Target{{BaseRecipient: BaseRecipient{Email: "timestamp@example.test"}}}}
	c.Assert(PostGroup(&group), check.IsNil)
	template := Template{Name: "timestamp template", UserId: 1, Subject: "test", Text: "test"}
	c.Assert(PostTemplate(&template), check.IsNil)
	page := Page{Name: "timestamp page", UserId: 1, HTML: "<p>test</p>"}
	c.Assert(PostPage(&page), check.IsNil)
	smtp := SMTP{Name: "timestamp smtp", UserId: 1, Host: "example.test:25", FromAddress: "sender@example.test"}
	c.Assert(PostSMTP(&smtp), check.IsNil)
	for _, value := range []time.Time{group.ModifiedDate, template.ModifiedDate, page.ModifiedDate, smtp.ModifiedDate} {
		c.Assert(value.IsZero(), check.Equals, false)
	}
	old := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	group.ModifiedDate = old
	c.Assert(PutGroup(&group), check.IsNil)
	loaded, err := GetGroup(group.Id, 1)
	c.Assert(err, check.IsNil)
	c.Assert(loaded.ModifiedDate.Equal(old), check.Equals, true)
}
