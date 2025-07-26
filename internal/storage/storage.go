package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"openstack-reporter/internal/models"
)

const (
	dataDir      = "data"
	reportFile   = "openstack_report.json"
	backupPrefix = "backup_"
)

type Storage struct {
	dataPath string
}

func NewStorage() *Storage {
	return &Storage{
		dataPath: dataDir,
	}
}

// Initialize creates the data directory if it doesn't exist
func (s *Storage) Initialize() error {
	if err := os.MkdirAll(s.dataPath, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}
	return nil
}

// SaveReport saves the resource report to JSON file
func (s *Storage) SaveReport(report *models.ResourceReport) error {
	reportPath := filepath.Join(s.dataPath, reportFile)

	// Create backup of existing report
	if _, err := os.Stat(reportPath); err == nil {
		backupPath := filepath.Join(s.dataPath, fmt.Sprintf("%s%d_%s",
			backupPrefix, time.Now().Unix(), reportFile))
		if err := os.Rename(reportPath, backupPath); err != nil {
			return fmt.Errorf("failed to create backup: %w", err)
		}
	}

	// Marshal report to JSON
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal report: %w", err)
	}

	// Write to file
	if err := os.WriteFile(reportPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write report file: %w", err)
	}

	return nil
}

// LoadReport loads the resource report from JSON file
func (s *Storage) LoadReport() (*models.ResourceReport, error) {
	reportPath := filepath.Join(s.dataPath, reportFile)

	data, err := os.ReadFile(reportPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no saved report found")
		}
		return nil, fmt.Errorf("failed to read report file: %w", err)
	}

	var report models.ResourceReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("failed to unmarshal report: %w", err)
	}

	return &report, nil
}

// ReportExists checks if a saved report exists
func (s *Storage) ReportExists() bool {
	reportPath := filepath.Join(s.dataPath, reportFile)
	_, err := os.Stat(reportPath)
	return err == nil
}

// GetReportAge returns the age of the saved report
func (s *Storage) GetReportAge() (time.Duration, error) {
	reportPath := filepath.Join(s.dataPath, reportFile)

	info, err := os.Stat(reportPath)
	if err != nil {
		return 0, fmt.Errorf("failed to get report file info: %w", err)
	}

	return time.Since(info.ModTime()), nil
}

// CleanupBackups removes backup files older than specified duration
func (s *Storage) CleanupBackups(maxAge time.Duration) error {
	files, err := os.ReadDir(s.dataPath)
	if err != nil {
		return fmt.Errorf("failed to read data directory: %w", err)
	}

	for _, file := range files {
		if !file.IsDir() && len(file.Name()) > len(backupPrefix) &&
		   file.Name()[:len(backupPrefix)] == backupPrefix {

			filePath := filepath.Join(s.dataPath, file.Name())
			info, err := file.Info()
			if err != nil {
				continue
			}

			if time.Since(info.ModTime()) > maxAge {
				if err := os.Remove(filePath); err != nil {
					return fmt.Errorf("failed to remove backup file %s: %w", filePath, err)
				}
			}
		}
	}

	return nil
}
