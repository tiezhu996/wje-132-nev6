package service

import (
	"errors"
	"log/slog"
	"strings"
	"time"

	"safetyplatform/internal/constants"
	"safetyplatform/internal/model"
	"safetyplatform/internal/repository"
	"safetyplatform/internal/util"
)

// SafetyIncidentService 安全事件业务逻辑。
type SafetyIncidentService struct {
	repo     *repository.SafetyIncidentRepository
	userRepo *repository.UserRepository
	logger   *slog.Logger
}

// NewSafetyIncidentService 构造安全事件服务。
func NewSafetyIncidentService(repo *repository.SafetyIncidentRepository, userRepo *repository.UserRepository, logger *slog.Logger) *SafetyIncidentService {
	return &SafetyIncidentService{repo: repo, userRepo: userRepo, logger: logger}
}

// Report 上报事件。
func (s *SafetyIncidentService) Report(reporterID uint64, title, description string, occurredAt time.Time,
	siteID, area, severity, category string, involvedUserIDs, photoURLs []string) (*model.SafetyIncident, error) {
	if !constants.IsValidSeverity(severity) {
		return nil, util.NewAppError(constants.CodeValidationFailed, "SafetyIncident[severity="+severity+"] report: invalid severity")
	}
	i := &model.SafetyIncident{
		Title: title, Description: description, OccurredAt: occurredAt, SiteID: siteID, Area: area,
		SeverityLevel: severity, Category: category,
		InvolvedUserIDs: model.JSONList(involvedUserIDs), PhotoURLs: model.JSONList(photoURLs),
		Status: constants.IncidentReported, ReporterID: reporterID,
	}
	if err := s.repo.Create(i); err != nil {
		s.logger.Error(constants.LogIncidentReportFailed, "error", err.Error())
		return nil, util.Wrap(err, "SafetyIncident[title=%s] report create failed", title)
	}
	s.logger.Info(constants.LogIncidentReportSuccess, "incident_id", i.ID)
	return i, nil
}

// Assign 指派调查。
func (s *SafetyIncidentService) Assign(id uint64) (*model.SafetyIncident, error) {
	i, err := s.repo.FindByID(id)
	if err != nil {
		return nil, util.Wrap(err, "SafetyIncident[id=%d] assign find failed", id)
	}
	if !canTransitionIncident(i.Status, constants.IncidentInvestigating) {
		return nil, util.NewAppError(constants.CodeIncidentStatusConflict, "SafetyIncident[id="+u64(id)+"] assign conflict: status="+i.Status)
	}
	out, err := s.transition(id, i.Status, map[string]any{"status": constants.IncidentInvestigating}, constants.LogIncidentAssignSuccess, "assign")
	if err != nil {
		return nil, err
	}
	return out, nil
}

// SubmitRectification 提交整改：提交后进入待复核（review_pending）。
// 驳回后重新提交时，若未重新填写措施/截止时间则保留原值，仅执行状态迁移。
func (s *SafetyIncidentService) SubmitRectification(id uint64, measures string, deadline *time.Time) (*model.SafetyIncident, error) {
	i, err := s.repo.FindByID(id)
	if err != nil {
		return nil, util.Wrap(err, "SafetyIncident[id=%d] rectify find failed", id)
	}
	if !canTransitionIncident(i.Status, constants.IncidentReviewPending) {
		return nil, util.NewAppError(constants.CodeIncidentStatusConflict, "SafetyIncident[id="+u64(id)+"] rectify conflict: status="+i.Status)
	}
	measures = strings.TrimSpace(measures)
	// 首次提交必须有整改措施；驳回后重新提交允许留空，留空则保留原措施。
	if measures == "" && strings.TrimSpace(i.RectificationMeasures) == "" {
		return nil, util.NewAppError(constants.CodeValidationFailed,
			"SafetyIncident[id="+u64(id)+"] rectify failed: rectification_measures required")
	}
	fields := map[string]any{"status": constants.IncidentReviewPending}
	if measures != "" {
		fields["rectification_measures"] = measures
	}
	if deadline != nil {
		fields["rectification_deadline"] = deadline
	}
	// 进入待复核时清理上一轮的复核痕迹，避免旧意见误导本次复核；整改措施与截止时间不在此清除。
	fields["review_result"] = ""
	fields["review_comment"] = ""
	fields["reviewer_id"] = 0
	fields["reviewer_name"] = ""
	fields["reviewed_at"] = nil
	out, err := s.transition(id, i.Status, fields, constants.LogIncidentRectifySuccess, "rectify")
	if err != nil {
		return nil, err
	}
	s.logger.Info(constants.LogIncidentRectifySuccess, "incident_id", id)
	return out, nil
}

// Review 安全管理员复核整改。approved=true 验收通过并关闭；approved=false 驳回，
// 必须填写驳回原因，退回整改中，原整改措施与截止时间保留。
func (s *SafetyIncidentService) Review(id, reviewerID uint64, reviewerName string, approved bool, comment string) (*model.SafetyIncident, error) {
	comment = strings.TrimSpace(comment)
	if !approved && comment == "" {
		return nil, util.NewAppError(constants.CodeReviewCommentRequired,
			"SafetyIncident[id="+u64(id)+"] review reject failed: review_comment required")
	}
	i, err := s.repo.FindByID(id)
	if err != nil {
		return nil, util.Wrap(err, "SafetyIncident[id=%d] review find failed", id)
	}
	if i.Status != constants.IncidentReviewPending {
		return nil, util.NewAppError(constants.CodeIncidentStatusConflict, "SafetyIncident[id="+u64(id)+"] review conflict: status="+i.Status+", expect=review_pending")
	}
	targetStatus := constants.IncidentClosed
	result := constants.ReviewApproved
	logTpl := constants.LogIncidentReviewSuccess
	if !approved {
		targetStatus = constants.IncidentInvestigating
		result = constants.ReviewRejected
		logTpl = constants.LogIncidentReviewRejected
	}
	now := time.Now()
	fields := map[string]any{
		"status":         targetStatus,
		"review_result":  result,
		"review_comment": comment,
		"reviewer_id":    reviewerID,
		"reviewer_name":  reviewerName,
		"reviewed_at":    now,
	}
	out, err := s.transition(id, i.Status, fields, logTpl, "review")
	if err != nil {
		return nil, err
	}
	s.logger.Info(logTpl, "incident_id", id, "reviewer_id", reviewerID, "review_result", result)
	return out, nil
}

// Close 关闭事件（仅兼容复核闭环上线前 status=resolved 的历史数据）。
func (s *SafetyIncidentService) Close(id uint64) (*model.SafetyIncident, error) {
	i, err := s.repo.FindByID(id)
	if err != nil {
		return nil, util.Wrap(err, "SafetyIncident[id=%d] close find failed", id)
	}
	if !canTransitionIncident(i.Status, constants.IncidentClosed) {
		return nil, util.NewAppError(constants.CodeIncidentStatusConflict, "SafetyIncident[id="+u64(id)+"] close conflict: status="+i.Status)
	}
	out, err := s.transition(id, i.Status, map[string]any{"status": constants.IncidentClosed}, constants.LogIncidentCloseSuccess, "close")
	if err != nil {
		return nil, err
	}
	return out, nil
}

// transition 执行原子条件状态迁移，将仓储层并发冲突转换为业务错误。
func (s *SafetyIncidentService) transition(id uint64, expectStatus string, fields map[string]any, logTpl, action string) (*model.SafetyIncident, error) {
	out, err := s.repo.TransitionStatus(id, expectStatus, fields)
	if err != nil {
		if errors.Is(err, repository.ErrStatusConflict) {
			s.logger.Warn(constants.LogIncidentStatusChangeFailed, "incident_id", id, "action", action, "expect", expectStatus)
			return nil, util.NewAppError(constants.CodeIncidentReviewConflict,
				"SafetyIncident[id="+u64(id)+"] "+action+" conflict: status already moved (duplicate or concurrent request)")
		}
		s.logger.Error(constants.LogIncidentStatusChangeFailed, "incident_id", id, "action", action, "error", err.Error())
		return nil, util.Wrap(err, "SafetyIncident[id=%d] %s transition failed", id, action)
	}
	s.logger.Info(logTpl, "incident_id", id)
	return out, nil
}

// ResolveReviewerName 查询复核人姓名。
func (s *SafetyIncidentService) ResolveReviewerName(reviewerID uint64) string {
	if reviewerID == 0 {
		return ""
	}
	u, err := s.userRepo.FindByID(reviewerID)
	if err != nil {
		return ""
	}
	return u.Name
}

// List 分页查询事件。
func (s *SafetyIncidentService) List(page, pageSize int, severity, status string, startDate, endDate *time.Time) ([]model.SafetyIncident, int64, error) {
	return s.repo.List(page, pageSize, severity, status, startDate, endDate)
}

// Get 事件详情。
func (s *SafetyIncidentService) Get(id uint64) (*model.SafetyIncident, error) {
	return s.repo.FindByID(id)
}

// Trend30 近 30 天趋势。
func (s *SafetyIncidentService) Trend30() ([]map[string]any, error) {
	return s.repo.Trend30()
}

// SeverityDistribution 严重等级分布。
func (s *SafetyIncidentService) SeverityDistribution() ([]map[string]any, error) {
	return s.repo.SeverityDistribution()
}

// PendingRectification 待整改列表。
func (s *SafetyIncidentService) PendingRectification() ([]model.SafetyIncident, error) {
	return s.repo.PendingRectification()
}

// PendingReview 待复核列表。
func (s *SafetyIncidentService) PendingReview() ([]model.SafetyIncident, error) {
	return s.repo.PendingReview()
}

func u64(v uint64) string {
	if v == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	return string(b[i:])
}
