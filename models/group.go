package models

import (
	"errors"
	"fmt"
	"net/mail"
	"time"

	log "github.com/darkarmy-cyber/darkphish/logger"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// Group contains the fields needed for a user -> group mapping
// Groups contain 1..* Targets
type Group struct {
	Id           int64     `json:"id"`
	UserId       int64     `json:"-"`
	Name         string    `json:"name"`
	ModifiedDate time.Time `json:"modified_date"`
	Targets      []Target  `json:"targets" gorm:"-"`
}

// GroupSummaries is a struct representing the overview of Groups.
type GroupSummaries struct {
	Total  int64          `json:"total"`
	Groups []GroupSummary `json:"groups"`
}

// GroupSummary represents a summary of the Group model. The only
// difference is that, instead of listing the Targets (which could be expensive
// for large groups), it lists the target count.
type GroupSummary struct {
	Id           int64     `json:"id"`
	Name         string    `json:"name"`
	ModifiedDate time.Time `json:"modified_date"`
	NumTargets   int64     `json:"num_targets"`
}

// GroupTarget is used for a many-to-many relationship between 1..* Groups and 1..* Targets
type GroupTarget struct {
	GroupId  int64 `json:"-"`
	TargetId int64 `json:"-"`
}

// Target contains the fields needed for individual targets specified by the user
// Groups contain 1..* Targets, but 1 Target may belong to 1..* Groups
type Target struct {
	Id int64 `json:"-"`
	BaseRecipient
}

// BaseRecipient contains the fields for a single recipient. This is the base
// struct used in members of groups and campaign results.
type BaseRecipient struct {
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Position  string `json:"position"`
}

// FormatAddress returns the email address to use in the "To" header of the email
func (r *BaseRecipient) FormatAddress() string {
	addr := r.Email
	if r.FirstName != "" && r.LastName != "" {
		a := &mail.Address{
			Name:    fmt.Sprintf("%s %s", r.FirstName, r.LastName),
			Address: r.Email,
		}
		addr = a.String()
	}
	return addr
}

// FormatAddress returns the email address to use in the "To" header of the email
func (t *Target) FormatAddress() string {
	addr := t.Email
	if t.FirstName != "" && t.LastName != "" {
		a := &mail.Address{
			Name:    fmt.Sprintf("%s %s", t.FirstName, t.LastName),
			Address: t.Email,
		}
		addr = a.String()
	}
	return addr
}

// ErrEmailNotSpecified is thrown when no email is specified for the Target
var ErrEmailNotSpecified = errors.New("No email address specified")

// ErrGroupNameNotSpecified is thrown when a group name is not specified
var ErrGroupNameNotSpecified = errors.New("Group name not specified")

// ErrNoTargetsSpecified is thrown when no targets are specified by the user
var ErrNoTargetsSpecified = errors.New("No targets specified")

// Validate performs validation on a group given by the user
func (g *Group) Validate() error {
	switch {
	case g.Name == "":
		return ErrGroupNameNotSpecified
	case len(g.Targets) == 0:
		return ErrNoTargetsSpecified
	}
	return nil
}

// GetGroups returns the groups owned by the given user.
func GetGroups(uid int64) ([]Group, error) {
	gs := []Group{}
	err := db.Where("user_id=?", uid).Find(&gs).Error
	if err != nil {
		log.Error(err)
		return gs, err
	}
	for i := range gs {
		gs[i].Targets, err = GetTargets(gs[i].Id)
		if err != nil {
			log.Error(err)
		}
	}
	return gs, nil
}

// GetGroupSummaries returns the summaries for the groups
// created by the given uid.
func GetGroupSummaries(uid int64) (GroupSummaries, error) {
	gs := GroupSummaries{}
	query := db.Table("groups").Where("user_id=?", uid)
	err := query.Select("id, name, modified_date").Scan(&gs.Groups).Error
	if err != nil {
		log.Error(err)
		return gs, err
	}
	for i := range gs.Groups {
		query = db.Table("group_targets").Where("group_id=?", gs.Groups[i].Id)
		err = query.Count(&gs.Groups[i].NumTargets).Error
		if err != nil {
			return gs, err
		}
	}
	gs.Total = int64(len(gs.Groups))
	return gs, nil
}

// GetGroup returns the group, if it exists, specified by the given id and user_id.
func GetGroup(id int64, uid int64) (Group, error) {
	g := Group{}
	err := db.Where("user_id=? and id=?", uid, id).Take(&g).Error
	if err != nil {
		log.Error(err)
		return g, err
	}
	g.Targets, err = GetTargets(g.Id)
	if err != nil {
		log.Error(err)
	}
	return g, nil
}

// GetGroupSummary returns the summary for the requested group
func GetGroupSummary(id int64, uid int64) (GroupSummary, error) {
	g := GroupSummary{}
	query := db.Table("groups").Where("user_id=? and id=?", uid, id)
	err := query.Select("id, name, modified_date").Take(&g).Error
	if err != nil {
		log.Error(err)
		return g, err
	}
	query = db.Table("group_targets").Where("group_id=?", id)
	err = query.Count(&g.NumTargets).Error
	if err != nil {
		return g, err
	}
	return g, nil
}

// GetGroupByName returns the group, if it exists, specified by the given name and user_id.
func GetGroupByName(n string, uid int64) (Group, error) {
	g := Group{}
	err := db.Where("user_id=? and name=?", uid, n).Take(&g).Error
	if err != nil {
		log.Error(err)
		return g, err
	}
	g.Targets, err = GetTargets(g.Id)
	if err != nil {
		log.Error(err)
	}
	return g, err
}

// PostGroup creates a new group in the database.
func PostGroup(g *Group) error {
	if g.Id != 0 {
		return errors.New("new groups must not specify an id")
	}
	if err := g.Validate(); err != nil {
		return err
	}
	// Insert the group into the DB
	tx := beginLicenseTransaction()
	if tx.Error != nil {
		return tx.Error
	}
	defer tx.Rollback()
	if err := lockLicenseUsage(tx); err != nil {
		return err
	}
	if err := enforceGroupLicense(tx, g); err != nil {
		return err
	}
	err := tx.Save(g).Error
	if err != nil {
		tx.Rollback()
		log.Error(err)
		return err
	}
	for _, t := range g.Targets {
		err = insertTargetIntoGroup(tx, t, g.Id)
		if err != nil {
			tx.Rollback()
			log.Error(err)
			return err
		}
	}
	err = tx.Commit().Error
	if err != nil {
		log.Error(err)
		tx.Rollback()
		return err
	}
	return nil
}

// PutGroup updates the given group if found in the database.
func PutGroup(g *Group) error {
	if err := g.Validate(); err != nil {
		return err
	}
	tx := beginLicenseTransaction()
	if tx.Error != nil {
		return tx.Error
	}
	defer tx.Rollback()
	if err := lockLicenseUsage(tx); err != nil {
		return err
	}
	// Fetch group's existing targets under the same usage lock.
	ts, err := getTargetsWithDB(tx, g.Id)
	if err != nil {
		log.WithFields(logrus.Fields{
			"group_id": g.Id,
		}).Error("Error getting targets from group")
		return err
	}
	// Preload the caches
	cacheNew := make(map[string]int64, len(g.Targets))
	for _, t := range g.Targets {
		cacheNew[t.Email] = t.Id
	}

	cacheExisting := make(map[string]int64, len(ts))
	for _, t := range ts {
		cacheExisting[t.Email] = t.Id
	}

	if err := enforceGroupLicense(tx, g); err != nil {
		return err
	}
	// Check existing targets, removing any that are no longer in the group.
	for _, t := range ts {
		if _, ok := cacheNew[t.Email]; ok {
			continue
		}

		// If the target does not exist in the group any longer, we delete it
		err := tx.Where("group_id=? and target_id=?", g.Id, t.Id).Delete(&GroupTarget{}).Error
		if err != nil {
			tx.Rollback()
			log.WithFields(logrus.Fields{
				"email": t.Email,
			}).Error("Error deleting email")
			return err
		}
	}
	// Add any targets that are not in the database yet.
	for _, nt := range g.Targets {
		// If the target already exists in the database, we should just update
		// the record with the latest information.
		if id, ok := cacheExisting[nt.Email]; ok {
			nt.Id = id
			err = UpdateTarget(tx, nt, g.UserId)
			if err != nil {
				log.Error(err)
				tx.Rollback()
				return err
			}
			continue
		}
		// Otherwise, add target if not in database
		err = insertTargetIntoGroup(tx, nt, g.Id)
		if err != nil {
			log.Error(err)
			tx.Rollback()
			return err
		}
	}
	err = tx.Save(g).Error
	if err != nil {
		log.Error(err)
		return err
	}
	err = tx.Commit().Error
	if err != nil {
		tx.Rollback()
		return err
	}
	return nil
}

// DeleteGroup deletes a given group by group ID and user ID
func DeleteGroup(g *Group) error {
	// Delete all the group_targets entries for this group
	err := db.Where("group_id=?", g.Id).Delete(&GroupTarget{}).Error
	if err != nil {
		log.Error(err)
		return err
	}
	// Delete the group itself
	err = db.Delete(g).Error
	if err != nil {
		log.Error(err)
		return err
	}
	return err
}

func insertTargetIntoGroup(tx *gorm.DB, t Target, gid int64) error {
	if _, err := mail.ParseAddress(t.Email); err != nil {
		log.WithFields(logrus.Fields{"email": t.Email}).Error("Invalid email")
		return err
	}
	var group Group
	if err := tx.Select("id", "user_id").Where("id=?", gid).Take(&group).Error; err != nil {
		return err
	}
	// Recipient identity is tenant-local. Reuse an identical row only when it
	// is already associated with another group owned by the same user.
	var existing Target
	err := tx.Table("targets").
		Select("targets.*").
		Joins("JOIN group_targets gt ON gt.target_id=targets.id").
		Where("gt.group_id IN (?) AND targets.email=? AND targets.first_name=? AND targets.last_name=? AND targets.position=?",
			tx.Model(&Group{}).Select("id").Where("user_id=?", group.UserId),
			t.Email, t.FirstName, t.LastName, t.Position).
		First(&existing).Error
	switch {
	case err == nil:
		t.Id = existing.Id
	case errors.Is(err, gorm.ErrRecordNotFound):
		t.Id = 0
		if err = tx.Create(&t).Error; err != nil {
			return err
		}
	default:
		return err
	}
	if err = tx.Create(&GroupTarget{GroupId: gid, TargetId: t.Id}).Error; err != nil {
		log.Error(err)
	}
	return err
}

// UpdateTarget updates recipient information without crossing tenant boundaries.
// Legacy databases may contain a target row shared by groups from different users;
// in that case all associations for the current owner move to a private copy first.
func UpdateTarget(tx *gorm.DB, target Target, ownerID int64) error {
	var foreignReferences int64
	if err := tx.Table("group_targets gt").
		Where("gt.target_id=? AND gt.group_id IN (?)", target.Id,
			tx.Model(&Group{}).Select("id").Where("user_id<>?", ownerID)).
		Count(&foreignReferences).Error; err != nil {
		return err
	}
	targetID := target.Id
	if foreignReferences > 0 {
		copyTarget := target
		copyTarget.Id = 0
		if err := tx.Create(&copyTarget).Error; err != nil {
			return err
		}
		var ownerGroupIDs []int64
		if err := tx.Model(&Group{}).
			Where("user_id=? AND id IN (?)", ownerID,
				tx.Table("group_targets").Select("group_id").Where("target_id=?", target.Id)).
			Pluck("id", &ownerGroupIDs).Error; err != nil {
			return err
		}
		for _, groupID := range ownerGroupIDs {
			if err := tx.Model(&GroupTarget{}).
				Where("group_id=? AND target_id=?", groupID, target.Id).
				Update("target_id", copyTarget.Id).Error; err != nil {
				return err
			}
		}
		targetID = copyTarget.Id
	}
	targetInfo := map[string]interface{}{
		"first_name": target.FirstName,
		"last_name":  target.LastName,
		"position":   target.Position,
	}
	err := tx.Model(&Target{}).Where("id=?", targetID).Updates(targetInfo).Error
	if err != nil {
		log.WithFields(logrus.Fields{"email": target.Email}).Error(err)
	}
	return err
}

// GetTargets performs a many-to-many select to get all the Targets for a Group
func GetTargets(gid int64) ([]Target, error) {
	return getTargetsWithDB(db, gid)
}

func getTargetsWithDB(connection *gorm.DB, gid int64) ([]Target, error) {
	ts := []Target{}
	err := connection.Table("targets").Select("targets.id, targets.email, targets.first_name, targets.last_name, targets.position").Joins("left join group_targets gt ON targets.id = gt.target_id").Where("gt.group_id=?", gid).Order("targets.id ASC").Scan(&ts).Error
	return ts, err
}
