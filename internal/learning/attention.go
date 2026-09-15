package learning

import (
	"context"
	"net/http"
	"time"
)

// AttentionItem contains only the information needed by the dashboard.
// At is the deadline, start time, or oldest answer awaiting grading.
type AttentionItem struct {
	ID        uint64    `json:"id"`
	Kind      string    `json:"kind"`
	Status    string    `json:"status"`
	Title     string    `json:"title"`
	ClassName string    `json:"class_name"`
	Subject   string    `json:"subject"`
	At        time.Time `json:"at"`
	Count     int       `json:"count"`
}

// Attention is read on dashboard navigation only. SQL filters completed work
// before limiting the result, so no per-task requests or polling are needed.
func (s *Service) Attention(ctx context.Context, actor Actor) ([]AttentionItem, error) {
	var query string
	var args []any
	switch actor.Role {
	case "student":
		query = `SELECT id,kind,state,title,class_name,subject,at,n FROM (
 SELECT a.id,'assignments' kind,
 CASE WHEN (a.close_at IS NOT NULL AND a.close_at<=UTC_TIMESTAMP()) OR (a.due_at<UTC_TIMESTAMP() AND NOT a.allow_late) THEN 'closed'
 WHEN a.due_at<UTC_TIMESTAMP() THEN 'overdue' ELSE 'due' END state,
 a.title,c.title class_name,COALESCE(s.name,'Mata pelajaran') subject,a.due_at at,1 n,
 CASE WHEN a.due_at<UTC_TIMESTAMP() THEN 0 ELSE 2 END priority
 FROM assignments a JOIN classes c ON c.id=a.class_id LEFT JOIN subjects s ON s.id=a.subject_id
 WHERE a.deleted_at IS NULL AND a.status='published' AND c.status='active'
 AND a.due_at<=DATE_ADD(UTC_TIMESTAMP(),INTERVAL 7 DAY)
 AND EXISTS(SELECT 1 FROM class_members cm WHERE cm.class_id=c.id AND cm.student_user_id=? AND cm.status='active')
 AND NOT EXISTS(SELECT 1 FROM assignment_submissions sub WHERE sub.assignment_id=a.id AND sub.student_user_id=?)
 UNION ALL
 SELECT a.id,'assessments',IF(a.start_at<=UTC_TIMESTAMP(),'ongoing','scheduled'),a.title,
 GROUP_CONCAT(DISTINCT c.title ORDER BY c.title SEPARATOR ', '),COALESCE(s.name,'Mata pelajaran'),a.start_at,1,
 IF(a.start_at<=UTC_TIMESTAMP(),1,2)
 FROM assessments a JOIN assessment_targets t ON t.assessment_id=a.id JOIN classes c ON c.id=t.class_id
 LEFT JOIN subjects s ON s.id=a.subject_id
 WHERE a.deleted_at IS NULL AND a.status='published' AND c.status='active'
 AND a.start_at<=DATE_ADD(UTC_TIMESTAMP(),INTERVAL 7 DAY) AND a.end_at>UTC_TIMESTAMP()
 AND EXISTS(SELECT 1 FROM class_members cm WHERE cm.class_id=c.id AND cm.student_user_id=? AND cm.status='active')
 AND NOT EXISTS(SELECT 1 FROM assessment_attempts att WHERE att.assessment_id=a.id AND att.student_user_id=? AND att.status IN ('submitted','graded'))
 GROUP BY a.id,a.start_at,a.title,s.name
 ) reminders ORDER BY priority,at,id LIMIT 5`
		args = []any{actor.ID, actor.ID, actor.ID, actor.ID}
	case "teacher":
		query = `SELECT id,kind,state,title,class_name,subject,at,n FROM (
 SELECT a.id,'assignments' kind,'grading' state,a.title,c.title class_name,COALESCE(s.name,'Mata pelajaran') subject,MIN(sub.submitted_at) at,COUNT(*) n,0 priority
 FROM assignments a JOIN classes c ON c.id=a.class_id LEFT JOIN subjects s ON s.id=a.subject_id
 JOIN assignment_submissions sub ON sub.assignment_id=a.id AND sub.graded_at IS NULL
 WHERE a.teacher_user_id=? AND a.deleted_at IS NULL AND a.status IN ('published','closed') AND c.status='active'
 AND EXISTS(SELECT 1 FROM class_teachers ct WHERE ct.class_id=c.id AND ct.teacher_user_id=? AND ct.status='active' AND ct.subject_id=a.subject_id)
 GROUP BY a.id,a.title,c.title,s.name
 UNION ALL
 SELECT a.id,'assessments','grading',a.title,GROUP_CONCAT(DISTINCT c.title ORDER BY c.title SEPARATOR ', '),COALESCE(s.name,'Mata pelajaran'),MIN(att.submitted_at),COUNT(*),0
 FROM assessments a JOIN assessment_attempts att ON att.assessment_id=a.id AND att.status='submitted'
 JOIN classes c ON c.id=att.class_id LEFT JOIN subjects s ON s.id=a.subject_id
 WHERE a.teacher_user_id=? AND a.deleted_at IS NULL AND c.status='active'
 AND EXISTS(SELECT 1 FROM class_teachers ct WHERE ct.class_id=c.id AND ct.teacher_user_id=? AND ct.status='active' AND ct.subject_id=a.subject_id)
 GROUP BY a.id,a.title,s.name
 UNION ALL
 SELECT a.id,'assessments',IF(a.start_at<=UTC_TIMESTAMP(),'ongoing','scheduled'),a.title,GROUP_CONCAT(DISTINCT c.title ORDER BY c.title SEPARATOR ', '),COALESCE(s.name,'Mata pelajaran'),a.start_at,1,1
 FROM assessments a JOIN assessment_targets t ON t.assessment_id=a.id JOIN classes c ON c.id=t.class_id LEFT JOIN subjects s ON s.id=a.subject_id
 WHERE a.teacher_user_id=? AND a.deleted_at IS NULL AND a.status='published' AND c.status='active'
 AND a.start_at<=DATE_ADD(UTC_TIMESTAMP(),INTERVAL 7 DAY) AND a.end_at>UTC_TIMESTAMP()
 AND EXISTS(SELECT 1 FROM class_teachers ct WHERE ct.class_id=c.id AND ct.teacher_user_id=? AND ct.status='active' AND ct.subject_id=a.subject_id)
 GROUP BY a.id,a.start_at,a.title,s.name
 ) reminders ORDER BY priority,at,id LIMIT 5`
		args = []any{actor.ID, actor.ID, actor.ID, actor.ID, actor.ID, actor.ID}
	default:
		return nil, forbidden
	}
	rows, err := s.repository.q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []AttentionItem{}
	for rows.Next() {
		var item AttentionItem
		if err := rows.Scan(&item.ID, &item.Kind, &item.Status, &item.Title, &item.ClassName, &item.Subject, &item.At, &item.Count); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (h *Handler) Attention(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.Attention(r.Context(), actor(r))
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}
