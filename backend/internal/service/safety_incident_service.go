package service

import (
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
	ok, err := s.repo.TransitionStatus(id, constants.IncidentReported, map[string]any{
		"status": constants.IncidentInvestigating,
	})
	if err != nil {
		return nil, util.Wrap(err, "SafetyIncident[id=%d] assign save failed", id)
	}
	if !ok {
		return nil, s.statusConflict(id, "assign")
	}
	s.logger.Info(constants.LogIncidentAssignSuccess, "incident_id", id)
	return s.Get(id)
}

// SubmitRectification 提交整改：整改中 -> 待复核，等待安全管理员验收。
func (s *SafetyIncidentService) SubmitRectification(id uint64, measures string, deadline *time.Time) (*model.SafetyIncident, error) {
	ok, err := s.repo.TransitionStatus(id, constants.IncidentInvestigating, map[string]any{
		"status":                 constants.IncidentPendingReview,
		"rectification_measures": measures,
		"rectification_deadline": deadline,
	})
	if err != nil {
		return nil, util.Wrap(err, "SafetyIncident[id=%d] rectify save failed", id)
	}
	if !ok {
		return nil, s.statusConflict(id, "rectify")
	}
	s.logger.Info(constants.LogIncidentRectifySuccess, "incident_id", id)
	return s.Get(id)
}

// Review 整改复核：验收通过则关闭事件；驳回必须填写原因并退回整改中，原整改措施与截止时间保留。
// 通过 CAS 迁移保证重复验收或整改与验收并发时只允许一次状态迁移，失败请求不覆盖复核意见。
func (s *SafetyIncidentService) Review(id, reviewerID uint64, approved bool, comment string) (*model.SafetyIncident, error) {
	comment = strings.TrimSpace(comment)
	if err := validateReview(id, approved, comment); err != nil {
		return nil, err
	}
	now := time.Now()
	updates := map[string]any{
		"reviewer_id":    reviewerID,
		"reviewed_at":    now,
		"review_comment": comment,
	}
	if approved {
		updates["status"] = constants.IncidentClosed
		updates["review_result"] = constants.ReviewResultApproved
	} else {
		updates["status"] = constants.IncidentInvestigating
		updates["review_result"] = constants.ReviewResultRejected
	}
	ok, err := s.repo.TransitionStatus(id, constants.IncidentPendingReview, updates)
	if err != nil {
		return nil, util.Wrap(err, "SafetyIncident[id=%d] review save failed", id)
	}
	if !ok {
		s.logger.Warn(constants.LogIncidentReviewFailed, "incident_id", id, "reviewer_id", reviewerID)
		return nil, s.statusConflict(id, "review")
	}
	s.logger.Info(constants.LogIncidentReviewSuccess, "incident_id", id, "reviewer_id", reviewerID, "approved", approved)
	return s.Get(id)
}

// statusConflict 构造状态冲突错误，并带上当前状态便于排查。
func (s *SafetyIncidentService) statusConflict(id uint64, action string) error {
	cur, err := s.repo.FindByID(id)
	if err != nil {
		return util.Wrap(err, "SafetyIncident[id=%d] "+action+" find failed", id)
	}
	return util.NewAppError(constants.CodeIncidentStatusConflict,
		"SafetyIncident[id="+u64(id)+"] "+action+" conflict: status="+cur.Status)
}

// validateReview 校验复核请求：驳回必须填写原因。
func validateReview(id uint64, approved bool, comment string) error {
	if !approved && strings.TrimSpace(comment) == "" {
		return util.NewAppError(constants.CodeValidationFailed, "SafetyIncident[id="+u64(id)+"] review reject: "+constants.MsgReviewCommentRequired)
	}
	return nil
}

// List 分页查询事件。
func (s *SafetyIncidentService) List(page, pageSize int, severity, status string, startDate, endDate *time.Time) ([]model.SafetyIncident, int64, error) {
	list, total, err := s.repo.List(page, pageSize, severity, status, startDate, endDate)
	if err != nil {
		return nil, 0, err
	}
	s.fillReviewerNames(list)
	return list, total, nil
}

// Get 事件详情。
func (s *SafetyIncidentService) Get(id uint64) (*model.SafetyIncident, error) {
	i, err := s.repo.FindByID(id)
	if err != nil {
		return nil, err
	}
	s.fillReviewerName(i)
	return i, nil
}

// fillReviewerName 填充单个事件的复核人姓名。
func (s *SafetyIncidentService) fillReviewerName(i *model.SafetyIncident) {
	if i.ReviewerID == 0 {
		return
	}
	if u, err := s.userRepo.FindByID(i.ReviewerID); err == nil {
		i.ReviewerName = u.Name
	}
}

// fillReviewerNames 批量填充列表的复核人姓名。
func (s *SafetyIncidentService) fillReviewerNames(list []model.SafetyIncident) {
	seen := map[uint64]bool{}
	var ids []uint64
	for _, it := range list {
		if it.ReviewerID > 0 && !seen[it.ReviewerID] {
			seen[it.ReviewerID] = true
			ids = append(ids, it.ReviewerID)
		}
	}
	if len(ids) == 0 {
		return
	}
	users, err := s.userRepo.FindByIDs(ids)
	if err != nil {
		s.logger.Warn("fill reviewer names failed", "error", err.Error())
		return
	}
	names := make(map[uint64]string, len(users))
	for _, u := range users {
		names[u.ID] = u.Name
	}
	for idx := range list {
		list[idx].ReviewerName = names[list[idx].ReviewerID]
	}
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
