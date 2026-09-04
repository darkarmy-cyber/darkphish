package models

import (
	"time"

	check "gopkg.in/check.v1"
)

func (s *ModelsSuite) TestHandleEmailOpenedDoesNotAdvanceWhenEventSaveFails(c *check.C) {
	campaign := s.createCampaign(c)
	result := campaign.Results[0]
	originalStatus := result.Status

	err := db.Exec(`CREATE TRIGGER darkphish_test_fail_event
		BEFORE INSERT ON events
		BEGIN
			SELECT RAISE(FAIL, 'forced event persistence failure');
		END`).Error
	c.Assert(err, check.IsNil)
	defer db.Exec(`DROP TRIGGER darkphish_test_fail_event`)

	err = result.HandleEmailOpened(EventDetails{})
	c.Assert(err, check.NotNil)
	persisted, getErr := GetResult(result.RId)
	c.Assert(getErr, check.IsNil)
	c.Assert(persisted.Status, check.Equals, originalStatus)
}

func (s *ModelsSuite) TestGetCampaignResultsOrdersEvents(c *check.C) {
	campaign := s.createCampaign(c)
	c.Assert(db.Where("campaign_id = ?", campaign.Id).Delete(&Event{}).Error, check.IsNil)

	base := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	events := []*Event{
		{CampaignId: campaign.Id, Email: "first@example.test", Time: base.Add(time.Minute), Message: "same-time-first"},
		{CampaignId: campaign.Id, Email: "early@example.test", Time: base, Message: "early"},
		{CampaignId: campaign.Id, Email: "second@example.test", Time: base.Add(time.Minute), Message: "same-time-second"},
	}
	for _, event := range events {
		c.Assert(db.Save(event).Error, check.IsNil)
	}

	results, err := GetCampaignResults(campaign.Id, campaign.UserId)
	c.Assert(err, check.IsNil)
	c.Assert(results.Events, check.HasLen, 3)
	c.Assert(results.Events[0].Message, check.Equals, "early")
	c.Assert(results.Events[1].Message, check.Equals, "same-time-first")
	c.Assert(results.Events[2].Message, check.Equals, "same-time-second")
}
