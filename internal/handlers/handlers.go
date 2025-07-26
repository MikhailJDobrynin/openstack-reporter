package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"openstack-reporter/internal/models"
	"openstack-reporter/internal/openstack"
	"openstack-reporter/internal/storage"
	"openstack-reporter/internal/pdf"
)

type Handler struct {
	storage          *storage.Storage
	progressChannels map[string]chan openstack.ProgressMessage
	mu               sync.RWMutex
}

func NewHandler() *Handler {
	storage := storage.NewStorage()
	if err := storage.Initialize(); err != nil {
		log.Printf("Warning: Failed to initialize storage: %v", err)
	}

	return &Handler{
		storage:          storage,
		progressChannels: make(map[string]chan openstack.ProgressMessage),
	}
}

// GetResources returns cached resources or loads them if not available
func (h *Handler) GetResources(c *gin.Context) {
	// Try to load cached report first
	report, err := h.storage.LoadReport()
	if err != nil {
		log.Printf("No cached report found, attempting to fetch from OpenStack: %v", err)

		// If no cache, try to fetch from OpenStack
		freshReport, fetchErr := h.fetchFromOpenStack()
		if fetchErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to load cached data and unable to fetch from OpenStack",
				"details": fetchErr.Error(),
			})
			return
		}

		report = freshReport

		// Save to cache
		if saveErr := h.storage.SaveReport(report); saveErr != nil {
			log.Printf("Warning: Failed to save report to cache: %v", saveErr)
		}
	}

	c.JSON(http.StatusOK, report)
}

// RefreshResources fetches fresh data from OpenStack and saves it
func (h *Handler) RefreshResources(c *gin.Context) {
	report, err := h.fetchFromOpenStack()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch resources from OpenStack",
			"details": err.Error(),
		})
		return
	}

	// Save the fresh report
	if err := h.storage.SaveReport(report); err != nil {
		log.Printf("Warning: Failed to save refreshed report: %v", err)
	}

	// Clean up old backups (keep last 7 days)
	if err := h.storage.CleanupBackups(7 * 24 * time.Hour); err != nil {
		log.Printf("Warning: Failed to cleanup backups: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Resources refreshed successfully",
		"generated_at": report.GeneratedAt,
		"total_resources": len(report.Resources),
	})
}

// RefreshWithProgress fetches fresh data from OpenStack with progress updates
func (h *Handler) RefreshWithProgress(c *gin.Context) {
	progressChan := make(chan openstack.ProgressMessage, 100)
	sessionID := fmt.Sprintf("session_%d", time.Now().UnixNano())

	// Store progress channel
	h.mu.Lock()
	h.progressChannels[sessionID] = progressChan
	h.mu.Unlock()

	// Start background refresh
	go func() {
		defer func() {
			// Clean up when goroutine is done
			h.mu.Lock()
			delete(h.progressChannels, sessionID)
			h.mu.Unlock()
			close(progressChan)
		}()

		report, err := h.fetchFromOpenStackWithProgress(progressChan)
		if err != nil {
			select {
			case progressChan <- openstack.ProgressMessage{
				Type:    "error",
				Message: fmt.Sprintf("Failed to fetch resources: %v", err),
			}:
			default:
			}
			return
		}

		// Save the fresh report
		if err := h.storage.SaveReport(report); err != nil {
			log.Printf("Warning: Failed to save refreshed report: %v", err)
		}

		// Clean up old backups
		if err := h.storage.CleanupBackups(7 * 24 * time.Hour); err != nil {
			log.Printf("Warning: Failed to cleanup backups: %v", err)
		}

		select {
		case progressChan <- openstack.ProgressMessage{
			Type:    "complete",
			Message: "Resources refreshed successfully",
			Summary: calculateTypeSummary(report.Resources),
		}:
		default:
		}
	}()

	c.JSON(http.StatusOK, gin.H{
		"message":    "Refresh started",
		"session_id": sessionID,
	})
}

// GetProgress returns SSE stream of progress updates
func (h *Handler) GetProgress(c *gin.Context) {
	sessionID := c.Query("session_id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session_id is required"})
		return
	}

	h.mu.RLock()
	progressChan, exists := h.progressChannels[sessionID]
	h.mu.RUnlock()

	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")

	// Send events
	for {
		select {
		case msg, ok := <-progressChan:
			if !ok {
				return
			}

			data, _ := json.Marshal(msg)
			fmt.Fprintf(c.Writer, "data: %s\n\n", data)
			c.Writer.Flush()

			if msg.Type == "complete" || msg.Type == "error" {
				return
			}
		case <-c.Request.Context().Done():
			return
		}
	}
}

// ExportToPDF generates and returns a PDF report
func (h *Handler) ExportToPDF(c *gin.Context) {
	log.Printf("PDF export requested from %s", c.ClientIP())

	// Check if report exists
	if !h.storage.ReportExists() {
		log.Printf("PDF export failed: no report data available")
		c.JSON(http.StatusNotFound, gin.H{
			"error": "No report data available for export",
			"details": "Please refresh the data first",
		})
		return
	}

	// Load current report
	report, err := h.storage.LoadReport()
	if err != nil {
		log.Printf("PDF export failed: error loading report: %v", err)
		c.JSON(http.StatusNotFound, gin.H{
			"error": "No report data available for export",
			"details": "Please refresh the data first",
		})
		return
	}

	log.Printf("PDF export: loaded report with %d resources", len(report.Resources))

	// Generate PDF
	log.Printf("PDF export: starting PDF generation")
	pdfGenerator := pdf.NewGenerator()
	pdfData, err := pdfGenerator.GenerateReport(report)
	if err != nil {
		log.Printf("PDF export failed: error generating PDF: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to generate PDF",
			"details": err.Error(),
		})
		return
	}

	log.Printf("PDF export: successfully generated PDF (%d bytes)", len(pdfData))

	// Set headers for PDF download
	filename := "openstack_report_" + time.Now().Format("2006-01-02_15-04-05") + ".pdf"
	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Header("Content-Length", string(rune(len(pdfData))))

	c.Data(http.StatusOK, "application/pdf", pdfData)
}

// GetReportStatus returns information about the current report
func (h *Handler) GetReportStatus(c *gin.Context) {
	status := gin.H{
		"report_exists": h.storage.ReportExists(),
		"last_check": time.Now(),
	}

	if h.storage.ReportExists() {
		age, err := h.storage.GetReportAge()
		if err == nil {
			status["report_age_hours"] = age.Hours()
			status["report_age_human"] = formatDuration(age)
		}
	}

	c.JSON(http.StatusOK, status)
}

// fetchFromOpenStack connects to OpenStack and fetches all resources
func (h *Handler) fetchFromOpenStack() (*models.ResourceReport, error) {
	client, err := openstack.NewClient()
	if err != nil {
		return nil, err
	}

	return client.GetAllResources()
}

// fetchFromOpenStackWithProgress connects to OpenStack and fetches all resources with progress updates
func (h *Handler) fetchFromOpenStackWithProgress(progressChan chan openstack.ProgressMessage) (*models.ResourceReport, error) {
	select {
	case progressChan <- openstack.ProgressMessage{
		Type:    "start",
		Message: "Initializing OpenStack client...",
	}:
	default:
	}

	client, err := openstack.NewClient()
	if err != nil {
		return nil, err
	}

	select {
	case progressChan <- openstack.ProgressMessage{
		Type:    "progress",
		Message: "Getting resources with progress updates...",
	}:
	default:
	}

	return client.GetAllResourcesWithProgress(progressChan)
}

// calculateTypeSummary creates a summary of resources by type
func calculateTypeSummary(resources []models.Resource) map[string]int {
	summary := make(map[string]int)
	for _, resource := range resources {
		summary[resource.Type]++
	}
	return summary
}

// formatDuration formats duration in human readable format
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return "меньше минуты"
	}
	if d < time.Hour {
		minutes := int(d.Minutes())
		if minutes == 1 {
			return "1 минута"
		}
		return string(rune(minutes)) + " минут"
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		if hours == 1 {
			return "1 час"
		}
		return string(rune(hours)) + " часов"
	}
	days := int(d.Hours() / 24)
	if days == 1 {
		return "1 день"
	}
	return string(rune(days)) + " дней"
}
