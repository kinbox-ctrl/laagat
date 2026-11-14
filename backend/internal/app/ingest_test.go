package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-pdf/fpdf"
	"github.com/xuri/excelize/v2"
)

var sampleRows = [][]string{
	{"M/s Example Builders - Estimate"},
	{"Sr. No.", "Particulars", "UOM", "Quantity", "Rate (Rs)", "Amount (Rs)"},
	{"", "SUBSTRUCTURE"},
	{"1", "Earthwork excavation", "Cum", "95", "450", "42,750"},
	{"2", "PCC 1:4:8", "Cum", "4.5", "6,200.00", "Rs. 27,900/-"},
	{"", "Sub Total", "", "", "", "70,650"},
	{"3", "Electrical works", "LS", "", "", "2,85,000"},
	{"", "Grand Total", "", "", "", "3,55,650"},
	{"", "Total in words: three lakh..."},
}

func TestParseTable(t *testing.T) {
	items, total, err := ParseTable(sampleRows)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 || total != 355650 {
		t.Fatalf("items=%d total=%v: %+v", len(items), total, items)
	}
	if items[1].Rate != 6200 || items[1].Amount != 27900 || items[1].Unit != "Cum" || items[1].SlNo != "2" {
		t.Fatalf("row 2: %+v", items[1])
	}
	if items[2].Amount != 285000 || items[2].Qty != 0 {
		t.Fatalf("lump sum row: %+v", items[2])
	}
	if _, _, err := ParseTable([][]string{{"hello", "world"}, {"1", "2"}}); err == nil {
		t.Fatal("expected an error for a grid with no BOQ header")
	}
}

func TestReadXLSXFindsTheBOQSheet(t *testing.T) {
	f := excelize.NewFile()
	f.SetCellValue("Sheet1", "A1", "Cover page")
	f.NewSheet("BOQ")
	for i, row := range sampleRows {
		cells := make([]any, len(row))
		for j, c := range row {
			cells[j] = c
		}
		f.SetSheetRow("BOQ", "A"+string(rune('1'+i)), &cells)
	}
	path := filepath.Join(t.TempDir(), "boq.xlsx")
	if err := f.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	rows, err := readXLSX(path)
	if err != nil {
		t.Fatal(err)
	}
	if items, _, err := ParseTable(rows); err != nil || len(items) != 3 {
		t.Fatalf("items=%v err=%v", items, err)
	}
}

// TestPDFWorker runs the real Python worker on a generated PDF with a ruled
// BOQ table. It is skipped when the worker environment is not installed.
func TestPDFWorker(t *testing.T) {
	python, _ := filepath.Abs("../../../workers/.venv/bin/python")
	script, _ := filepath.Abs("../../../workers/extract_tables.py")
	if _, err := os.Stat(python); err != nil {
		t.Skip("PDF worker not installed (run `make worker`)")
	}
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 9)
	widths := []float64{15, 70, 18, 22, 25, 30}
	for _, row := range sampleRows[1:8] {
		for i, w := range widths {
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			pdf.CellFormat(w, 7, cell, "1", 0, "L", false, 0, "")
		}
		pdf.Ln(-1)
	}
	path := filepath.Join(t.TempDir(), "boq.pdf")
	if err := pdf.OutputFileAndClose(path); err != nil {
		t.Fatal(err)
	}
	s := &Server{PDFWorker: []string{python, script}}
	rows, err := s.readPDF(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	items, total, err := ParseTable(rows)
	if err != nil || len(items) != 3 || total != 355650 {
		t.Fatalf("items=%+v total=%v err=%v", items, total, err)
	}
}
