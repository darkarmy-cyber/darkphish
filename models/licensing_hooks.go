package models

import "gorm.io/gorm"

// BeforeCreate prevents campaign creation from bypassing Community licensing
// through alternate callers. Compatibility tools and tests remain unaffected
// until the official Community startup configures a license manager.
func (c *Campaign) BeforeCreate(tx *gorm.DB) error {
	return enforceCampaignLicense(tx)
}
