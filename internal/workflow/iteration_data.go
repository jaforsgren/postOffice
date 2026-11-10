package workflow

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"postOffice/internal/script"
)

const (
	csvDataExtension  = ".csv"
	jsonDataExtension = ".json"
)

var (
	// ErrUnsupportedDataFormat is returned for a data file that is neither CSV nor JSON.
	ErrUnsupportedDataFormat = errors.New("unsupported data file format (use .csv or .json)")
	// ErrNoDataRows is returned for a data file without any iteration rows.
	ErrNoDataRows = errors.New("data file has no rows")
	// ErrInvalidDataFile is returned for a data file whose content cannot drive iterations.
	ErrInvalidDataFile = errors.New("invalid data file")
)

// LoadIterationData reads a Postman-style data file: a CSV file whose header
// row names the variables and whose every other row is one iteration, or a
// JSON array of objects, one per iteration. JSON values keep their types.
func LoadIterationData(path string) ([]script.IterationData, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read data file: %w", err)
	}

	var rows []script.IterationData
	switch strings.ToLower(filepath.Ext(path)) {
	case csvDataExtension:
		rows, err = parseCSVData(content)
	case jsonDataExtension:
		rows, err = parseJSONData(content)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedDataFormat, filepath.Base(path))
	}
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrNoDataRows, filepath.Base(path))
	}
	return rows, nil
}

func parseCSVData(content []byte) ([]script.IterationData, error) {
	records, err := csv.NewReader(strings.NewReader(string(content))).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidDataFile, err)
	}
	if len(records) == 0 {
		return nil, nil
	}

	header := records[0]
	rows := make([]script.IterationData, 0, len(records)-1)
	for _, record := range records[1:] {
		row := make(script.IterationData, len(header))
		for column, key := range header {
			row[key] = record[column]
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func parseJSONData(content []byte) ([]script.IterationData, error) {
	var rows []script.IterationData
	if err := json.Unmarshal(content, &rows); err != nil {
		return nil, fmt.Errorf("%w: expected a JSON array of objects: %v", ErrInvalidDataFile, err)
	}
	return rows, nil
}
