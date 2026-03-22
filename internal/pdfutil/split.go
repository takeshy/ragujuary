package pdfutil

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/ledongthuc/pdf"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

const DefaultMaxPages = 6

// PDFChunk represents a contiguous range of pages from a PDF
type PDFChunk struct {
	Data       []byte
	StartPage  int // 1-based
	EndPage    int // 1-based, inclusive
	TotalPages int
}

// PageCount returns the number of pages in a PDF
func PageCount(data []byte) (int, error) {
	rs := bytes.NewReader(data)
	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed
	return api.PageCount(rs, conf)
}

// SplitPages splits a PDF into chunks of at most maxPages pages each.
// If maxPages <= 0, DefaultMaxPages (6) is used.
// If the PDF has <= maxPages pages, returns a single chunk with the original data.
func SplitPages(data []byte, maxPages int) ([]PDFChunk, error) {
	if maxPages <= 0 {
		maxPages = DefaultMaxPages
	}

	totalPages, err := PageCount(data)
	if err != nil {
		return nil, fmt.Errorf("failed to get page count: %w", err)
	}

	if totalPages <= maxPages {
		return []PDFChunk{{
			Data:       data,
			StartPage:  1,
			EndPage:    totalPages,
			TotalPages: totalPages,
		}}, nil
	}

	var chunks []PDFChunk
	for start := 1; start <= totalPages; start += maxPages {
		end := start + maxPages - 1
		if end > totalPages {
			end = totalPages
		}

		var buf bytes.Buffer
		rs := bytes.NewReader(data)
		conf := model.NewDefaultConfiguration()
		conf.ValidationMode = model.ValidationRelaxed
		pageSelection := fmt.Sprintf("%d-%d", start, end)

		if err := api.Collect(rs, &buf, []string{pageSelection}, conf); err != nil {
			return nil, fmt.Errorf("failed to extract pages %s: %w", pageSelection, err)
		}

		chunks = append(chunks, PDFChunk{
			Data:       buf.Bytes(),
			StartPage:  start,
			EndPage:    end,
			TotalPages: totalPages,
		})
	}

	return chunks, nil
}

// ExtractText extracts plain text from a PDF, returning text per page.
func ExtractText(data []byte) ([]string, error) {
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("failed to open PDF: %w", err)
	}

	numPages := r.NumPage()
	pages := make([]string, 0, numPages)
	for i := 1; i <= numPages; i++ {
		p := r.Page(i)
		if p.V.IsNull() {
			pages = append(pages, "")
			continue
		}
		text, err := p.GetPlainText(nil)
		if err != nil {
			pages = append(pages, "")
			continue
		}
		pages = append(pages, strings.TrimSpace(text))
	}
	return pages, nil
}

// ExtractAllText extracts all text from a PDF as a single string.
func ExtractAllText(data []byte) (string, error) {
	pages, err := ExtractText(data)
	if err != nil {
		return "", err
	}
	return strings.Join(pages, "\n\n"), nil
}
