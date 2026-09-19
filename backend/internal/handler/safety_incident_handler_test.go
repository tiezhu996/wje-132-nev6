package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"safetyplatform/internal/config"
	"safetyplatform/internal/constants"
	"safetyplatform/internal/middleware"
	"safetyplatform/internal/model"
	"safetyplatform/internal/repository"
	"safetyplatform/internal/service"
	"safetyplatform/internal/util"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// reviewFlowEnv 复核闭环接口测试环境。
type reviewFlowEnv struct {
	engine *gin.Engine
	db     *gorm.DB
	cfg    *config.Config
}

func newReviewFlowEnv(t *testing.T) *reviewFlowEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.SafetyIncident{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	users := []model.User{
		{Phone: "13800000001", Name: "系统管理员", Role: constants.RoleAdmin},
		{Phone: "13800000002", Name: "王安全", Role: constants.RoleSafetyManager},
		{Phone: "13800000004", Name: "赵工", Role: constants.RoleWorker},
	}
	for i := range users {
		if err := db.Create(&users[i]).Error; err != nil {
			t.Fatalf("seed user: %v", err)
		}
	}
	cfg := &config.Config{JWTSecret: "review-flow-test-secret"}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := service.NewSafetyIncidentService(repository.NewSafetyIncidentRepository(db), repository.NewUserRepository(db), logger)
	h := NewSafetyIncidentHandler(svc, logger)

	engine := gin.New()
	g := engine.Group("/api/v1/incidents")
	g.Use(middleware.AuthRequired(cfg))
	g.GET("/:id", h.Get)
	g.POST("/:id/assign", middleware.RequireRole(constants.RoleAdmin, constants.RoleSafetyManager), h.Assign)
	g.POST("/:id/rectify", middleware.RequireRole(constants.RoleAdmin, constants.RoleSafetyManager), h.Rectify)
	g.POST("/:id/review", middleware.RequireRole(constants.RoleSafetyManager), h.Review)
	return &reviewFlowEnv{engine: engine, db: db, cfg: cfg}
}

func (e *reviewFlowEnv) token(t *testing.T, userID uint64, phone, role string) string {
	t.Helper()
	token, err := util.GenerateToken(e.cfg.JWTSecret, time.Hour, userID, phone, role)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	return token
}

func (e *reviewFlowEnv) call(t *testing.T, method, path, token string, body any) (int, map[string]any) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	e.engine.ServeHTTP(w, req)
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response %q: %v", w.Body.String(), err)
	}
	return w.Code, resp
}

func (e *reviewFlowEnv) seedIncident(t *testing.T, status string) uint64 {
	t.Helper()
	deadline := time.Now().Add(72 * time.Hour).Truncate(time.Second)
	i := model.SafetyIncident{
		Title: "临边防护缺失", OccurredAt: time.Now(), SeverityLevel: constants.SeverityMajor,
		Category: "坠落", Status: status, ReporterID: 3,
		RectificationMeasures: "加装防护栏杆", RectificationDeadline: &deadline,
	}
	if err := e.db.Create(&i).Error; err != nil {
		t.Fatalf("seed incident: %v", err)
	}
	return i.ID
}

func incidentOf(t *testing.T, resp map[string]any) map[string]any {
	t.Helper()
	data, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatalf("response missing data: %v", resp)
	}
	return data
}

// 完整复核闭环：整改 -> 待复核 -> 驳回(必须填原因) -> 退回整改中(措施保留) -> 再整改 -> 验收通过关闭。
func TestIncidentReviewClosedLoop(t *testing.T) {
	env := newReviewFlowEnv(t)
	manager := env.token(t, 2, "13800000002", constants.RoleSafetyManager)
	id := env.seedIncident(t, constants.IncidentInvestigating)

	// 提交整改 -> 待复核
	code, resp := env.call(t, http.MethodPost, "/api/v1/incidents/"+itoa(id)+"/rectify", manager,
		map[string]any{"measures": "已加装防护栏杆并验收自检", "deadline": "2026-09-25T10:00:00Z"})
	if code != http.StatusOK {
		t.Fatalf("rectify: code=%d resp=%v", code, resp)
	}
	if got := incidentOf(t, resp)["status"]; got != constants.IncidentPendingReview {
		t.Fatalf("after rectify status=%v, want pending_review", got)
	}

	// 驳回但不填原因 -> 422，状态不变
	code, _ = env.call(t, http.MethodPost, "/api/v1/incidents/"+itoa(id)+"/review", manager,
		map[string]any{"approved": false, "comment": ""})
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("reject without comment: code=%d, want 422", code)
	}
	if got := env.currentStatus(t, id); got != constants.IncidentPendingReview {
		t.Fatalf("status after invalid reject=%q, want pending_review", got)
	}

	// 填原因驳回 -> 退回整改中，原措施与截止时间保留
	code, resp = env.call(t, http.MethodPost, "/api/v1/incidents/"+itoa(id)+"/review", manager,
		map[string]any{"approved": false, "comment": "防护栏未固定牢固"})
	if code != http.StatusOK {
		t.Fatalf("reject: code=%d resp=%v", code, resp)
	}
	data := incidentOf(t, resp)
	if got := data["status"]; got != constants.IncidentInvestigating {
		t.Fatalf("after reject status=%v, want investigating", got)
	}
	if got := data["review_result"]; got != constants.ReviewResultRejected {
		t.Fatalf("review_result=%v, want rejected", got)
	}
	if got := data["review_comment"]; got != "防护栏未固定牢固" {
		t.Fatalf("review_comment=%v", got)
	}
	if got := data["reviewer_name"]; got != "王安全" {
		t.Fatalf("reviewer_name=%v, want 王安全", got)
	}
	if got := data["rectification_measures"]; got != "已加装防护栏杆并验收自检" {
		t.Fatalf("measures not preserved: %v", got)
	}
	if data["rectification_deadline"] == nil || data["rectification_deadline"] == "" {
		t.Fatal("deadline not preserved after reject")
	}

	// 再次提交整改 -> 待复核
	code, _ = env.call(t, http.MethodPost, "/api/v1/incidents/"+itoa(id)+"/rectify", manager,
		map[string]any{"measures": "重新固定防护栏并复验"})
	if code != http.StatusOK {
		t.Fatalf("re-rectify: code=%d", code)
	}

	// 验收通过 -> 关闭
	code, resp = env.call(t, http.MethodPost, "/api/v1/incidents/"+itoa(id)+"/review", manager,
		map[string]any{"approved": true, "comment": "现场复核合格"})
	if code != http.StatusOK {
		t.Fatalf("approve: code=%d", code)
	}
	data = incidentOf(t, resp)
	if got := data["status"]; got != constants.IncidentClosed {
		t.Fatalf("after approve status=%v, want closed", got)
	}
	if got := data["review_result"]; got != constants.ReviewResultApproved {
		t.Fatalf("review_result=%v, want approved", got)
	}
}

// 只有安全管理员能验收：工人/管理员调用复核接口均被拒绝。
func TestIncidentReviewRBAC(t *testing.T) {
	env := newReviewFlowEnv(t)
	id := env.seedIncident(t, constants.IncidentPendingReview)
	worker := env.token(t, 4, "13800000004", constants.RoleWorker)
	admin := env.token(t, 1, "13800000001", constants.RoleAdmin)

	for name, token := range map[string]string{"worker": worker, "admin": admin} {
		code, _ := env.call(t, http.MethodPost, "/api/v1/incidents/"+itoa(id)+"/review", token,
			map[string]any{"approved": true})
		if code != http.StatusForbidden {
			t.Fatalf("%s review: code=%d, want 403", name, code)
		}
	}
	if got := env.currentStatus(t, id); got != constants.IncidentPendingReview {
		t.Fatalf("status=%q after forbidden reviews, want pending_review", got)
	}
}

// 重复验收/并发冲突：只允许一次状态迁移，失败请求不得覆盖复核意见。
func TestIncidentReviewDuplicateConflict(t *testing.T) {
	env := newReviewFlowEnv(t)
	manager := env.token(t, 2, "13800000002", constants.RoleSafetyManager)
	id := env.seedIncident(t, constants.IncidentPendingReview)

	code, _ := env.call(t, http.MethodPost, "/api/v1/incidents/"+itoa(id)+"/review", manager,
		map[string]any{"approved": true, "comment": "首次验收意见"})
	if code != http.StatusOK {
		t.Fatalf("first approve: code=%d", code)
	}

	// 重复验收 -> 409，且复核意见保持首次内容
	code, _ = env.call(t, http.MethodPost, "/api/v1/incidents/"+itoa(id)+"/review", manager,
		map[string]any{"approved": false, "comment": "迟到驳回"})
	if code != http.StatusConflict {
		t.Fatalf("duplicate review: code=%d, want 409", code)
	}
	_, resp := env.call(t, http.MethodGet, "/api/v1/incidents/"+itoa(id), manager, nil)
	data := incidentOf(t, resp)
	if got := data["status"]; got != constants.IncidentClosed {
		t.Fatalf("status=%v, want closed", got)
	}
	if got := data["review_comment"]; got != "首次验收意见" {
		t.Fatalf("review comment overwritten by failed request: %v", got)
	}
	if got := data["review_result"]; got != constants.ReviewResultApproved {
		t.Fatalf("review result overwritten by failed request: %v", got)
	}

	// 已关闭事件再提交整改 -> 409
	code, _ = env.call(t, http.MethodPost, "/api/v1/incidents/"+itoa(id)+"/rectify", manager,
		map[string]any{"measures": "重复整改"})
	if code != http.StatusConflict {
		t.Fatalf("rectify on closed: code=%d, want 409", code)
	}
}

// 指派调查：仅 reported 可迁移，重复指派冲突。
func TestIncidentAssignConflict(t *testing.T) {
	env := newReviewFlowEnv(t)
	manager := env.token(t, 2, "13800000002", constants.RoleSafetyManager)
	id := env.seedIncident(t, constants.IncidentReported)

	code, resp := env.call(t, http.MethodPost, "/api/v1/incidents/"+itoa(id)+"/assign", manager, nil)
	if code != http.StatusOK {
		t.Fatalf("assign: code=%d", code)
	}
	if got := incidentOf(t, resp)["status"]; got != constants.IncidentInvestigating {
		t.Fatalf("after assign status=%v, want investigating", got)
	}
	code, _ = env.call(t, http.MethodPost, "/api/v1/incidents/"+itoa(id)+"/assign", manager, nil)
	if code != http.StatusConflict {
		t.Fatalf("duplicate assign: code=%d, want 409", code)
	}
}

func (e *reviewFlowEnv) currentStatus(t *testing.T, id uint64) string {
	t.Helper()
	var i model.SafetyIncident
	if err := e.db.First(&i, id).Error; err != nil {
		t.Fatalf("reload incident: %v", err)
	}
	return i.Status
}

func itoa(v uint64) string {
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
