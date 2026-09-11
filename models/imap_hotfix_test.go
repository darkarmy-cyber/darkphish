package models

import (
	"errors"

	"gopkg.in/check.v1"
)

func (s *ModelsSuite) TestIMAPPasswordPreserveRejectsAmbiguousRows(c *check.C) {
	const uid int64 = 91001
	first := IMAP{UserId: uid, Enabled: true, Host: "127.0.0.1", Port: 993, Username: "first@example.test", Password: "sealed-first", TLS: true, Folder: DefaultIMAPFolder, IMAPFreq: DefaultIMAPFreq}
	second := IMAP{UserId: uid, Enabled: true, Host: "127.0.0.1", Port: 993, Username: "second@example.test", Password: "sealed-second", TLS: true, Folder: DefaultIMAPFolder, IMAPFreq: DefaultIMAPFreq}
	c.Assert(db.Create(&first).Error, check.IsNil)
	c.Assert(db.Create(&second).Error, check.IsNil)

	update := IMAP{Enabled: false, Host: "localhost", Port: 994, Username: "updated@example.test", TLS: false, Folder: DefaultIMAPFolder, IMAPFreq: DefaultIMAPFreq}
	err := updateIMAPWithoutPassword(&update, uid)
	c.Assert(errors.Is(err, ErrIMAPAmbiguousState), check.Equals, true)

	var rows []IMAP
	c.Assert(db.Where("user_id = ?", uid).Find(&rows).Error, check.IsNil)
	c.Assert(rows, check.HasLen, 2)
	passwords := map[string]bool{}
	for _, row := range rows {
		passwords[row.Password] = true
		c.Assert(row.Host, check.Equals, "127.0.0.1")
		c.Assert(row.Port, check.Equals, uint16(993))
		c.Assert(row.Enabled, check.Equals, true)
	}
	c.Assert(passwords["sealed-first"], check.Equals, true)
	c.Assert(passwords["sealed-second"], check.Equals, true)
}

func (s *ModelsSuite) TestIMAPPasswordPreserveRejectsEmptyPasswordDuplicate(c *check.C) {
	const uid int64 = 91002
	first := IMAP{UserId: uid, Enabled: true, Host: "127.0.0.1", Port: 993, Username: "first@example.test", Password: "sealed-first", TLS: true, Folder: DefaultIMAPFolder, IMAPFreq: DefaultIMAPFreq}
	duplicate := IMAP{UserId: uid, Enabled: true, Host: "127.0.0.1", Port: 993, Username: "legacy-empty@example.test", Password: "", TLS: true, Folder: DefaultIMAPFolder, IMAPFreq: DefaultIMAPFreq}
	c.Assert(db.Create(&first).Error, check.IsNil)
	c.Assert(db.Create(&duplicate).Error, check.IsNil)

	update := IMAP{Enabled: false, Host: "localhost", Port: 994, Username: "updated@example.test", TLS: false, Folder: DefaultIMAPFolder, IMAPFreq: DefaultIMAPFreq}
	err := updateIMAPWithoutPassword(&update, uid)
	c.Assert(errors.Is(err, ErrIMAPAmbiguousState), check.Equals, true)

	var rows []IMAP
	c.Assert(db.Where("user_id = ?", uid).Find(&rows).Error, check.IsNil)
	c.Assert(rows, check.HasLen, 2)
	for _, row := range rows {
		c.Assert(row.Host, check.Equals, "127.0.0.1")
		c.Assert(row.Port, check.Equals, uint16(993))
		c.Assert(row.Enabled, check.Equals, true)
	}
}
