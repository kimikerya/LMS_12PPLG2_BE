package learning

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"lms-website-be/internal/database"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var schoolZone = time.FixedZone("WIB", 7*3600)

type MonitorFilter struct {
	From, To time.Time
	ClassID  uint64
}
type MonitorRow struct {
	ReviewStatus string            `json:"review_status"`
	UserID       uint64            `json:"user_id"`
	ClassID      uint64            `json:"class_id"`
	SubjectID    uint64            `json:"subject_id"`
	Name         string            `json:"name"`
	Identifier   string            `json:"identifier"`
	Class        string            `json:"class_name"`
	Subject      string            `json:"subject"`
	Teachers     string            `json:"teachers"`
	Materials    int               `json:"materials"`
	Opened       int               `json:"opened"`
	Trackable    int               `json:"trackable"`
	Tasks        int               `json:"tasks"`
	Submitted    int               `json:"submitted"`
	Overdue      int               `json:"overdue"`
	Late         int               `json:"late"`
	Graded       int               `json:"graded"`
	Exams        int               `json:"exams"`
	ExamDone     int               `json:"exam_done"`
	PendingGrade int               `json:"pending_grade"`
	Deleted      int               `json:"deleted"`
	Average      *float64          `json:"average"`
	Reasons      []string          `json:"reasons"`
	Evidence     []MonitorEvidence `json:"evidence"`
	joined       time.Time
	sum          float64
	scored       int
	unopened     int
}
type MonitorEvidence struct {
	Kind   string `json:"kind"`
	ID     uint64 `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
}
type MonitorDay struct {
	Day       string `json:"day"`
	Published int    `json:"published"`
	Submitted int    `json:"submitted"`
	Opened    int    `json:"opened"`
}
type MonitorReview struct {
	ID        uint64    `json:"id"`
	UserID    uint64    `json:"user_id"`
	ClassID   uint64    `json:"class_id"`
	SubjectID uint64    `json:"subject_id"`
	Name      string    `json:"name"`
	Author    string    `json:"author"`
	Status    string    `json:"status"`
	Note      string    `json:"note"`
	At        time.Time `json:"at"`
}
type MonitorReport struct {
	From          string          `json:"from"`
	To            string          `json:"to"`
	AsOf          time.Time       `json:"as_of"`
	TrackingSince time.Time       `json:"tracking_since"`
	Students      []*MonitorRow   `json:"students"`
	Teachers      []*MonitorRow   `json:"teachers"`
	Days          []*MonitorDay   `json:"days"`
	Reviews       []MonitorReview `json:"reviews"`
}

func monitorKey(user, class, subject uint64) string {
	return fmt.Sprintf("%d/%d/%d", user, class, subject)
}
func monitorParams(r *http.Request) (MonitorFilter, error) {
	now := time.Now().In(schoolZone)
	f := MonitorFilter{}
	from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
	if from == "" {
		from = now.AddDate(0, 0, -29).Format("2006-01-02")
	}
	if to == "" {
		to = now.Format("2006-01-02")
	}
	var err error
	f.From, err = time.ParseInLocation("2006-01-02", from, schoolZone)
	if err != nil {
		return f, invalid("Tanggal awal tidak valid.")
	}
	f.To, err = time.ParseInLocation("2006-01-02", to, schoolZone)
	if err != nil {
		return f, invalid("Tanggal akhir tidak valid.")
	}
	f.To = f.To.AddDate(0, 0, 1)
	if !f.To.After(f.From) || f.To.Sub(f.From) > 366*24*time.Hour || f.From.After(now) {
		return f, invalid("Pilih rentang tanggal maksimal 366 hari dan tidak dimulai di masa depan.")
	}
	if raw := r.URL.Query().Get("class_id"); raw != "" {
		f.ClassID, err = strconv.ParseUint(raw, 10, 64)
		if err != nil || f.ClassID == 0 {
			return f, invalid("Kelas tidak valid.")
		}
	}
	return f, nil
}

type monitorContent struct {
	kind                        string
	id, class, subject, teacher uint64
	title                       string
	published                   time.Time
	due                         *time.Time
	max                         float64
	deleted                     bool
}
type monitorAnswer struct {
	at       time.Time
	score    *float64
	released *time.Time
	graded   bool
	late     bool
}

func (s *Service) MonitoringReport(ctx context.Context, a Actor, f MonitorFilter) (MonitorReport, error) {
	out := MonitorReport{From: f.From.In(schoolZone).Format("2006-01-02"), To: f.To.Add(-time.Second).In(schoolZone).Format("2006-01-02"), AsOf: time.Now().UTC(), Students: []*MonitorRow{}, Teachers: []*MonitorRow{}, Days: []*MonitorDay{}, Reviews: []MonitorReview{}}
	if a.Role != "curriculum" && a.Role != "principal" {
		return out, forbidden
	}
	q := s.repository.q
	if err := q.QueryRowContext(ctx, "SELECT applied_at FROM schema_migrations WHERE version='044_material_access.sql'").Scan(&out.TrackingSince); err != nil {
		return out, err
	}
	cutoff := f.To.UTC()
	if cutoff.After(out.AsOf) {
		cutoff = out.AsOf
	}
	dayMap := map[string]*MonitorDay{}
	for d := f.From; d.Before(f.To) && !d.After(out.AsOf); d = d.AddDate(0, 0, 1) {
		v := &MonitorDay{Day: d.Format("2006-01-02")}
		out.Days = append(out.Days, v)
		dayMap[v.Day] = v
	}
	classClause := ""
	var args []any
	if f.ClassID != 0 {
		classClause = " AND c.id=?"
		args = append(args, f.ClassID)
	}
	rows, err := q.QueryContext(ctx, `SELECT cm.student_user_id,c.id,ct.subject_id,u.full_name,COALESCE(up.nis,u.login_id),c.title,s.name,GROUP_CONCAT(DISTINCT tu.full_name ORDER BY tu.full_name SEPARATOR ', '),cm.joined_at FROM class_members cm JOIN classes c ON c.id=cm.class_id JOIN users u ON u.id=cm.student_user_id LEFT JOIN student_profiles up ON up.user_id=u.id JOIN class_teachers ct ON ct.class_id=c.id AND ct.status='active' AND ct.subject_id IS NOT NULL JOIN users tu ON tu.id=ct.teacher_user_id JOIN subjects s ON s.id=ct.subject_id WHERE cm.status='active' AND c.status='active' AND u.deleted_at IS NULL`+classClause+` GROUP BY cm.student_user_id,c.id,ct.subject_id,u.full_name,up.nis,u.login_id,c.title,s.name,cm.joined_at ORDER BY c.title,u.full_name,s.name`, args...)
	if err != nil {
		return out, err
	}
	students := map[string]*MonitorRow{}
	byClassSubject := map[string][]*MonitorRow{}
	for rows.Next() {
		v := &MonitorRow{Reasons: []string{}, Evidence: []MonitorEvidence{}}
		if err = rows.Scan(&v.UserID, &v.ClassID, &v.SubjectID, &v.Name, &v.Identifier, &v.Class, &v.Subject, &v.Teachers, &v.joined); err != nil {
			rows.Close()
			return out, err
		}
		out.Students = append(out.Students, v)
		students[monitorKey(v.UserID, v.ClassID, v.SubjectID)] = v
		k := monitorKey(0, v.ClassID, v.SubjectID)
		byClassSubject[k] = append(byClassSubject[k], v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	rows, err = q.QueryContext(ctx, `SELECT DISTINCT u.id,c.id,s.id,u.full_name,COALESCE(tp.employee_id,u.login_id),c.title,s.name FROM class_teachers ct JOIN users u ON u.id=ct.teacher_user_id LEFT JOIN teacher_profiles tp ON tp.user_id=u.id JOIN classes c ON c.id=ct.class_id JOIN subjects s ON s.id=ct.subject_id WHERE ct.status='active' AND c.status='active' AND u.deleted_at IS NULL`+classClause+` ORDER BY c.title,u.full_name,s.name`, args...)
	if err != nil {
		return out, err
	}
	teachers := map[string]*MonitorRow{}
	for rows.Next() {
		v := &MonitorRow{Reasons: []string{}, Evidence: []MonitorEvidence{}}
		if err = rows.Scan(&v.UserID, &v.ClassID, &v.SubjectID, &v.Name, &v.Identifier, &v.Class, &v.Subject); err != nil {
			rows.Close()
			return out, err
		}
		out.Teachers = append(out.Teachers, v)
		teachers[monitorKey(v.UserID, v.ClassID, v.SubjectID)] = v
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	// The cohort contains published content in the selected period, including closed work.
	contentSQL := `SELECT 'materials',m.id,m.class_id,COALESCE(m.subject_id,0),m.teacher_user_id,m.title,m.published_at,NULL,0,m.deleted_at IS NOT NULL FROM materials m WHERE m.status='published'
 UNION ALL SELECT 'assignments',a.id,a.class_id,COALESCE(a.subject_id,0),a.teacher_user_id,a.title,a.published_at,a.due_at,COALESCE(a.max_points,100),a.deleted_at IS NOT NULL FROM assignments a WHERE a.status IN ('published','closed')
 UNION ALL SELECT 'assessments',e.id,t.class_id,COALESCE(e.subject_id,0),e.teacher_user_id,e.title,COALESCE(e.start_at,e.created_at),e.end_at,COALESCE((SELECT SUM(points) FROM assessment_questions WHERE assessment_id=e.id),0),e.deleted_at IS NOT NULL FROM assessments e JOIN assessment_targets t ON t.assessment_id=e.id WHERE e.status IN ('published','closed')`
	rows, err = q.QueryContext(ctx, "SELECT * FROM ("+contentSQL+") content WHERE published_at>=? AND published_at<? AND (?=0 OR class_id=?)", f.From.UTC(), cutoff, f.ClassID, f.ClassID)
	if err != nil {
		return out, err
	}
	contents := []monitorContent{}
	for rows.Next() {
		var v monitorContent
		if err = rows.Scan(&v.kind, &v.id, &v.class, &v.subject, &v.teacher, &v.title, &v.published, &v.due, &v.max, &v.deleted); err != nil {
			rows.Close()
			return out, err
		}
		contents = append(contents, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	answers := map[string]monitorAnswer{}
	rows, err = q.QueryContext(ctx, `SELECT 'assignments',s.assignment_id,s.student_user_id,a.class_id,s.submitted_at,s.score,s.result_released_at,COALESCE(s.graded_at<?,FALSE),COALESCE(s.submitted_late,s.submitted_at>a.due_at,FALSE) FROM assignment_submissions s JOIN assignments a ON a.id=s.assignment_id WHERE a.published_at>=? AND a.published_at<? AND s.submitted_at<? AND (?=0 OR a.class_id=?) UNION ALL SELECT 'assessments',s.assessment_id,s.student_user_id,s.class_id,s.submitted_at,s.final_score,s.result_released_at,COALESCE(s.graded_at<?,FALSE),FALSE FROM assessment_attempts s JOIN assessments a ON a.id=s.assessment_id WHERE COALESCE(a.start_at,a.created_at)>=? AND COALESCE(a.start_at,a.created_at)<? AND s.submitted_at<? AND (?=0 OR s.class_id=?)`, cutoff, f.From.UTC(), cutoff, cutoff, f.ClassID, f.ClassID, cutoff, f.From.UTC(), cutoff, cutoff, f.ClassID, f.ClassID)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var kind string
		var id, user, class uint64
		var v monitorAnswer
		if err = rows.Scan(&kind, &id, &user, &class, &v.at, &v.score, &v.released, &v.graded, &v.late); err != nil {
			rows.Close()
			return out, err
		}
		key := kind + "/" + monitorKey(id, user, class)
		if old, ok := answers[key]; !ok || v.at.After(old.at) {
			answers[key] = v
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	opened := map[string]time.Time{}
	rows, err = q.QueryContext(ctx, `SELECT ma.material_id,ma.student_user_id,ma.first_opened_at FROM material_access ma JOIN materials m ON m.id=ma.material_id WHERE m.published_at>=? AND m.published_at<? AND ma.first_opened_at<? AND (?=0 OR m.class_id=?)`, f.From.UTC(), cutoff, cutoff, f.ClassID, f.ClassID)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var id, user uint64
		var at time.Time
		if err = rows.Scan(&id, &user, &at); err != nil {
			rows.Close()
			return out, err
		}
		opened[monitorKey(id, user, 0)] = at
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	for _, v := range contents {
		teacher := teachers[monitorKey(v.teacher, v.class, v.subject)]
		if teacher != nil {
			if v.deleted {
				teacher.Deleted++
			} else {
				teacher.Evidence = append(teacher.Evidence, MonitorEvidence{v.kind, v.id, v.title, "Konten pembelajaran pada periode ini"})
				switch v.kind {
				case "materials":
					teacher.Materials++
				case "assignments":
					teacher.Tasks++
				case "assessments":
					teacher.Exams++
				}
			}
		}
		if v.deleted {
			continue
		}
		if day := dayMap[v.published.In(schoolZone).Format("2006-01-02")]; day != nil {
			day.Published++
		}
		for _, student := range byClassSubject[monitorKey(0, v.class, v.subject)] {
			if v.published.Before(student.joined) {
				continue
			}
			status := "Belum dikumpulkan"
			if v.kind == "materials" {
				student.Materials++
				at, ok := opened[monitorKey(v.id, student.UserID, 0)]
				materialStatus := "Belum tercatat dibuka · belum masuk penanda akses"
				if ok {
					materialStatus = "Dibuka pada " + at.In(schoolZone).Format("02/01/2006 15:04 WIB")
					student.Opened++
					if day := dayMap[at.In(schoolZone).Format("2006-01-02")]; day != nil {
						day.Opened++
					}
				}
				if !v.published.Before(out.TrackingSince) && v.published.Add(7*24*time.Hour).Before(cutoff) {
					student.Trackable++
					if !ok {
						student.unopened++
						materialStatus = "Belum tercatat dibuka (terbit ≥7 hari)"
					}
				}
				student.Evidence = append(student.Evidence, MonitorEvidence{v.kind, v.id, v.title, materialStatus})
				continue
			}
			answer, done := answers[v.kind+"/"+monitorKey(v.id, student.UserID, v.class)]
			if v.kind == "assignments" {
				student.Tasks++
				if done {
					student.Submitted++
					if teacher != nil {
						teacher.Submitted++
					}
					if answer.late {
						student.Late++
						if teacher != nil {
							teacher.Late++
						}
					}
				} else if v.due != nil && v.due.Before(cutoff) {
					student.Overdue++
					if teacher != nil {
						teacher.Overdue++
					}
					status = "Lewat tenggat, belum dikumpulkan"
				}
			} else {
				student.Exams++
				if done {
					student.ExamDone++
					if teacher != nil {
						teacher.ExamDone++
					}
				}
			}
			if done {
				status = "Dikumpulkan"
				if day := dayMap[answer.at.In(schoolZone).Format("2006-01-02")]; day != nil {
					day.Submitted++
				}
				if answer.graded {
					student.Graded++
					if teacher != nil {
						teacher.Graded++
					}
				} else {
					student.PendingGrade++
					if teacher != nil {
						teacher.PendingGrade++
					}
				}
				if answer.score != nil && answer.released != nil && answer.released.Before(cutoff) && v.max > 0 {
					student.sum += *answer.score / v.max * 100
					student.scored++
					status = "Nilai dirilis"
				}
			}
			student.Evidence = append(student.Evidence, MonitorEvidence{v.kind, v.id, v.title, status})
		}
	}
	for _, v := range out.Students {
		if v.scored > 0 {
			avg := v.sum / float64(v.scored)
			v.Average = &avg
		}
		if v.Overdue >= 3 {
			v.Reasons = append(v.Reasons, fmt.Sprintf("%d tugas lewat tenggat belum dikumpulkan", v.Overdue))
		}
		unopened := v.unopened
		if v.Trackable > 0 && float64(v.Trackable-unopened)/float64(v.Trackable) < 0.5 {
			v.Reasons = append(v.Reasons, fmt.Sprintf("%d dari %d materi terpantau belum dibuka (terbit ≥7 hari)", unopened, v.Trackable))
		}
	}
	rows, err = q.QueryContext(ctx, `SELECT r.id,r.target_user_id,r.class_id,r.subject_id,u.full_name,a.full_name,r.status,r.note,r.created_at FROM monitoring_reviews r JOIN users u ON u.id=r.target_user_id JOIN users a ON a.id=r.author_user_id WHERE r.created_at<? AND (?=0 OR r.class_id=?) ORDER BY r.created_at DESC,r.id DESC`, cutoff, f.ClassID, f.ClassID)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var v MonitorReview
		if err = rows.Scan(&v.ID, &v.UserID, &v.ClassID, &v.SubjectID, &v.Name, &v.Author, &v.Status, &v.Note, &v.At); err != nil {
			rows.Close()
			return out, err
		}
		out.Reviews = append(out.Reviews, v)
		key := monitorKey(v.UserID, v.ClassID, v.SubjectID)
		if row := students[key]; row != nil && row.ReviewStatus == "" {
			row.ReviewStatus = v.Status
		}
		if row := teachers[key]; row != nil && row.ReviewStatus == "" {
			row.ReviewStatus = v.Status
		}
	}
	err = rows.Err()
	rows.Close()
	return out, err
}

func (h *Handler) Monitoring(w http.ResponseWriter, r *http.Request) {
	f, err := monitorParams(r)
	if err != nil {
		respondError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	out, err := h.service.MonitoringReport(ctx, actor(r), f)
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, 200, out)
}
func (s *Service) RecordMaterialAccess(ctx context.Context, a Actor, id uint64) error {
	if a.Role != "student" {
		return forbidden
	}
	return s.repository.transact(ctx, func(q database.Querier) error {
		var class uint64
		var status string
		err := q.QueryRowContext(ctx, "SELECT class_id,status FROM materials WHERE id=? AND deleted_at IS NULL FOR UPDATE", id).Scan(&class, &status)
		if errors.Is(err, sql.ErrNoRows) {
			return notFound
		}
		if err != nil {
			return err
		}
		if status != "published" {
			return notFound
		}
		if err = requireMember(ctx, q, a.ID, class); err != nil {
			return err
		}
		_, err = q.ExecContext(ctx, "INSERT INTO material_access(material_id,student_user_id) VALUES(?,?) ON DUPLICATE KEY UPDATE last_opened_at=UTC_TIMESTAMP()", id, a.ID)
		return err
	})
}
func (h *Handler) MaterialAccess(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := h.service.RecordMaterialAccess(r.Context(), actor(r), id); err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"recorded": true})
}

type MonitorReviewInput struct {
	UserID    uint64 `json:"user_id"`
	ClassID   uint64 `json:"class_id"`
	SubjectID uint64 `json:"subject_id"`
	Status    string `json:"status"`
	Note      string `json:"note"`
}

func (s *Service) AddMonitorReview(ctx context.Context, a Actor, in MonitorReviewInput) error {
	if a.Role != "curriculum" {
		return forbidden
	}
	in.Note = strings.TrimSpace(in.Note)
	if in.Note == "" || len(in.Note) > 4000 || (in.Status != "review" && in.Status != "monitor" && in.Status != "resolved") {
		return invalid("Isi catatan maksimal 4000 karakter dan pilih status tindak lanjut.")
	}
	return s.repository.transact(ctx, func(q database.Querier) error {
		if err := lockClass(ctx, q, in.ClassID); err != nil {
			return err
		}
		var eligible bool
		err := q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM class_teachers ct WHERE ct.class_id=? AND ct.subject_id=? AND ct.status='active' AND (ct.teacher_user_id=? OR EXISTS(SELECT 1 FROM class_members cm WHERE cm.class_id=ct.class_id AND cm.student_user_id=? AND cm.status='active')))`, in.ClassID, in.SubjectID, in.UserID, in.UserID).Scan(&eligible)
		if err != nil {
			return err
		}
		if !eligible {
			return invalid("Pengguna tidak berada pada kelas dan mapel ini.")
		}
		id, err := insertID(ctx, q, "INSERT INTO monitoring_reviews(target_user_id,class_id,subject_id,author_user_id,status,note) VALUES(?,?,?,?,?,?)", in.UserID, in.ClassID, in.SubjectID, a.ID, in.Status, in.Note)
		if err != nil {
			return err
		}
		return audit(ctx, q, a, "review", "monitoring_reviews", id)
	})
}
func (h *Handler) MonitorReview(w http.ResponseWriter, r *http.Request) {
	var in MonitorReviewInput
	if !decode(w, r, &in) {
		return
	}
	if err := h.service.AddMonitorReview(r.Context(), actor(r), in); err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, 201, map[string]bool{"saved": true})
}
