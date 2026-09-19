package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"safetyplatform/internal/constants"
	"safetyplatform/internal/dto"
	"safetyplatform/internal/middleware"
	"safetyplatform/internal/service"
	"safetyplatform/internal/util"

	"github.com/gin-gonic/gin"
)

// SafetyIncidentHandler 安全事件 HTTP 处理器。
type SafetyIncidentHandler struct {
	svc    *service.SafetyIncidentService
	logger *slog.Logger
}

// NewSafetyIncidentHandler 构造安全事件处理器。
func NewSafetyIncidentHandler(svc *service.SafetyIncidentService, logger *slog.Logger) *SafetyIncidentHandler {
	return &SafetyIncidentHandler{svc: svc, logger: logger}
}

// List 事件列表。
func (h *SafetyIncidentHandler) List(c *gin.Context) {
	var q dto.PageQuery
	_ = c.ShouldBindQuery(&q)
	q.Normalize()
	var startDate, endDate *time.Time
	if s := c.Query("start_date"); s != "" {
		if t, err := time.Parse("2006-01-02", s); err == nil {
			startDate = &t
		}
	}
	if s := c.Query("end_date"); s != "" {
		if t, err := time.Parse("2006-01-02", s); err == nil {
			endDate = &t
		}
	}
	list, total, err := h.svc.List(q.Page, q.PageSize, c.Query("severity"), c.Query("status"), startDate, endDate)
	if err != nil {
		h.wrapError(c, err, "SafetyIncident list failed")
		return
	}
	OK(c, pageResponse(list, total, q.Page, q.PageSize))
}

// Get 事件详情。
func (h *SafetyIncidentHandler) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		Fail(c, http.StatusBadRequest, constants.CodeBadRequest, "SafetyIncident[id] get: invalid id")
		return
	}
	inc, err := h.svc.Get(id)
	if err != nil {
		h.wrapError(c, err, "SafetyIncident get failed")
		return
	}
	OK(c, inc)
}

// Report 上报事件。
func (h *SafetyIncidentHandler) Report(c *gin.Context) {
	var req dto.IncidentReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, constants.CodeBadRequest, "SafetyIncident report: "+err.Error())
		return
	}
	inc, err := h.svc.Report(middleware.GetUserID(c), req.Title, req.Description, req.OccurredAt,
		req.SiteID, req.Area, req.SeverityLevel, req.Category, req.InvolvedUserIDs, req.PhotoURLs)
	if err != nil {
		h.wrapError(c, err, "SafetyIncident[title="+req.Title+"] report failed")
		return
	}
	OKWithMessage(c, constants.MsgIncidentReported, inc)
}

// Assign 指派调查。
func (h *SafetyIncidentHandler) Assign(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		Fail(c, http.StatusBadRequest, constants.CodeBadRequest, "SafetyIncident[id] assign: invalid id")
		return
	}
	inc, err := h.svc.Assign(id)
	if err != nil {
		h.wrapError(c, err, "SafetyIncident assign failed")
		return
	}
	OK(c, inc)
}

// Rectify 提交整改。
func (h *SafetyIncidentHandler) Rectify(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		Fail(c, http.StatusBadRequest, constants.CodeBadRequest, "SafetyIncident[id] rectify: invalid id")
		return
	}
	var req dto.RectificationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, constants.CodeBadRequest, "SafetyIncident[id="+strconv.FormatUint(id, 10)+"] rectify: "+err.Error())
		return
	}
	inc, err := h.svc.SubmitRectification(id, req.Measures, req.Deadline)
	if err != nil {
		h.wrapError(c, err, "SafetyIncident rectify failed")
		return
	}
	OKWithMessage(c, constants.MsgIncidentRectified, inc)
}

// Review 隐患整改复核（安全管理员验收）。
func (h *SafetyIncidentHandler) Review(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		Fail(c, http.StatusBadRequest, constants.CodeBadRequest, "SafetyIncident[id] review: invalid id")
		return
	}
	var req dto.IncidentReviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, constants.CodeBadRequest, "SafetyIncident[id="+strconv.FormatUint(id, 10)+"] review: "+err.Error())
		return
	}
	reviewerID := middleware.GetUserID(c)
	reviewerName := h.resolveReviewerName(reviewerID, middleware.GetPhone(c))
	inc, err := h.svc.Review(id, reviewerID, reviewerName, req.Approved, req.Comment)
	if err != nil {
		h.wrapError(c, err, "SafetyIncident review failed: role="+middleware.GetRole(c))
		return
	}
	msg := constants.MsgIncidentReviewPassed
	if !req.Approved {
		msg = constants.MsgIncidentReviewRejected
	}
	OKWithMessage(c, msg, inc)
}

// resolveReviewerName 复核人姓名优先取用户资料，缺失时回退为手机号。
func (h *SafetyIncidentHandler) resolveReviewerName(reviewerID uint64, fallback string) string {
	if name := h.svc.ResolveReviewerName(reviewerID); name != "" {
		return name
	}
	return fallback
}

// Close 关闭事件。
func (h *SafetyIncidentHandler) Close(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		Fail(c, http.StatusBadRequest, constants.CodeBadRequest, "SafetyIncident[id] close: invalid id")
		return
	}
	inc, err := h.svc.Close(id)
	if err != nil {
		h.wrapError(c, err, "SafetyIncident close failed")
		return
	}
	OKWithMessage(c, constants.MsgIncidentClosed, inc)
}

func (h *SafetyIncidentHandler) wrapError(c *gin.Context, err error, ctx string) {
	var appErr *util.AppError
	if errors.As(err, &appErr) {
		c.Set("audit_detail", appErr.Message)
		h.logger.Warn("incident handler error", "context", ctx, "error", appErr.Error())
		Fail(c, appErrorStatus(appErr.Code), appErr.Code, appErr.Message)
		return
	}
	h.logger.Error("incident handler error", "context", ctx, "error", err.Error())
	Fail(c, http.StatusInternalServerError, constants.CodeInternalError, constants.MsgInternalError)
}
