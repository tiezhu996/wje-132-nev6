package repository

import (
	"sync"
	"testing"
	"time"

	"safetyplatform/internal/constants"
	"safetyplatform/internal/model"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// newIncidentTestDB 构造内存 SQLite 测试库并自动迁移事件表。
func newIncidentTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.SafetyIncident{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db
}

func seedIncident(t *testing.T, db *gorm.DB, status string) model.SafetyIncident {
	t.Helper()
	i := model.SafetyIncident{
		Title: "测试隐患", OccurredAt: time.Now(), SeverityLevel: constants.SeverityMinor,
		Category: "其他", Status: status, ReporterID: 1,
		RectificationMeasures: "原整改措施", ReviewComment: "既有复核意见",
	}
	if err := db.Create(&i).Error; err != nil {
		t.Fatalf("seed incident: %v", err)
	}
	return i
}

// 状态匹配时迁移成功并写入更新字段。
func TestTransitionStatusSuccess(t *testing.T) {
	db := newIncidentTestDB(t)
	repo := NewSafetyIncidentRepository(db)
	i := seedIncident(t, db, constants.IncidentInvestigating)

	ok, err := repo.TransitionStatus(i.ID, constants.IncidentInvestigating, map[string]any{
		"status": constants.IncidentPendingReview,
	})
	if err != nil || !ok {
		t.Fatalf("expected transition ok, got ok=%v err=%v", ok, err)
	}
	var got model.SafetyIncident
	if err := db.First(&got, i.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.Status != constants.IncidentPendingReview {
		t.Fatalf("status = %q, want pending_review", got.Status)
	}
}

// 状态不匹配时迁移失败，且不得覆盖既有复核意见与整改措施。
func TestTransitionStatusConflictKeepsReviewFields(t *testing.T) {
	db := newIncidentTestDB(t)
	repo := NewSafetyIncidentRepository(db)
	i := seedIncident(t, db, constants.IncidentInvestigating)

	// 模拟并发下迟到的重复验收：当前状态不是 pending_review，必须迁移失败。
	ok, err := repo.TransitionStatus(i.ID, constants.IncidentPendingReview, map[string]any{
		"status":         constants.IncidentClosed,
		"review_result":  constants.ReviewResultApproved,
		"review_comment": "迟到验收",
	})
	if err != nil {
		t.Fatalf("transition: %v", err)
	}
	if ok {
		t.Fatal("expected no transition when status mismatches")
	}
	var got model.SafetyIncident
	if err := db.First(&got, i.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.Status != constants.IncidentInvestigating {
		t.Fatalf("status = %q, want investigating", got.Status)
	}
	if got.ReviewComment != "既有复核意见" {
		t.Fatalf("review comment overwritten: %q", got.ReviewComment)
	}
	if got.RectificationMeasures != "原整改措施" {
		t.Fatalf("rectification measures overwritten: %q", got.RectificationMeasures)
	}
}

// 并发验收同一待复核事件：只允许一次状态迁移。
func TestTransitionStatusConcurrentOnlyOneWins(t *testing.T) {
	db := newIncidentTestDB(t)
	repo := NewSafetyIncidentRepository(db)
	i := seedIncident(t, db, constants.IncidentPendingReview)

	const workers = 8
	var wg sync.WaitGroup
	wins := make(chan bool, workers)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := repo.TransitionStatus(i.ID, constants.IncidentPendingReview, map[string]any{
				"status":        constants.IncidentClosed,
				"review_result": constants.ReviewResultApproved,
			})
			if err != nil {
				t.Errorf("transition: %v", err)
				return
			}
			wins <- ok
		}()
	}
	wg.Wait()
	close(wins)

	success := 0
	for ok := range wins {
		if ok {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("expected exactly 1 successful transition, got %d", success)
	}
}

// 历史 resolved 数据迁移为 pending_review。
func TestMigrateLegacyStatuses(t *testing.T) {
	db := newIncidentTestDB(t)
	repo := NewSafetyIncidentRepository(db)
	legacy := seedIncident(t, db, constants.IncidentResolved)
	fresh := seedIncident(t, db, constants.IncidentInvestigating)

	if err := repo.MigrateLegacyStatuses(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	var gotLegacy, gotFresh model.SafetyIncident
	if err := db.First(&gotLegacy, legacy.ID).Error; err != nil {
		t.Fatalf("reload legacy: %v", err)
	}
	if err := db.First(&gotFresh, fresh.ID).Error; err != nil {
		t.Fatalf("reload fresh: %v", err)
	}
	if gotLegacy.Status != constants.IncidentPendingReview {
		t.Fatalf("legacy status = %q, want pending_review", gotLegacy.Status)
	}
	if gotFresh.Status != constants.IncidentInvestigating {
		t.Fatalf("fresh status = %q, want investigating", gotFresh.Status)
	}
}
