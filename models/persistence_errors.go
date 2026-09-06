package models

import "gorm.io/gorm"

// ErrRecordNotFound preserves the public missing-object distinction without
// requiring HTTP adapters to import the ORM. Use errors.Is for wrapped errors.
var ErrRecordNotFound = gorm.ErrRecordNotFound
