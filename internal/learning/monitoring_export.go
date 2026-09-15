package learning

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

func xmlText(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(strings.Map(func(r rune) rune {
		if r < 32 && r != '\n' && r != '\t' && r != '\r' {
			return -1
		}
		return r
	}, s)))
	return b.String()
}

type workbookSheet struct {
	name string
	rows [][]any
}

func makeWorkbook(sheets []workbookSheet) ([]byte, error) {
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	write := func(path, body string) error {
		w, e := z.Create(path)
		if e != nil {
			return e
		}
		_, e = w.Write([]byte(body))
		return e
	}
	header := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`
	types := header + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>`
	workbook := header + `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets>`
	rels := header + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`
	for i, s := range sheets {
		n := i + 1
		types += fmt.Sprintf(`<Override PartName="/xl/worksheets/sheet%d.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>`, n)
		workbook += fmt.Sprintf(`<sheet name="%s" sheetId="%d" r:id="rId%d"/>`, xmlText(s.name), n, n)
		rels += fmt.Sprintf(`<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet%d.xml"/>`, n, n)
		var data strings.Builder
		data.WriteString(header + `<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetViews><sheetView workbookViewId="0"><pane ySplit="1" topLeftCell="A2" activePane="bottomLeft" state="frozen"/></sheetView></sheetViews><sheetData>`)
		for rowIndex, row := range s.rows {
			fmt.Fprintf(&data, `<row r="%d">`, rowIndex+1)
			for column, cell := range row {
				ref := ""
				for n := column + 1; n > 0; n = (n - 1) / 26 {
					ref = string(rune('A'+(n-1)%26)) + ref
				}
				ref += strconv.Itoa(rowIndex + 1)
				switch v := cell.(type) {
				case int:
					fmt.Fprintf(&data, "<c r=\"%s\"><v>%d</v></c>", ref, v)
				case float64:
					data.WriteString("<c r=\"" + ref + "\"><v>" + strconv.FormatFloat(v, 'f', 2, 64) + "</v></c>")
				default:
					data.WriteString(`<c r="` + ref + `" t="inlineStr"><is><t xml:space="preserve">` + xmlText(fmt.Sprint(v)) + `</t></is></c>`)
				}
			}
			data.WriteString("</row>")
		}
		data.WriteString("</sheetData></worksheet>")
		if e := write(fmt.Sprintf("xl/worksheets/sheet%d.xml", n), data.String()); e != nil {
			return nil, e
		}
	}
	for path, body := range map[string]string{"[Content_Types].xml": types + "</Types>", "xl/workbook.xml": workbook + "</sheets></workbook>", "xl/_rels/workbook.xml.rels": rels + "</Relationships>", "_rels/.rels": header + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`} {
		if e := write(path, body); e != nil {
			return nil, e
		}
	}
	if e := z.Close(); e != nil {
		return nil, e
	}
	return b.Bytes(), nil
}
func monitoringWorkbook(out MonitorReport) ([]byte, error) {
	students := [][]any{{"Nama siswa", "ID / NIS", "Kelas", "Mapel", "Guru aktif", "Materi terbit", "Materi dibuka", "Materi layak dipantau ≥7 hari", "Tugas", "Dikumpulkan", "Tugas lewat tenggat belum masuk", "Terlambat", "Asesmen", "Asesmen dikumpulkan", "Menunggu penilaian", "Rata-rata nilai dirilis (0–100)", "Alasan peninjauan"}}
	teachers := [][]any{{"Nama guru", "NIP / ID pengguna", "Kelas", "Mapel", "Materi terbit", "Tugas terbit", "Asesmen terjadwal", "Jawaban tugas masuk", "Jawaban asesmen masuk", "Jawaban dinilai", "Menunggu penilaian", "Tugas siswa belum masuk lewat tenggat", "Konten dihapus"}}
	details := [][]any{{"Siswa", "Kelas", "Mapel", "Jenis", "ID konten", "Judul", "Status"}}
	for _, v := range out.Students {
		var average any = ""
		if v.Average != nil {
			average = *v.Average
		}
		students = append(students, []any{v.Name, v.Identifier, v.Class, v.Subject, v.Teachers, v.Materials, v.Opened, v.Trackable, v.Tasks, v.Submitted, v.Overdue, v.Late, v.Exams, v.ExamDone, v.PendingGrade, average, strings.Join(v.Reasons, "; ")})
		for _, e := range v.Evidence {
			details = append(details, []any{v.Name, v.Class, v.Subject, e.Kind, strconv.FormatUint(e.ID, 10), e.Title, e.Status})
		}
	}
	for _, v := range out.Teachers {
		teachers = append(teachers, []any{v.Name, v.Identifier, v.Class, v.Subject, v.Materials, v.Tasks, v.Exams, v.Submitted, v.ExamDone, v.Graded, v.PendingGrade, v.Overdue, v.Deleted})
	}
	reviews := [][]any{{"Pengguna", "ID kelas", "ID mapel", "Status", "Catatan", "Dicatat oleh", "Waktu WIB"}}
	for _, v := range out.Reviews {
		reviews = append(reviews, []any{v.Name, strconv.FormatUint(v.ClassID, 10), strconv.FormatUint(v.SubjectID, 10), v.Status, v.Note, v.Author, v.At.In(schoolZone).Format("2006-01-02 15:04")})
	}
	days := [][]any{{"Tanggal WIB", "Konten terbit / asesmen dimulai per kelas", "Jawaban dikumpulkan", "Materi pertama dibuka"}}
	for _, v := range out.Days {
		days = append(days, []any{v.Day, v.Published, v.Submitted, v.Opened})
	}
	info := [][]any{{"Keterangan", "Nilai"}, {"Periode", out.From + " sampai " + out.To}, {"Dibuat pada WIB", out.AsOf.In(schoolZone).Format("2006-01-02 15:04")}, {"Tracking materi dimulai WIB", out.TrackingSince.In(schoolZone).Format("2006-01-02 15:04")}, {"Cakupan", "Anggota dan penugasan mapel aktif saat laporan dibuat. Konten dalam periode terpilih, sejak siswa bergabung; asesmen berdasarkan waktu mulai. Konten dihapus tidak menjadi kewajiban siswa."}, {"Akses materi", "Membuka halaman materi bukan bukti membaca atau memahami. Materi sebelum tracking tidak memicu penanda. Masa tunggu 7 hari."}, {"Perlu ditinjau", "Minimal 3 tugas lewat tenggat belum masuk, atau kurang dari 50% materi layak dipantau dibuka. Penanda membantu peninjauan, bukan vonis."}, {"Nilai", "Rata-rata sederhana nilai yang sudah dirilis, dinormalisasi per tugas/asesmen ke 0–100. Nilai kosong tidak dianggap nol."}, {"Aktivitas guru", "Jumlah konten dan penilaian bukan ukuran tunggal kualitas mengajar; bandingkan konteks kelas, beban dan periode."}}
	return makeWorkbook([]workbookSheet{{"Keterangan", info}, {"Siswa", students}, {"Guru", teachers}, {"Rincian siswa", details}, {"Tren harian", days}, {"Tindak lanjut", reviews}})
}
func (h *Handler) MonitoringExport(w http.ResponseWriter, r *http.Request) {
	f, err := monitorParams(r)
	if err != nil {
		respondError(w, err)
		return
	}
	out, err := h.service.MonitoringReport(r.Context(), actor(r), f)
	if err != nil {
		respondError(w, err)
		return
	}
	data, err := monitoringWorkbook(out)
	if err != nil {
		respondError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="laporan-aktivitas.xlsx"`)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Write(data)
}
