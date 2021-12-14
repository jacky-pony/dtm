package storage

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/yedf/dtm/common"
	"github.com/yedf/dtm/dtmcli/dtmimp"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SqlStore struct {
}

func (s *SqlStore) PopulateData(skipDrop bool) {
	file := fmt.Sprintf("%s/storage.%s.sql", common.GetCallerCodeDir(), config.DB["driver"])
	common.RunSQLScript(config.DB, file, skipDrop)
}

func (s *SqlStore) GetTransGlobal(gid string, trans *TransGlobalStore) error {
	dbr := dbGet().Model(trans).Where("gid=?", gid).First(trans)
	return wrapError(dbr.Error)
}

func (s *SqlStore) GetTransGlobals(lid int, globals interface{}) {
	dbGet().Must().Where("id < ?", lid).Order("id desc").Limit(100).Find(globals)
}

func (s *SqlStore) GetBranches(gid string) []TransBranchStore {
	branches := []TransBranchStore{}
	dbGet().Must().Where("gid=?", gid).Order("id asc").Find(&branches)
	return branches
}

func (s *SqlStore) UpdateBranches(branches []TransBranchStore, updates []string) *gorm.DB {
	return dbGet().Clauses(clause.OnConflict{
		OnConstraint: "trans_branch_op_pkey",
		DoUpdates:    clause.AssignmentColumns(updates),
	}).Create(branches)
}

func (s *SqlStore) LockGlobalSaveBranches(gid string, status string, branches []TransBranchStore, branchStart int) error {
	return dbGet().Transaction(func(tx *gorm.DB) error {
		err := lockTransGlobal(tx, gid, status)
		if err != nil {
			return err
		}
		dbr := tx.Save(branches)
		return dbr.Error
	})
}

func (s *SqlStore) SaveNewTrans(global *TransGlobalStore, branches []TransBranchStore) error {
	return dbGet().Transaction(func(db1 *gorm.DB) error {
		db := &common.DB{DB: db1}
		dbr := db.Must().Clauses(clause.OnConflict{
			DoNothing: true,
		}).Create(global)
		if dbr.RowsAffected <= 0 { // 如果这个不是新事务，返回错误
			return ErrUniqueConflict
		}
		if len(branches) > 0 {
			db.Must().Clauses(clause.OnConflict{
				DoNothing: true,
			}).Create(&branches)
		}
		return nil
	})
}

func (s *SqlStore) ChangeGlobalStatus(global *TransGlobalStore, newStatus string, updates []string, finished bool) error {
	old := global.Status
	global.Status = newStatus
	dbr := dbGet().Must().Model(global).Where("status=? and gid=?", old, global.Gid).Select(updates).Updates(global)
	if dbr.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SqlStore) TouchCronTime(global *TransGlobalStore, nextCronInterval int64) {
	global.NextCronTime = common.GetNextTime(nextCronInterval)
	global.UpdateTime = common.GetNextTime(0)
	global.NextCronInterval = nextCronInterval
	dbGet().Must().Model(global).Where("status=? and gid=?", global.Status, global.Gid).
		Select([]string{"next_cron_time", "update_time", "next_cron_interval"}).Updates(global)
}

func (s *SqlStore) LockOneGlobalTrans(global *TransGlobalStore, expireIn time.Duration) error {
	db := dbGet()
	getTime := dtmimp.GetDBSpecial().TimestampAdd
	expire := int(expireIn / time.Second)
	whereTime := fmt.Sprintf("next_cron_time < %s", getTime(expire))
	owner := uuid.NewString()
	dbr := db.Must().Model(global).
		Where(whereTime + "and status in ('prepared', 'aborting', 'submitted')").
		Limit(1).
		Select([]string{"owner", "next_cron_time"}).
		Updates(&TransGlobalStore{
			Owner:        owner,
			NextCronTime: common.GetNextTime(common.DtmConfig.RetryInterval),
		})
	if dbr.RowsAffected == 0 {
		return ErrNotFound
	}
	dbr = db.Must().Where("owner=?", owner).First(global)
	return nil
}

func lockTransGlobal(db *gorm.DB, gid string, status string) error {
	g := &TransGlobalStore{}
	dbr := db.Clauses(clause.Locking{Strength: "UPDATE"}).Model(g).Where("gid=? and status=?", gid, status).First(g)
	return wrapError(dbr.Error)
}
