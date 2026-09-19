package service

// incidentTransitions 定义隐患状态机允许的迁移：
//
//	reported      --指派调查-->      investigating
//	investigating --提交整改-->      review_pending
//	review_pending--复核通过-->      closed
//	review_pending--复核驳回-->      investigating（退回整改，原措施与截止时间保留）
//	investigating --重新提交整改-->  review_pending
//	resolved(历史)--关闭-->          closed（兼容复核闭环上线前的旧数据）
var incidentTransitions = map[string]map[string]bool{
	"reported":       {"investigating": true},
	"investigating":  {"review_pending": true},
	"review_pending": {"closed": true, "investigating": true},
	"resolved":       {"closed": true},
}

// canTransitionIncident 判断状态迁移是否合法。
func canTransitionIncident(from, to string) bool {
	next, ok := incidentTransitions[from]
	if !ok {
		return false
	}
	return next[to]
}
