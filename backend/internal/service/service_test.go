package service

import (
	"errors"
	"testing"

	"safetyplatform/internal/constants"
	"safetyplatform/internal/util"
)

func TestU64(t *testing.T) {
	if u64(0) != "0" || u64(123) != "123" {
		t.Error("u64 mismatch")
	}
}

func TestContains(t *testing.T) {
	if !contains(constants.UserRoleValues, constants.RoleInspector) {
		t.Error("inspector should be in roles")
	}
	if contains(constants.UserRoleValues, "bogus") {
		t.Error("bogus should not be in roles")
	}
}

func TestIncidentCategories(t *testing.T) {
	found := false
	for _, c := range constants.IncidentCategories {
		if c == "坠落" {
			found = true
		}
	}
	if !found {
		t.Error("坠落 should be in categories")
	}
}

// 驳回必须填写原因：空原因/纯空白原因在校验阶段直接失败。
func TestReviewRejectRequiresComment(t *testing.T) {
	for _, comment := range []string{"", "   "} {
		err := validateReview(1, false, comment)
		var appErr *util.AppError
		if !errors.As(err, &appErr) {
			t.Fatalf("expected AppError for comment %q, got %v", comment, err)
		}
		if appErr.Code != constants.CodeValidationFailed {
			t.Fatalf("expected validation failed code, got %d", appErr.Code)
		}
	}
	if err := validateReview(1, false, "防护栏未修复"); err != nil {
		t.Fatalf("reject with reason should pass: %v", err)
	}
	if err := validateReview(1, true, ""); err != nil {
		t.Fatalf("approve without comment should pass: %v", err)
	}
}

// 待复核必须是合法事件状态（复核闭环状态机的一环）。
func TestPendingReviewIsValidStatus(t *testing.T) {
	if !constants.IsValidIncidentStatus(constants.IncidentPendingReview) {
		t.Error("pending_review should be a valid incident status")
	}
}
