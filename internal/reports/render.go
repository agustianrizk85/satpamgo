package reports

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"strconv"

	"github.com/phpdave11/gofpdf"
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
