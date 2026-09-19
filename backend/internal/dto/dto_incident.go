package dto

import "time"

// IncidentReportRequest 上报事件请求。
type IncidentReportRequest struct {
	Title           string    `json:"title" binding:"required,max=200"`
	Description     string    `json:"description"`
	OccurredAt      time.Time `json:"occurred_at" binding:"required"`
	SiteID          string    `json:"site_id" binding:"max=50"`
	Area            string    `json:"area" binding:"max=100"`
	SeverityLevel   string    `json:"severity_level" binding:"required,oneof=near_miss minor moderate major fatal"`
	Category        string    `json:"category" binding:"max=50"`
	InvolvedUserIDs []string  `json:"involved_user_ids"`
	PhotoURLs       []string  `json:"photo_urls"`
}

// RectificationRequest 整改请求。驳回后重新提交时措施与截止时间可留空，留空则保留原值。
type RectificationRequest struct {
	Measures string     `json:"measures"`
	Deadline *time.Time `json:"deadline"`
}

// IncidentReviewRequest 隐患整改复核请求。
// approved=true 验收通过并关闭；approved=false 驳回，comment 必填驳回原因。
type IncidentReviewRequest struct {
	Approved bool   `json:"approved"`
	Comment  string `json:"comment" binding:"max=500"`
}
