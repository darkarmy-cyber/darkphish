package models

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"gopkg.in/check.v1"
)

func (s *ModelsSuite) TestObjectWritesEnforceCreateOnlyAndOwnership(c *check.C) {
	owner, err := GetUser(1)
	c.Assert(err, check.IsNil)
	attacker := createSecurityTestUser(c, "object-boundary-attacker", RoleUser)

	template := Template{Name: "owned template", UserId: owner.Id, Subject: "original subject", Text: "original text"}
	c.Assert(PostTemplate(&template), check.IsNil)
	page := Page{Name: "owned page", UserId: owner.Id, HTML: "<p>original page</p>"}
	c.Assert(PostPage(&page), check.IsNil)
	smtp := SMTP{Name: "owned smtp", UserId: owner.Id, Host: "smtp.example.test:25", FromAddress: "owner@example.test"}
	c.Assert(PostSMTP(&smtp), check.IsNil)

	campaign := Campaign{
		UserId: owner.Id, Name: "association guard", TemplateId: template.Id,
		PageId: page.Id, SMTPId: smtp.Id, Status: CampaignQueued,
	}
	c.Assert(db.Create(&campaign).Error, check.IsNil)
	c.Assert(db.Create(&MailLog{UserId: owner.Id, CampaignId: campaign.Id, RId: "association-guard", SendDate: time.Now().UTC()}).Error, check.IsNil)

	c.Assert(PostTemplate(&Template{Id: template.Id, UserId: attacker.Id, Name: "overwrite", Text: "overwrite"}), check.Equals, ErrTemplateIDSpecified)
	c.Assert(PostPage(&Page{Id: page.Id, UserId: attacker.Id, Name: "overwrite", HTML: "<p>overwrite</p>"}), check.Equals, ErrPageIDSpecified)
	c.Assert(PostSMTP(&SMTP{Id: smtp.Id, UserId: attacker.Id, Name: "overwrite", Host: "evil.example:25", FromAddress: "evil@example.test"}), check.Equals, ErrSMTPIDSpecified)

	c.Assert(errors.Is(PutTemplate(&Template{Id: template.Id, UserId: attacker.Id, Name: "overwrite", Text: "overwrite"}), gorm.ErrRecordNotFound), check.Equals, true)
	c.Assert(errors.Is(PutPage(&Page{Id: page.Id, UserId: attacker.Id, Name: "overwrite", HTML: "<p>overwrite</p>"}), gorm.ErrRecordNotFound), check.Equals, true)
	c.Assert(errors.Is(PutSMTP(&SMTP{Id: smtp.Id, UserId: attacker.Id, Interface: "SMTP", Name: "overwrite", Host: "evil.example:25", FromAddress: "evil@example.test"}), gorm.ErrRecordNotFound), check.Equals, true)

	reloadedTemplate, err := GetTemplate(template.Id, owner.Id)
	c.Assert(err, check.IsNil)
	c.Assert(reloadedTemplate.Subject, check.Equals, "original subject")
	reloadedPage, err := GetPage(page.Id, owner.Id)
	c.Assert(err, check.IsNil)
	c.Assert(reloadedPage.Name, check.Equals, "owned page")
	reloadedSMTP, err := GetSMTP(smtp.Id, owner.Id)
	c.Assert(err, check.IsNil)
	c.Assert(reloadedSMTP.Host, check.Equals, "smtp.example.test:25")

	var reloadedCampaign Campaign
	c.Assert(db.Where("id=?", campaign.Id).Take(&reloadedCampaign).Error, check.IsNil)
	c.Assert(reloadedCampaign.TemplateId, check.Equals, template.Id)
	c.Assert(reloadedCampaign.PageId, check.Equals, page.Id)
	c.Assert(reloadedCampaign.SMTPId, check.Equals, smtp.Id)
	var queued int64
	c.Assert(db.Model(&MailLog{}).Where("campaign_id=? AND r_id=?", campaign.Id, "association-guard").Count(&queued).Error, check.IsNil)
	c.Assert(queued, check.Equals, int64(1))
}

func (s *ModelsSuite) TestLegacySharedRecipientUsesTenantCopyOnWrite(c *check.C) {
	ownerA := createSecurityTestUser(c, "recipient-owner-a", RoleUser)
	ownerB := createSecurityTestUser(c, "recipient-owner-b", RoleUser)
	recipient := Target{BaseRecipient: BaseRecipient{
		Email: "shared@example.test", FirstName: "Original", LastName: "Recipient", Position: "Analyst",
	}}
	groupA := Group{Name: "owner a group", UserId: ownerA.Id, Targets: []Target{recipient}}
	groupB := Group{Name: "owner b group", UserId: ownerB.Id, Targets: []Target{recipient}}
	c.Assert(PostGroup(&groupA), check.IsNil)
	c.Assert(PostGroup(&groupB), check.IsNil)
	c.Assert(groupA.Id == groupB.Id, check.Equals, false)

	targetsA, err := GetTargets(groupA.Id)
	c.Assert(err, check.IsNil)
	targetsB, err := GetTargets(groupB.Id)
	c.Assert(err, check.IsNil)
	c.Assert(targetsA[0].Id == targetsB[0].Id, check.Equals, false)

	// Recreate the historical globally deduplicated state to exercise the
	// migration-free copy-on-write path.
	c.Assert(db.Where("group_id=? AND target_id=?", groupB.Id, targetsB[0].Id).Delete(&GroupTarget{}).Error, check.IsNil)
	c.Assert(db.Delete(&Target{}, targetsB[0].Id).Error, check.IsNil)
	c.Assert(db.Create(&GroupTarget{GroupId: groupB.Id, TargetId: targetsA[0].Id}).Error, check.IsNil)

	campaign := Campaign{UserId: ownerB.Id, Name: "recipient snapshot", Status: CampaignQueued}
	c.Assert(db.Create(&campaign).Error, check.IsNil)
	result := Result{
		BaseRecipient: recipient.BaseRecipient, CampaignId: campaign.Id,
		UserId: ownerB.Id, RId: "recipient-snapshot", Status: StatusScheduled,
	}
	c.Assert(db.Create(&result).Error, check.IsNil)

	groupA.Targets = []Target{{BaseRecipient: BaseRecipient{
		Email: recipient.Email, FirstName: "Changed", LastName: "OnlyOwnerA", Position: "Manager",
	}}}
	c.Assert(PutGroup(&groupA), check.IsNil)

	updatedA, err := GetTargets(groupA.Id)
	c.Assert(err, check.IsNil)
	unchangedB, err := GetTargets(groupB.Id)
	c.Assert(err, check.IsNil)
	c.Assert(updatedA[0].FirstName, check.Equals, "Changed")
	c.Assert(unchangedB[0].FirstName, check.Equals, "Original")
	c.Assert(updatedA[0].Id == unchangedB[0].Id, check.Equals, false)

	var snapshot Result
	c.Assert(db.Where("r_id=?", "recipient-snapshot").Take(&snapshot).Error, check.IsNil)
	c.Assert(snapshot.FirstName, check.Equals, "Original")
	c.Assert(snapshot.LastName, check.Equals, "Recipient")
}
