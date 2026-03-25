package runner

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

// loadCSVExamples reads a CSV file and returns an ExamplesBlock.
// The first row is treated as headers, remaining rows as data.
// csvPath is resolved relative to basePath (the directory of the .probe file).
func loadCSVExamples(basePath, csvPath string) (*parser.ExamplesBlock, error) {
	resolved := csvPath
	if !filepath.IsAbs(csvPath) {
		resolved = filepath.Join(basePath, csvPath)
	}

	f, err := os.Open(resolved)
	if err != nil {
		return nil, fmt.Errorf("open CSV %s: %w", csvPath, err)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse CSV %s: %w", csvPath, err)
	}

	if len(records) < 2 {
		return nil, fmt.Errorf("CSV %s: need at least a header row and one data row", csvPath)
	}

	return &parser.ExamplesBlock{
		Headers: records[0],
		Rows:    records[1:],
		Source:  csvPath,
	}, nil
}
