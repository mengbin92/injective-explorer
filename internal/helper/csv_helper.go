// Package helper provides utility functions for address label import/export.
package helper

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"injective-explorer/internal/models"
)

// AddressLabelImport represents an address label from import file.
type AddressLabelImport struct {
	Address     string
	LabelType   string
	LabelValue  string
	LabelColor  string
	Description string
	Visible     bool
	CreatedBy   string
	Remark      string
}

// ParseCSVLabels parses CSV content into address labels.
//
// Parameters:
//   - content: CSV file content as bytes
//   - csvFormat: "simple" (address,label_name,remark) or "full" (address,label_name,label_type,label_color,description,visible,remark)
//
// Returns:
//   - []AddressLabelImport: Parsed labels
//   - error: Error if parsing fails
func ParseCSVLabels(content []byte, csvFormat string) ([]AddressLabelImport, error) {
	reader := csv.NewReader(strings.NewReader(string(content)))
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV: %w", err)
	}

	var labels []AddressLabelImport

	// Detect CSV format if not specified
	if csvFormat == "" {
		if len(records) > 0 && len(records[0]) >= 3 {
			firstRow := records[0]
			if strings.Contains(strings.ToLower(firstRow[0]), "address") ||
			   strings.Contains(strings.ToLower(firstRow[1]), "label") {
				csvFormat = "full"
			} else {
				csvFormat = "simple"
			}
		} else {
			csvFormat = "simple"
		}
	}

	for i, record := range records {
		// Skip header row if present
		if i == 0 && (strings.Contains(strings.ToLower(record[0]), "address") ||
		   strings.Contains(strings.ToLower(record[0]), "地址")) {
			continue
		}

		var label AddressLabelImport

		if csvFormat == "simple" {
			// Simple format: address,label_name,remark
			if len(record) < 3 {
				continue
			}
			label = AddressLabelImport{
				Address:    record[0],
				LabelValue: record[1],
				Remark:     record[2],
				LabelType:  "custom", // Default type
				Visible:    true,    // Default visible
			}
		} else {
			// Full format: address,label_name,label_type,label_color,description,visible,remark
			if len(record) < 7 {
				continue
			}
			visible := strings.ToLower(record[5]) == "true" || record[5] == "1"
			label = AddressLabelImport{
				Address:     record[0],
				LabelValue:  record[1],
				LabelType:   record[2],
				LabelColor:  record[3],
				Description: record[4],
				Visible:     visible,
				Remark:      record[6],
			}
		}

		labels = append(labels, label)
	}

	return labels, nil
}

// ParseJSONLabels parses JSON content into address labels.
//
// Parameters:
//   - content: JSON file content as bytes
//
// Returns:
//   - []AddressLabelImport: Parsed labels
//   - error: Error if parsing fails
func ParseJSONLabels(content []byte) ([]AddressLabelImport, error) {
	var labels []AddressLabelImport
	err := json.Unmarshal(content, &labels)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}
	return labels, nil
}

// ExportLabelsToCSV exports labels to CSV format.
//
// Parameters:
//   - labels: Labels to export
//
// Returns:
//   - string: CSV content
//   - string: Suggested file name
func ExportLabelsToCSV(labels []models.AddressLabel) (string, string) {
	var buf strings.Builder
	writer := csv.NewWriter(&buf)
	defer writer.Flush()

	// Write header
	writer.Write([]string{"Address", "ChainID", "LabelType", "LabelValue", "LabelColor", "Description", "Visible", "CreatedBy", "CreatedAt", "UpdatedAt"})

	// Write data
	for _, label := range labels {
		visible := "false"
		if label.Visible {
			visible = "true"
		}
		writer.Write([]string{
			label.Address,
			label.ChainID,
			label.LabelType,
			label.LabelValue,
			label.LabelColor,
			label.Description,
			visible,
			label.CreatedBy,
			label.CreatedAt.Format("2006-01-02 15:04:05"),
			label.UpdatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	fileName := fmt.Sprintf("address_labels_%s.csv", time.Now().Format("20060102_150405"))
	return buf.String(), fileName
}

// ExportLabelsToJSON exports labels to JSON format.
//
// Parameters:
//   - labels: Labels to export
//
// Returns:
//   - string: JSON content
//   - string: Suggested file name
func ExportLabelsToJSON(labels []models.AddressLabel) (string, string) {
	data := make([]map[string]interface{}, 0, len(labels))

	for _, label := range labels {
		data = append(data, map[string]interface{}{
			"address":     label.Address,
			"chain_id":    label.ChainID,
			"label_type":  label.LabelType,
			"label_value": label.LabelValue,
			"label_color": label.LabelColor,
			"description": label.Description,
			"visible":     label.Visible,
			"created_by":  label.CreatedBy,
			"created_at":  label.CreatedAt.Format(time.RFC3339),
			"updated_at":  label.UpdatedAt.Format(time.RFC3339),
		})
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return "", ""
	}

	fileName := fmt.Sprintf("address_labels_%s.json", time.Now().Format("20060102_150405"))
	return string(jsonBytes), fileName
}

// WriteCSV writes data to CSV writer with proper escaping.
func WriteCSV(writer *csv.Writer, record []string) error {
	return writer.Write(record)
}
