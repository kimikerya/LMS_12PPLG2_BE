package learning

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"lms-website-be/internal/database"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const maxUploadFile = 10 << 20
const maxUploadTotal = 25 << 20

type Attachment struct {
	ID     uint64 `json:"id"`
	Name   string `json:"name"`
	URL    string `json:"url"`
	IsFile bool   `json:"is_file"`
}
type assignmentPackage struct {
	AssignmentInput
	Publish bool         `json:"publish"`
	Links   []Attachment `json:"links"`
}

var fileTypes = map[string]string{".pdf": "application/pdf", ".doc": "application/msword", ".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document", ".xls": "application/vnd.ms-excel", ".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", ".ppt": "application/vnd.ms-powerpoint", ".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation", ".odt": "application/vnd.oasis.opendocument.text", ".ods": "application/vnd.oasis.opendocument.spreadsheet", ".odp": "application/vnd.oasis.opendocument.presentation", ".zip": "application/zip", ".txt": "text/plain", ".csv": "text/csv", ".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg"}

func uploadRoot() string {
	if v := os.Getenv("LMS_UPLOAD_DIR"); v != "" {
		return v
	}
	return filepath.Join(".local", "uploads")
}
func validStoredKey(value string) bool {
	if !strings.HasPrefix(value, "local:") {
		return false
	}
	key := strings.TrimPrefix(value, "local:")
	if len(key) != 32 {
		return false
	}
	_, err := hex.DecodeString(key)
	return err == nil
}
func removeUploads(root string, files []Attachment) {
	for _, f := range files {
		if validStoredKey(f.URL) {
			_ = os.Remove(filepath.Join(root, strings.TrimPrefix(f.URL, "local:")))
		}
	}
}
func matchesFile(ext string, header []byte) bool {
	switch ext {
	case ".pdf":
		return strings.HasPrefix(string(header), "%PDF-")
	case ".png":
		return strings.HasPrefix(string(header), "\x89PNG\r\n\x1a\n")
	case ".jpg", ".jpeg":
		return len(header) > 2 && header[0] == 0xff && header[1] == 0xd8 && header[2] == 0xff
	case ".doc", ".xls", ".ppt":
		return strings.HasPrefix(string(header), "\xd0\xcf\x11\xe0\xa1\xb1\x1a\xe1")
	case ".txt", ".csv":
		return !strings.ContainsRune(string(header), '\x00')
	default:
		return strings.HasPrefix(string(header), "PK\x03\x04") || strings.HasPrefix(string(header), "PK\x05\x06")
	}
}
func receiveUploads(w http.ResponseWriter, r *http.Request, root string) (files []Attachment, err error) {
	files = []Attachment{}
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadTotal+(2<<20))
	err = r.ParseMultipartForm(1 << 20)
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	if err != nil {
		return nil, invalid("Unggahan tidak valid atau melebihi batas total 25 MB.")
	}
	headers := r.MultipartForm.File["files"]
	if len(headers) > 5 {
		return nil, invalid("Maksimal 5 file per pengiriman.")
	}
	for key := range r.MultipartForm.File {
		if key != "files" {
			return nil, invalid("Kolom unggahan tidak valid.")
		}
	}
	defer func() {
		if err != nil {
			removeUploads(root, files)
		}
	}()
	var total int64
	for _, h := range headers {
		name := filepath.Base(strings.ReplaceAll(h.Filename, "\\", "/"))
		ext := strings.ToLower(filepath.Ext(name))
		if fileTypes[ext] == "" || len(name) > 240 || strings.ContainsAny(name, "\r\n\x00") || !utf8.ValidString(name) {
			return files, invalid("Format file tidak didukung. Gunakan PDF, dokumen Office/OpenDocument, gambar JPG/PNG, TXT/CSV, atau ZIP.")
		}
		if h.Size <= 0 || h.Size > maxUploadFile {
			return files, invalid("File tidak boleh kosong dan maksimal 10 MB per file.")
		}
		total += h.Size
		if total > maxUploadTotal {
			return files, invalid("Total seluruh file maksimal 25 MB.")
		}
		source, e := h.Open()
		if e != nil {
			return files, e
		}
		header := make([]byte, 512)
		n, e := source.Read(header)
		if e != nil && e != io.EOF {
			source.Close()
			return files, e
		}
		if !matchesFile(ext, header[:n]) {
			source.Close()
			return files, invalid("Isi file tidak sesuai dengan format: " + name)
		}
		if _, e = source.Seek(0, io.SeekStart); e != nil {
			source.Close()
			return files, e
		}
		keyBytes := make([]byte, 16)
		if _, e = rand.Read(keyBytes); e != nil {
			source.Close()
			return files, e
		}
		key := hex.EncodeToString(keyBytes)
		if e = os.MkdirAll(root, 0700); e != nil {
			source.Close()
			return files, e
		}
		dest, e := os.OpenFile(filepath.Join(root, key), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			source.Close()
			return files, e
		}
		files = append(files, Attachment{Name: name, URL: "local:" + key, IsFile: true})
		nbytes, copyErr := io.Copy(dest, io.LimitReader(source, maxUploadFile+1))
		closeErr := dest.Close()
		source.Close()
		if copyErr != nil {
			return files, copyErr
		}
		if closeErr != nil {
			return files, closeErr
		}
		if nbytes > maxUploadFile {
			return files, invalid("File melebihi 10 MB.")
		}
	}
	return files, nil
}
func multipartMetadata(r *http.Request, in any) error {
	raw := r.FormValue("metadata")
	if len(raw) > 128*1024 {
		return invalid("Isian tugas terlalu panjang.")
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(in); err != nil {
		return invalid("Isian tugas atau jawaban tidak valid.")
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return invalid("Kirim satu objek metadata.")
	}
	return nil
}
func (s *Service) CreateAssignmentPackage(ctx context.Context, a Actor, in assignmentPackage, files []Attachment) (uint64, error) {
	if a.Role != "teacher" {
		return 0, forbidden
	}
	if len(in.Links) > 10 {
		return 0, invalid("Maksimal 10 tautan referensi.")
	}
	for i := range in.Links {
		v := &in.Links[i]
		v.Name = strings.TrimSpace(v.Name)
		if !validURL(&v.URL) || v.Name == "" || len(v.Name) > 240 {
			return 0, invalid("Isi judul dan tautan HTTP/HTTPS yang valid.")
		}
	}
	var id uint64
	err := s.repository.transact(ctx, func(q database.Querier) error {
		nested := NewService(&Repository{q: q, transact: func(_ context.Context, fn func(database.Querier) error) error { return fn(q) }})
		var err error
		id, err = nested.CreateAssignment(ctx, a, in.AssignmentInput)
		if err != nil {
			return err
		}
		for _, v := range append(files, in.Links...) {
			if _, err = q.ExecContext(ctx, "INSERT INTO assignment_attachments (assignment_id,file_name,file_url) VALUES (?,?,?)", id, v.Name, v.URL); err != nil {
				return err
			}
		}
		if in.Publish {
			if err = nested.Publish(ctx, a, "assignments", id); err != nil {
				return err
			}
		}
		return nil
	})
	return id, err
}
func (h *Handler) UploadAssignment(w http.ResponseWriter, r *http.Request) {
	root := uploadRoot()
	files, err := receiveUploads(w, r, root)
	if err != nil {
		respondError(w, err)
		return
	}
	var in assignmentPackage
	if err = multipartMetadata(r, &in); err != nil {
		removeUploads(root, files)
		respondError(w, err)
		return
	}
	id, err := h.service.CreateAssignmentPackage(r.Context(), actor(r), in, files)
	if err != nil {
		removeUploads(root, files)
		respondError(w, err)
		return
	}
	status := "draft"
	if in.Publish {
		status = "published"
	}
	writeJSON(w, 201, map[string]any{"id": id, "status": status})
}
func (h *Handler) UploadSubmission(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	root := uploadRoot()
	files, err := receiveUploads(w, r, root)
	if err != nil {
		respondError(w, err)
		return
	}
	var in SubmitInput
	if err = multipartMetadata(r, &in); err != nil {
		removeUploads(root, files)
		respondError(w, err)
		return
	}
	in.Files = files
	submissionID, err := h.service.Submit(r.Context(), actor(r), id, in)
	if err != nil {
		removeUploads(root, files)
		respondError(w, err)
		return
	}
	writeJSON(w, 201, map[string]any{"id": submissionID})
}
func attachmentLink(v Attachment, kind string) Attachment {
	if validStoredKey(v.URL) {
		v.IsFile = true
		v.URL = fmt.Sprintf("/api/learning-files/%s/%d", kind, v.ID)
	}
	return v
}
func (r *Repository) AssignmentAttachments(ctx context.Context, id uint64) ([]Attachment, error) {
	rows, err := r.q.QueryContext(ctx, "SELECT id,file_name,file_url FROM assignment_attachments WHERE assignment_id=? ORDER BY id", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Attachment{}
	for rows.Next() {
		var v Attachment
		if err = rows.Scan(&v.ID, &v.Name, &v.URL); err != nil {
			return nil, err
		}
		items = append(items, attachmentLink(v, "assignment"))
	}
	return items, rows.Err()
}
func (r *Repository) SubmissionFiles(ctx context.Context, assignmentID uint64, items []Submission) error {
	rows, err := r.q.QueryContext(ctx, `SELECT f.id,f.submission_id,f.file_name,f.file_url FROM submission_files f JOIN assignment_submissions s ON s.id=f.submission_id WHERE s.assignment_id=? ORDER BY f.id`, assignmentID)
	if err != nil {
		return err
	}
	defer rows.Close()
	index := map[uint64]int{}
	for i := range items {
		index[items[i].ID] = i
		items[i].Files = []Attachment{}
	}
	for rows.Next() {
		var v Attachment
		var sid uint64
		if err = rows.Scan(&v.ID, &sid, &v.Name, &v.URL); err != nil {
			return err
		}
		if i, ok := index[sid]; ok {
			items[i].Files = append(items[i].Files, attachmentLink(v, "submission"))
		}
	}
	return rows.Err()
}
func (h *Handler) DownloadFile(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		a := actor(r)
		var assignmentID, studentID uint64
		var name, stored string
		query := "SELECT assignment_id,0,file_name,file_url FROM assignment_attachments WHERE id=?"
		contentKind := "assignments"
		if kind == "material" {
			query = "SELECT material_id,0,file_name,file_url FROM material_attachments WHERE id=?"
			contentKind = "materials"
		}
		if kind == "submission" {
			query = "SELECT s.assignment_id,s.student_user_id,f.file_name,f.file_url FROM submission_files f JOIN assignment_submissions s ON s.id=f.submission_id WHERE f.id=?"
		}
		err := h.service.repository.q.QueryRowContext(r.Context(), query, id).Scan(&assignmentID, &studentID, &name, &stored)
		if errors.Is(err, sql.ErrNoRows) {
			err = notFound
		}
		if err != nil {
			respondError(w, err)
			return
		}
		if kind == "submission" && a.Role == "student" && studentID != a.ID {
			respondError(w, forbidden)
			return
		}
		visible, err := h.service.List(r.Context(), a, contentKind, Filter{Limit: 1}, assignmentID)
		if err != nil {
			respondError(w, err)
			return
		}
		if len(visible) == 0 {
			respondError(w, notFound)
			return
		}
		if !validStoredKey(stored) {
			respondError(w, notFound)
			return
		}
		file, err := os.Open(filepath.Join(uploadRoot(), strings.TrimPrefix(stored, "local:")))
		if err != nil {
			respondError(w, notFound)
			return
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			respondError(w, err)
			return
		}
		w.Header().Set("Content-Type", fileTypes[strings.ToLower(filepath.Ext(name))])
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("Content-Security-Policy", "sandbox")
		http.ServeContent(w, r, name, info.ModTime(), file)
	}
}
