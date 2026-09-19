package repository

import (
	"errors"
	"fmt"
	"time"

	"safetyplatform/internal/constants"
	"safetyplatform/internal/model"

	"gorm.io/gorm"
)

// SafetyIncidentRepository 安全事件仓储。
type SafetyIncidentRepository struct {
	db *gorm.DB
}

// NewSafetyIncidentRepository 构造安全事件仓储。
func NewSafetyIncidentRepository(db *gorm.DB) *SafetyIncidentRepository {
	return &SafetyIncidentRepository{db: db}
}

// Create 创建事件。
func (r *SafetyIncidentRepository) Create(i *model.SafetyIncident) error {
	if err := r.db.Create(i).Error; err != nil {
		return fmt.Errorf("create safety incident: %w", err)
	}
	return nil
}

// FindByID 按 ID 查询事件。
func (r *SafetyIncidentRepository) FindByID(id uint64) (*model.SafetyIncident, error) {
	var i model.SafetyIncident
	if err := r.db.First(&i, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find safety incident by id: %w", err)
	}
	return &i, nil
}

// List 分页查询事件，支持严重等级/状态/时间筛选。
func (r *SafetyIncidentRepository) List(page, pageSize int, severity, status string, startDate, endDate *time.Time) ([]model.SafetyIncident, int64, error) {
	var list []model.SafetyIncident
	var total int64
	q := r.db.Model(&model.SafetyIncident{})
	if severity != "" {
		q = q.Where("severity_level = ?", severity)
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if startDate != nil {
		q = q.Where("occurred_at >= ?", startDate)
	}
	if endDate != nil {
		q = q.Where("occurred_at <= ?", endDate)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count incidents: %w", err)
	}
	if err := q.Order("occurred_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, 0, fmt.Errorf("list incidents: %w", err)
	}
	return list, total, nil
}

// Update 更新事件。
func (r *SafetyIncidentRepository) Update(i *model.SafetyIncident) error {
	if err := r.db.Save(i).Error; err != nil {
		return fmt.Errorf("update safety incident: %w", err)
	}
	return nil
}

// TransitionStatus 在事务内按“当前状态必须为 expectStatus”做原子条件更新。
// 并发/重复请求时只有一个请求的 WHERE status = ? 能命中，其余返回 ErrStatusConflict，
// 从而保证一次状态迁移且失败请求的字段不会落库（复核意见不会被覆盖）。
func (r *SafetyIncidentRepository) TransitionStatus(id uint64, expectStatus string, fields map[string]any) (*model.SafetyIncident, error) {
	var out model.SafetyIncident
	err := r.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&model.SafetyIncident{}).
			Where("id = ? AND status = ?", id, expectStatus).
			Updates(fields)
		if res.Error != nil {
			return fmt.Errorf("transition incident status: %w", res.Error)
		}
		if res.RowsAffected == 0 {
			return ErrStatusConflict
		}
		if err := tx.First(&out, id).Error; err != nil {
			return fmt.Errorf("reload incident after transition: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Trend30 近 30 天事件趋势。
func (r *SafetyIncidentRepository) Trend30() ([]map[string]any, error) {
	start := time.Now().AddDate(0, 0, -29)
	var rows []map[string]any
	if err := r.db.Model(&model.SafetyIncident{}).
		Select("DATE(occurred_at) AS day, COUNT(*) AS cnt").
		Where("occurred_at >= ?", start).
		Group("DATE(occurred_at)").Order("day ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("incident trend: %w", err)
	}
	return rows, nil
}

// SeverityDistribution 严重等级分布。
func (r *SafetyIncidentRepository) SeverityDistribution() ([]map[string]any, error) {
	var rows []map[string]any
	if err := r.db.Model(&model.SafetyIncident{}).
		Select("severity_level, COUNT(*) AS cnt").Group("severity_level").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("severity distribution: %w", err)
	}
	return rows, nil
}

// PendingRectification 待整改事件。
func (r *SafetyIncidentRepository) PendingRectification() ([]model.SafetyIncident, error) {
	var list []model.SafetyIncident
	if err := r.db.Where("status IN ?", []string{"reported", "investigating"}).
		Order("rectification_deadline ASC").Limit(10).Find(&list).Error; err != nil {
		return nil, fmt.Errorf("pending rectification: %w", err)
	}
	return list, nil
}

// PendingReview 待安全管理员复核的事件。
func (r *SafetyIncidentRepository) PendingReview() ([]model.SafetyIncident, error) {
	var list []model.SafetyIncident
	if err := r.db.Where("status = ?", constants.IncidentReviewPending).
		Order("reviewed_at IS NULL DESC, created_at ASC").Limit(10).Find(&list).Error; err != nil {
		return nil, fmt.Errorf("pending review: %w", err)
	}
	return list, nil
}
