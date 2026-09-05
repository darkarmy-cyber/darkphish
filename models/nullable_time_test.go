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

func (s *ModelsSuite) TestCampaignSummaryLegacyDates(c *check.C) {
	campaign := s.createCampaign(c)
	user, err := GetUser(campaign.UserId)
	c.Assert(err, check.IsNil)
	for _, instant := range []time.Time{{}, time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)} {
		// Reproduce persisted pre-nullability data without calling model hooks.
		c.Assert(db.Model(&Campaign{}).Where("id=?", campaign.Id).UpdateColumns(map[string]interface{}{
			"send_by_date": instant, "completed_date": instant,
		}).Error, check.IsNil)
		owned, err := GetCampaignSummaries(user.Id)
		c.Assert(err, check.IsNil)
		accessible, err := GetAccessibleCampaignSummaries(user, time.Now().UTC())
		c.Assert(err, check.IsNil)
		single, err := GetCampaignSummary(campaign.Id, user.Id)
		c.Assert(err, check.IsNil)
		authorized, err := GetAccessibleCampaignSummary(campaign.Id, user)
		c.Assert(err, check.IsNil)
		c.Assert(len(owned.Campaigns), check.Equals, 1)
		c.Assert(len(accessible.Campaigns), check.Equals, 1)
		for _, summary := range []CampaignSummary{owned.Campaigns[0], accessible.Campaigns[0], single, authorized} {
			if instant.IsZero() {
				c.Assert(summary.SendByDate, check.IsNil)
				c.Assert(summary.CompletedDate, check.IsNil)
				encoded, err := json.Marshal(summary)
				c.Assert(err, check.IsNil)
				c.Assert(strings.Contains(string(encoded), "\"send_by_date\":null"), check.Equals, true)
				c.Assert(strings.Contains(string(encoded), "\"completed_date\":null"), check.Equals, true)
			} else {
				c.Assert(summary.SendByDate.Equal(instant), check.Equals, true)
				c.Assert(summary.CompletedDate.Equal(instant), check.Equals, true)
			}
		}
	}
}
