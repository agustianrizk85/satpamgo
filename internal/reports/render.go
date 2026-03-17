package reports

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/phpdave11/gofpdf"
	"golang.org/x/image/webp"
)

func renderCSV(headers []string, rows [][]string) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.Write(headers); err != nil {
		return nil, err
	}
	for _, row := range rows {
		if err := w.Write(row); err != nil {
			return nil, err
		}
	}
	w.Flush()
	return buf.Bytes(), w.Error()
}

func renderSimplePDF(title string, headers []string, rows [][]string, summary []string) ([]byte, error) {
	pdf := gofpdf.New("L", "mm", "A4", "")
	pdf.SetMargins(8, 8, 8)
	pdf.SetAutoPageBreak(true, 8)
	pdf.AddPage()
	pdf.SetFont("Arial", "B", 14)
	pdf.CellFormat(0, 8, title, "", 1, "L", false, 0, "")
	pdf.SetFont("Arial", "", 9)
	for _, line := range summary {
		pdf.CellFormat(0, 5, line, "", 1, "L", false, 0, "")
	}
	pdf.Ln(2)

	widths := make([]float64, len(headers))
	totalWidth := 281.0
	base := totalWidth / float64(len(headers))
	for i := range widths {
		widths[i] = base
	}

	pdf.SetFont("Arial", "B", 8)
	for i, header := range headers {
		pdf.CellFormat(widths[i], 7, header, "1", 0, "C", false, 0, "")
	}
	pdf.Ln(-1)

	pdf.SetFont("Arial", "", 7)
	for _, row := range rows {
		for i, cell := range row {
			pdf.CellFormat(widths[i], 6, truncateCell(cell, 32), "1", 0, "L", false, 0, "")
		}
		pdf.Ln(-1)
	}

	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

type attendancePDFInput struct {
	Title       string
	PlaceName   string
	FromDate    string
	ToDate      string
	GeneratedBy string
	Rows        []AttendanceReportRow
	StorageRoot string
}

type patrolPDFInput struct {
	Title       string
	PlaceName   string
	FromDate    string
	ToDate      string
	GeneratedBy string
	Rows        []PatrolScanReportRow
	StorageRoot string
}

func renderAttendancePDF(input attendancePDFInput) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(10, 10, 10)
	pdf.SetAutoPageBreak(true, 10)

	renderAttendanceCoverPage(pdf, input)
	renderAttendanceIntroPage(pdf, input)
	renderAttendanceDetailPages(pdf, input)

	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func renderPatrolPDF(input patrolPDFInput) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(10, 10, 10)
	pdf.SetAutoPageBreak(true, 10)

	renderReportCoverPage(pdf, strings.ToUpper(strings.ReplaceAll(input.Title, " ", "\n")), input.PlaceName, input.FromDate, input.ToDate)
	renderReportIntroPage(pdf, input.PlaceName, input.FromDate, input.ToDate, "Laporan Pekerjaan Patrol Scan", input.GeneratedBy)
	renderPatrolDetailPages(pdf, input)

	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func renderAttendanceCoverPage(pdf *gofpdf.Fpdf, input attendancePDFInput) {
	renderReportCoverPage(pdf, strings.ToUpper(strings.TrimSpace(input.Title)), input.PlaceName, input.FromDate, input.ToDate)
}

func renderReportCoverPage(pdf *gofpdf.Fpdf, title, placeName, fromDate, toDate string) {
	pdf.AddPage()
	pdf.SetFillColor(20, 53, 90)
	pdf.Rect(0, 0, 210, 297, "F")
	pdf.SetFillColor(235, 239, 245)
	pdf.Polygon([]gofpdf.PointType{{X: 122, Y: 297}, {X: 210, Y: 0}, {X: 210, Y: 160}}, "F")
	pdf.SetDrawColor(11, 36, 63)
	pdf.SetLineWidth(3)
	pdf.Line(95, 297, 210, 182)

	pdf.SetFillColor(26, 61, 100)
	pdf.SetAlpha(0.95, "Normal")
	pdf.RoundedRect(12, 22, 122, 58, 6, "1234", "F")
	pdf.SetAlpha(1, "Normal")

	pdf.SetTextColor(255, 255, 255)
	pdf.SetFont("Arial", "B", 22)
	lines := strings.Split(strings.TrimSpace(title), "\n")
	y := 38.0
	for _, line := range lines {
		pdf.Text(18, y, strings.TrimSpace(line))
		y += 11
	}
	pdf.SetFont("Arial", "", 11)
	pdf.Text(18, y-1, "AZKA Smart Secure System")
	pdf.SetFont("Arial", "B", 10)
	pdf.Text(18, y+12, "Site/Area : "+safeText(placeName, "Semua Place"))
	pdf.Text(18, y+22, "Tanggal : "+formatDateRange(fromDate, toDate))

	pdf.SetTextColor(255, 255, 255)
	pdf.SetAlpha(0.08, "Normal")
	pdf.TransformBegin()
	pdf.TransformRotate(52, 80, 170)
	pdf.SetFont("Arial", "B", 24)
	pdf.Text(18, 170, "AZKA SMART SECURE SYSTEM")
	pdf.TransformEnd()
	pdf.SetAlpha(1, "Normal")

	pdf.SetFillColor(255, 255, 255)
	pdf.SetDrawColor(190, 198, 209)
	pdf.RoundedRect(108, 252, 86, 28, 4, "1234", "FD")
	pdf.SetTextColor(90, 100, 112)
	pdf.SetFont("Arial", "B", 6)
	pdf.Text(113, 258, "POWERED BY")
	pdf.SetTextColor(34, 44, 150)
	pdf.SetFont("Arial", "B", 18)
	pdf.Text(113, 271, "AZKA")
	pdf.SetTextColor(220, 55, 49)
	pdf.SetFont("Arial", "B", 13)
	pdf.Text(145, 271, "SMART")
	pdf.SetFont("Arial", "B", 8)
	pdf.Text(145, 276, "SECURE SYSTEM")
}

func renderAttendanceIntroPage(pdf *gofpdf.Fpdf, input attendancePDFInput) {
	renderReportIntroPage(pdf, input.PlaceName, input.FromDate, input.ToDate, "Laporan Pekerjaan Absensi", input.GeneratedBy)
}

func renderReportIntroPage(pdf *gofpdf.Fpdf, placeName, fromDate, toDate, subject, generatedBy string) {
	pdf.AddPage()
	addAttendanceWatermark(pdf)
	pdf.SetTextColor(60, 69, 82)
	pdf.SetFont("Arial", "", 11)
	pdf.Text(18, 28, "Site/ Area  : "+safeText(placeName, "Semua Place"))
	pdf.Text(18, 38, "Perihal/ Subject: "+subject)

	pdf.SetFont("Arial", "B", 12)
	pdf.Text(18, 56, "Definisi/ Definition")
	pdf.SetFont("Arial", "", 10)
	writeParagraph(pdf, 18, 64, 170, 6, "Laporan harian ini adalah laporan yang kami kirimkan atas pelaksanaan pekerjaan yang telah kami lakukan pada hari ini. Laporan berisikan beberapa gambar dan penjelasan yang dapat kami laporkan. Dengan laporan ini kami berharap dapat menggambarkan sebagian rutinitas yang kami lakukan pada hari ini.")
	writeParagraph(pdf, 18, 102, 170, 6, "This daily report is the report we have submitted on the implementation of the work we have done today. The report contains several images and explanations that we can report. With this report we hope to describe some of the routines we do today.")
	pdf.SetFont("Arial", "", 11)
	pdf.Text(18, 142, "Tanggal/ Date: "+formatDateRange(fromDate, toDate))

	writeParagraph(pdf, 18, 236, 175, 6, "Laporan ini diproses melalui sistem AZKA Smart Secure System dimana hasil laporan ini adalah valid dan tidak membutuhkan tandatangan dari pelapor.")
	writeParagraph(pdf, 18, 256, 175, 6, "This report is processed through the AZKA Smart Secure System where the results of this report are valid and do not require the signature from our end.")
	pdf.SetTextColor(120, 126, 136)
	pdf.SetFont("Arial", "", 7)
	pdf.Text(135, 283, "Generated by: "+safeText(generatedBy, "-"))
}

func renderAttendanceDetailPages(pdf *gofpdf.Fpdf, input attendancePDFInput) {
	const cardsPerPage = 3
	for start := 0; start < len(input.Rows); start += cardsPerPage {
		pdf.AddPage()
		addAttendanceWatermark(pdf)
		for i := 0; i < cardsPerPage; i++ {
			idx := start + i
			if idx >= len(input.Rows) {
				break
			}
			y := 10.0 + float64(i)*76.0
			renderAttendanceCard(pdf, input.Rows[idx], input.StorageRoot, 10, y, 190, 70)
		}
	}
}

func renderPatrolDetailPages(pdf *gofpdf.Fpdf, input patrolPDFInput) {
	groups := groupPatrolRows(input.Rows)
	for _, group := range groups {
		pdf.AddPage()
		renderPatrolGroupFrame(pdf, group.title)

		currentY := 20.0
		for _, row := range group.rows {
			blockHeight := measurePatrolScanBlock(pdf, row, 186)
			if currentY+blockHeight > 272 {
				pdf.AddPage()
				renderPatrolGroupFrame(pdf, group.title)
				currentY = 20
			}
			renderPatrolScanBlock(pdf, row, input.StorageRoot, 12, currentY, 186, blockHeight)
			currentY += blockHeight + 2
		}
	}
}

func renderAttendanceCard(pdf *gofpdf.Fpdf, row AttendanceReportRow, storageRoot string, x, y, w, h float64) {
	pdf.SetDrawColor(155, 155, 155)
	pdf.Rect(x, y, w, h, "D")
	pdf.SetFillColor(28, 79, 132)
	pdf.Rect(x, y, w, 8, "F")
	pdf.SetTextColor(255, 255, 255)
	pdf.SetFont("Arial", "B", 9)
	pdf.SetXY(x, y+1.3)
	pdf.CellFormat(w, 4.5, buildAttendanceCardTitle(row), "", 0, "C", false, 0, "")

	pdf.SetTextColor(30, 30, 30)
	labelX := x + 3
	valueX := x + 28
	currentY := y + 14
	rowGap := 7.2
	renderLabelValue(pdf, labelX, valueX, currentY, "Nama", row.FullName)
	currentY += rowGap
	renderLabelValue(pdf, labelX, valueX, currentY, "Place", row.PlaceName)
	currentY += rowGap
	renderLabelValue(pdf, labelX, valueX, currentY, "Tanggal", formatDateLabel(row.AttendanceDate))
	currentY += rowGap
	renderLabelValue(pdf, labelX, valueX, currentY, "Shift", deref(row.ShiftName))
	currentY += rowGap
	renderLabelValue(pdf, labelX, valueX, currentY, "Check In", formatDateTimeLabel(deref(row.CheckInAt)))
	currentY += rowGap
	renderLabelValue(pdf, labelX, valueX, currentY, "Check Out", formatDateTimeLabel(deref(row.CheckOutAt)))
	currentY += rowGap
	renderLabelValue(pdf, labelX, valueX, currentY, "Status", row.Status)
	currentY += rowGap
	renderNotes(pdf, labelX, valueX, currentY, "Catatan", deref(row.Note), 70)

	imageX := x + 120
	imageY := y + 17
	boxW := 34.5
	boxH := 24.5
	gap := 5.5
	renderImageBox(pdf, storageRoot, deref(row.CheckInPhotoURL), imageX, imageY, boxW, boxH, "Check In")
	renderImageBox(pdf, storageRoot, deref(row.CheckOutPhotoURL), imageX+boxW+gap, imageY, boxW, boxH, "Check Out")
}

func renderLabelValue(pdf *gofpdf.Fpdf, labelX, valueX, y float64, label, value string) {
	pdf.SetTextColor(25, 25, 25)
	pdf.SetFont("Arial", "B", 8.8)
	pdf.Text(labelX, y, label)
	pdf.Text(valueX-4, y, ":")
	pdf.SetFont("Arial", "", 8.8)
	pdf.Text(valueX, y, safeText(value, "-"))
}

func renderNotes(pdf *gofpdf.Fpdf, labelX, valueX, y float64, label, value string, width float64) {
	pdf.SetFont("Arial", "B", 8.8)
	pdf.Text(labelX, y, label)
	pdf.Text(valueX-4, y, ":")
	pdf.SetXY(valueX, y-3.8)
	pdf.SetFont("Arial", "", 8.6)
	pdf.MultiCell(width, 4.4, safeText(value, "-"), "", "L", false)
}

func renderImageBox(pdf *gofpdf.Fpdf, storageRoot, photoURL string, x, y, w, h float64, label string) {
	pdf.SetTextColor(35, 35, 35)
	pdf.SetFont("Arial", "B", 7)
	pdf.SetXY(x, y-4.5)
	pdf.CellFormat(w, 4, label, "", 0, "C", false, 0, "")
	pdf.SetDrawColor(120, 120, 120)
	pdf.Rect(x, y, w, h, "D")

	imageName, imageType, imageReader, err := prepareReportImage(storageRoot, photoURL)
	if err != nil || imageReader == nil {
		pdf.SetFont("Arial", "", 7)
		pdf.SetTextColor(120, 120, 120)
		pdf.SetXY(x, y+h/2-2)
		pdf.CellFormat(w, 4, "No Image", "", 0, "C", false, 0, "")
		return
	}

	options := gofpdf.ImageOptions{ImageType: imageType}
	pdf.RegisterImageOptionsReader(imageName, options, imageReader)
	info := pdf.GetImageInfo(imageName)
	if info == nil || info.Width() <= 0 || info.Height() <= 0 {
		pdf.SetFont("Arial", "", 7)
		pdf.SetTextColor(120, 120, 120)
		pdf.SetXY(x, y+h/2-2)
		pdf.CellFormat(w, 4, "No Image", "", 0, "C", false, 0, "")
		return
	}

	iw := info.Width()
	ih := info.Height()
	scale := minFloat(w/iw, h/ih)
	drawW := iw * scale
	drawH := ih * scale
	drawX := x + (w-drawW)/2
	drawY := y + (h-drawH)/2
	pdf.ImageOptions(imageName, drawX, drawY, drawW, drawH, false, options, 0, "")
}

type patrolGroup struct {
	title string
	rows  []PatrolScanReportRow
}

func groupPatrolRows(rows []PatrolScanReportRow) []patrolGroup {
	order := make([]string, 0)
	grouped := make(map[string][]PatrolScanReportRow)
	titles := make(map[string]string)
	for _, row := range rows {
		key := strings.TrimSpace(row.SpotID)
		if key == "" {
			key = strings.TrimSpace(row.SpotCode) + "::" + strings.TrimSpace(row.SpotName)
		}
		if _, exists := grouped[key]; !exists {
			order = append(order, key)
			titles[key] = safeText(row.SpotName, "-") + " - " + safeText(row.SpotCode, "-")
		}
		grouped[key] = append(grouped[key], row)
	}
	result := make([]patrolGroup, 0, len(order))
	for _, key := range order {
		result = append(result, patrolGroup{title: titles[key], rows: grouped[key]})
	}
	return result
}

func renderPatrolGroupFrame(pdf *gofpdf.Fpdf, title string) {
	addAttendanceWatermark(pdf)
	pdf.SetDrawColor(150, 150, 150)
	pdf.Rect(10, 10, 190, 267, "D")
	pdf.SetFillColor(28, 79, 132)
	pdf.Rect(10, 10, 190, 8, "F")
	pdf.SetTextColor(255, 255, 255)
	pdf.SetFont("Arial", "B", 8.8)
	pdf.SetXY(10, 11.2)
	pdf.CellFormat(190, 4, title, "", 0, "C", false, 0, "")
}

func measurePatrolScanBlock(pdf *gofpdf.Fpdf, row PatrolScanReportRow, w float64) float64 {
	const (
		imageH     = 33.0
		imageW     = 46.0
		topBandH   = 9.0
		padding    = 2.0
		lineHeight = 5.0
	)

	infoX := imageW + 4
	infoW := w - infoX - 2
	valueW := infoW - 28
	infoHeight := maxFloat(
		measureWrappedLabelValue(pdf, "Scanned At", formatDateTimeLabel(row.ScannedAt), valueW, lineHeight),
		measureWrappedLabelValue(pdf, "Run ID", row.PatrolRunID, valueW, lineHeight)+
			measureWrappedLabelValue(pdf, "Spot", safeText(row.SpotName, "-")+" ("+safeText(row.SpotCode, "-")+")", valueW, lineHeight)+
			measureWrappedLabelValue(pdf, "Nama User", row.FullName, valueW, lineHeight),
	)
	contentH := maxFloat(imageH, infoHeight) + padding*2
	noteH := measureWrappedLabelValue(pdf, "Note", deref(row.Note), w-20, lineHeight)
	return topBandH + contentH + 9 + noteH + 6
}

func renderPatrolScanBlock(pdf *gofpdf.Fpdf, row PatrolScanReportRow, storageRoot string, x, y, w, blockHeight float64) {
	const (
		topBandH   = 9.0
		padding    = 2.0
		imageW     = 46.0
		imageH     = 33.0
		lineHeight = 5.0
	)

	pdf.SetDrawColor(150, 150, 150)
	pdf.Rect(x, y, w, blockHeight, "D")
	pdf.Rect(x, y, w, topBandH, "D")
	pdf.SetTextColor(40, 40, 40)
	pdf.SetFont("Arial", "B", 10)
	pdf.SetXY(x, y+2)
	pdf.CellFormat(w, 4, extractTimeForPatrol(row.ScannedAt), "", 0, "C", false, 0, "")

	contentY := y + topBandH + padding
	pdf.Rect(x, contentY, imageW, imageH, "D")
	imageName, imageType, imageReader, err := prepareReportImage(storageRoot, deref(row.PhotoURL))
	if err == nil && imageReader != nil {
		options := gofpdf.ImageOptions{ImageType: imageType}
		pdf.RegisterImageOptionsReader(imageName, options, imageReader)
		info := pdf.GetImageInfo(imageName)
		if info != nil && info.Width() > 0 && info.Height() > 0 {
			scale := minFloat(imageW/info.Width(), imageH/info.Height())
			drawW := info.Width() * scale
			drawH := info.Height() * scale
			drawX := x + (imageW-drawW)/2
			drawY := contentY + (imageH-drawH)/2
			pdf.ImageOptions(imageName, drawX, drawY, drawW, drawH, false, options, 0, "")
		}
	}

	infoX := x + imageW + 4
	infoY := contentY + 1
	infoW := w - (infoX - x) - 2
	infoY += renderWrappedLabelValue(pdf, infoX, infoY, 28, infoW-28, lineHeight, "Scanned At", formatDateTimeLabel(row.ScannedAt))
	infoY += renderWrappedLabelValue(pdf, infoX, infoY, 28, infoW-28, lineHeight, "Run ID", row.PatrolRunID)
	infoY += renderWrappedLabelValue(pdf, infoX, infoY, 28, infoW-28, lineHeight, "Spot", safeText(row.SpotName, "-")+" ("+safeText(row.SpotCode, "-")+")")
	infoY += renderWrappedLabelValue(pdf, infoX, infoY, 28, infoW-28, lineHeight, "Nama User", row.FullName)

	contentH := maxFloat(imageH, infoY-contentY) + padding
	metaY := contentY + contentH
	pdf.Rect(x, metaY, w, 9, "D")
	renderLabelValue(pdf, x+2, x+22, metaY+5.7, "Photo", "Available")
	if strings.TrimSpace(deref(row.PhotoURL)) == "" {
		renderLabelValue(pdf, x+2, x+22, metaY+5.7, "Photo", "Not Available")
	}
	noteY := metaY + 9
	noteH := renderWrappedLabelValue(pdf, x+2, noteY+2, 18, w-20, lineHeight, "Note", deref(row.Note))
	pdf.Rect(x, noteY, w, noteH+4, "D")
}

func renderWrappedLabelValue(pdf *gofpdf.Fpdf, x, y, labelW, valueW, lineHeight float64, label, value string) float64 {
	height := measureWrappedLabelValue(pdf, label, value, valueW, lineHeight)
	pdf.SetTextColor(25, 25, 25)
	pdf.SetFont("Arial", "B", 8.8)
	pdf.Text(x, y+3.6, label)
	pdf.Text(x+labelW-4, y+3.6, ":")
	pdf.SetXY(x+labelW, y)
	pdf.SetFont("Arial", "", 8.6)
	pdf.MultiCell(valueW, lineHeight, safeText(value, "-"), "", "L", false)
	return height
}

func measureWrappedLabelValue(pdf *gofpdf.Fpdf, label, value string, valueW, lineHeight float64) float64 {
	pdf.SetFont("Arial", "", 8.6)
	lines := pdf.SplitLines([]byte(safeText(value, "-")), valueW)
	if len(lines) == 0 {
		return lineHeight
	}
	return float64(len(lines)) * lineHeight
}

func extractTimeForPatrol(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 16 {
		return strings.ReplaceAll(value[11:16], ":", ".")
	}
	return "-"
}

func prepareReportImage(storageRoot, photoURL string) (string, string, *bytes.Reader, error) {
	path := resolveReportImagePath(storageRoot, photoURL)
	if path == "" {
		return "", "", nil, fmt.Errorf("empty path")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", "", nil, err
	}

	img, format, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".svg" {
			return "", "", nil, err
		}
		return "", "", nil, err
	}

	var encoded bytes.Buffer
	switch strings.ToLower(format) {
	case "jpeg", "jpg":
		if err := jpeg.Encode(&encoded, img, &jpeg.Options{Quality: 85}); err != nil {
			return "", "", nil, err
		}
		return path + "-jpg", "JPG", bytes.NewReader(encoded.Bytes()), nil
	default:
		if err := png.Encode(&encoded, img); err != nil {
			return "", "", nil, err
		}
		return path + "-png", "PNG", bytes.NewReader(encoded.Bytes()), nil
	}
}

func resolveReportImagePath(storageRoot, photoURL string) string {
	value := strings.TrimSpace(photoURL)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "/uploads/") {
		relative := strings.TrimPrefix(value, "/uploads/")
		return filepath.Join(storageRoot, filepath.FromSlash(relative))
	}
	if filepath.IsAbs(value) {
		return value
	}
	return ""
}

func addAttendanceWatermark(pdf *gofpdf.Fpdf) {
	pdf.SetAlpha(0.05, "Normal")
	pdf.SetTextColor(88, 103, 126)
	pdf.TransformBegin()
	pdf.TransformRotate(35, 105, 160)
	pdf.SetFont("Arial", "B", 28)
	pdf.Text(18, 190, "AZKA SMART SECURE SYSTEM")
	pdf.TransformEnd()
	pdf.SetAlpha(1, "Normal")
}

func writeParagraph(pdf *gofpdf.Fpdf, x, y, w, lineHeight float64, text string) {
	pdf.SetXY(x, y)
	pdf.MultiCell(w, lineHeight, text, "", "L", false)
}

func buildAttendanceCardTitle(row AttendanceReportRow) string {
	return safeText(row.FullName, "-") + " - " + formatDateLabel(row.AttendanceDate) + " - " + safeText(row.Status, "-")
}

func formatDateRange(fromDate, toDate string) string {
	from := formatDateLabel(fromDate)
	to := formatDateLabel(toDate)
	if from == "" && to == "" {
		return "-"
	}
	if to == "" || from == to {
		return from
	}
	if from == "" {
		return to
	}
	return from + " s/d " + to
}

func formatDateLabel(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 10 && value[4] == '-' && value[7] == '-' {
		return value[8:10] + "/" + value[5:7] + "/" + value[0:4]
	}
	return value
}

func formatDateTimeLabel(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	if len(value) >= 19 && value[4] == '-' && value[7] == '-' {
		datePart := formatDateLabel(value[:10])
		timePart := strings.NewReplacer(":", ".", "T", ", ").Replace(value[11:19])
		return datePart + ", " + timePart
	}
	return value
}

func safeText(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return value
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func init() {
	image.RegisterFormat("webp", "RIFF????WEBPVP8", webp.Decode, webp.DecodeConfig)
}

func stringifyInt(v int) string {
	return strconv.Itoa(v)
}

func truncateCell(value string, max int) string {
	if len(value) <= max {
		return value
	}
	if max <= 3 {
		return value[:max]
	}
	return value[:max-3] + "..."
}

func filename(base, ext string) string {
	return fmt.Sprintf("%s.%s", base, ext)
}
