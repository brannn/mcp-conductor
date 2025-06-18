/*
Copyright 2024 MCP Conductor Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package examples

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/go-logr/logr"

	mcpv1 "github.com/mcp-conductor/mcp-conductor/api/v1"
	"github.com/mcp-conductor/mcp-conductor/internal/agent"
)

// DataProcessor provides data processing capabilities
type DataProcessor struct {
	Logger logr.Logger
}

// NewDataProcessor creates a new data processor agent
func NewDataProcessor(logger logr.Logger) *DataProcessor {
	return &DataProcessor{
		Logger: logger.WithName("data-processor"),
	}
}

// GetCapabilities returns the capabilities this agent provides
func (d *DataProcessor) GetCapabilities() []string {
	return []string{
		"csv-transform",
		"json-validate",
		"data-clean",
		"data-aggregate",
		"data-filter",
		"format-convert",
	}
}

// GetTags returns the tags for this agent
func (d *DataProcessor) GetTags() []string {
	return []string{
		"data-processing",
		"transformation",
		"validation",
		"analytics",
	}
}

// ExecuteTask executes a data processing task
func (d *DataProcessor) ExecuteTask(ctx context.Context, task *mcpv1.Task) (*agent.TaskResult, error) {
	d.Logger.Info("Executing data processing task",
		"task", task.Name,
		"capabilities", task.Spec.RequiredCapabilities)

	// Parse task payload
	payload, err := d.parseTaskPayload(task)
	if err != nil {
		return &agent.TaskResult{
			Success: false,
			Error:   fmt.Sprintf("Failed to parse task payload: %v", err),
		}, nil
	}

	// Determine which capability to execute
	capability := d.determineCapability(task.Spec.RequiredCapabilities)
	if capability == "" {
		return &agent.TaskResult{
			Success: false,
			Error:   "No supported capability found in task requirements",
		}, nil
	}

	// Execute the appropriate capability
	result, err := d.executeCapability(ctx, capability, payload)
	if err != nil {
		return &agent.TaskResult{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	return &agent.TaskResult{
		Success: true,
		Data:    result,
	}, nil
}

// parseTaskPayload extracts parameters from the task payload
func (d *DataProcessor) parseTaskPayload(task *mcpv1.Task) (map[string]interface{}, error) {
	if task.Spec.Payload == nil {
		return map[string]interface{}{}, nil
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(task.Spec.Payload.Raw, &payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	return payload, nil
}

// determineCapability finds the first supported capability from the task requirements
func (d *DataProcessor) determineCapability(requiredCapabilities []string) string {
	supportedCapabilities := make(map[string]bool)
	for _, cap := range d.GetCapabilities() {
		supportedCapabilities[cap] = true
	}

	for _, required := range requiredCapabilities {
		if supportedCapabilities[required] {
			return required
		}
	}

	return ""
}

// executeCapability executes a specific capability
func (d *DataProcessor) executeCapability(ctx context.Context, capability string, payload map[string]interface{}) (map[string]interface{}, error) {
	switch capability {
	case "csv-transform":
		return d.transformCSV(ctx, payload)
	case "json-validate":
		return d.validateJSON(ctx, payload)
	case "data-clean":
		return d.cleanData(ctx, payload)
	case "data-aggregate":
		return d.aggregateData(ctx, payload)
	case "data-filter":
		return d.filterData(ctx, payload)
	case "format-convert":
		return d.convertFormat(ctx, payload)
	default:
		return nil, fmt.Errorf("unsupported capability: %s", capability)
	}
}

// transformCSV transforms CSV data according to specified rules
func (d *DataProcessor) transformCSV(ctx context.Context, payload map[string]interface{}) (map[string]interface{}, error) {
	// Simulate CSV transformation
	d.Logger.Info("Transforming CSV data")

	// In a real implementation, you would:
	// 1. Read CSV data from the specified source
	// 2. Apply transformations (field mapping, data type conversion, etc.)
	// 3. Write to the destination

	time.Sleep(2 * time.Second) // Simulate processing time

	return map[string]interface{}{
		"operation":        "csv-transform",
		"recordsProcessed": 1000,
		"transformations":  []string{"email_lowercase", "phone_normalize", "date_parse"},
		"outputFormat":     "json",
		"status":           "completed",
		"processingTime":   "2.1s",
	}, nil
}

// validateJSON validates JSON data against a schema
func (d *DataProcessor) validateJSON(ctx context.Context, payload map[string]interface{}) (map[string]interface{}, error) {
	d.Logger.Info("Validating JSON data")

	// Simulate JSON validation
	time.Sleep(1 * time.Second)

	return map[string]interface{}{
		"operation":      "json-validate",
		"valid":          true,
		"recordsChecked": 500,
		"errors":         []string{},
		"warnings":       []string{"deprecated_field_usage"},
		"status":         "completed",
	}, nil
}

// cleanData performs data cleaning operations
func (d *DataProcessor) cleanData(ctx context.Context, payload map[string]interface{}) (map[string]interface{}, error) {
	d.Logger.Info("Cleaning data")

	// Simulate data cleaning
	time.Sleep(3 * time.Second)

	return map[string]interface{}{
		"operation":         "data-clean",
		"recordsProcessed":  2000,
		"duplicatesRemoved": 45,
		"nullsHandled":      123,
		"outliersTreated":   12,
		"status":            "completed",
	}, nil
}

// aggregateData performs data aggregation
func (d *DataProcessor) aggregateData(ctx context.Context, payload map[string]interface{}) (map[string]interface{}, error) {
	d.Logger.Info("Aggregating data")

	// Simulate data aggregation
	time.Sleep(2 * time.Second)

	return map[string]interface{}{
		"operation": "data-aggregate",
		"groupBy":   []string{"category", "region"},
		"aggregations": map[string]interface{}{
			"total_sales": 1250000.50,
			"avg_price":   45.67,
			"count":       5000,
		},
		"status": "completed",
	}, nil
}

// filterData filters data based on criteria
func (d *DataProcessor) filterData(ctx context.Context, payload map[string]interface{}) (map[string]interface{}, error) {
	d.Logger.Info("Filtering data")

	// Simulate data filtering
	time.Sleep(1 * time.Second)

	return map[string]interface{}{
		"operation":       "data-filter",
		"inputRecords":    10000,
		"outputRecords":   7500,
		"filterCriteria":  payload["criteria"],
		"recordsFiltered": 2500,
		"status":          "completed",
	}, nil
}

// convertFormat converts data between formats
func (d *DataProcessor) convertFormat(ctx context.Context, payload map[string]interface{}) (map[string]interface{}, error) {
	d.Logger.Info("Converting data format")

	sourceFormat := d.getStringParam(payload, "sourceFormat", "csv")
	targetFormat := d.getStringParam(payload, "targetFormat", "json")

	// Simulate format conversion
	time.Sleep(1 * time.Second)

	return map[string]interface{}{
		"operation":        "format-convert",
		"sourceFormat":     sourceFormat,
		"targetFormat":     targetFormat,
		"recordsConverted": 3000,
		"status":           "completed",
	}, nil
}

// Helper methods

func (d *DataProcessor) getStringParam(payload map[string]interface{}, key, defaultValue string) string {
	if value, ok := payload[key]; ok {
		if str, ok := value.(string); ok {
			return str
		}
	}
	return defaultValue
}

// parseCSV parses CSV data from a string
func (d *DataProcessor) parseCSV(data string) ([][]string, error) {
	reader := csv.NewReader(strings.NewReader(data))
	records := [][]string{}

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}

	return records, nil
}

// validateJSONString validates if a string is valid JSON
func (d *DataProcessor) validateJSONString(data string) error {
	var js json.RawMessage
	return json.Unmarshal([]byte(data), &js)
}
