package pdf

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/jung-kurt/gofpdf"

	"openstack-reporter/internal/models"
)

type Generator struct{}

func NewGenerator() *Generator {
	return &Generator{}
}

// GenerateReport creates a PDF report from the resource data
func (g *Generator) GenerateReport(report *models.ResourceReport) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.AddPage()

	// Add title
	g.addTitle(pdf, "OpenStack Resources Report")

	// Add generation info
	g.addGenerationInfo(pdf, report.GeneratedAt)

	// Add summary section
	g.addSummary(pdf, report.Summary)

	// Add projects section
	g.addProjectsSection(pdf, report.Projects)

	// Add resources by type
	g.addResourcesByType(pdf, report.Resources)

	// Add detailed resources table
	g.addResourcesTable(pdf, report.Resources)

		// Generate PDF bytes
	var buf bytes.Buffer
	err := pdf.Output(&buf)
	if err != nil {
		return nil, fmt.Errorf("failed to generate PDF: %w", err)
	}

	return buf.Bytes(), nil
}

func (g *Generator) addTitle(pdf *gofpdf.Fpdf, title string) {
	pdf.SetFont("Arial", "B", 20)
	pdf.SetTextColor(0, 50, 100)

	// Center the title
	pageWidth, _ := pdf.GetPageSize()
	titleWidth := pdf.GetStringWidth(title)
	x := (pageWidth - titleWidth) / 2

	pdf.SetXY(x, 20)
	pdf.Cell(titleWidth, 15, title)
	pdf.Ln(25)
}

func (g *Generator) addGenerationInfo(pdf *gofpdf.Fpdf, generatedAt time.Time) {
	pdf.SetFont("Arial", "", 10)
	pdf.SetTextColor(100, 100, 100)

	info := fmt.Sprintf("Generated: %s", generatedAt.Format("2006-01-02 15:04:05"))
	pdf.Cell(0, 8, info)
	pdf.Ln(15)
}

func (g *Generator) addSummary(pdf *gofpdf.Fpdf, summary models.Summary) {
	// Section title
	pdf.SetFont("Arial", "B", 14)
	pdf.SetTextColor(0, 0, 0)
	pdf.Cell(0, 10, "Summary")
	pdf.Ln(12)

	// Summary data
	pdf.SetFont("Arial", "", 10)

	summaryData := [][]string{
		{"Projects", strconv.Itoa(summary.TotalProjects)},
		{"Virtual Machines", strconv.Itoa(summary.TotalServers)},
		{"Volumes", strconv.Itoa(summary.TotalVolumes)},
		{"Load Balancers", strconv.Itoa(summary.TotalLoadBalancers)},
		{"Floating IPs", strconv.Itoa(summary.TotalFloatingIPs)},
		{"VPN Services", strconv.Itoa(summary.TotalVPNServices)},
		{"Clusters", strconv.Itoa(summary.TotalClusters)},
		{"Routers", strconv.Itoa(summary.TotalRouters)},
	}

	// Create summary table
	colWidth := 80.0
	for _, row := range summaryData {
		pdf.SetFillColor(240, 240, 240)
		pdf.CellFormat(colWidth, 8, row[0], "1", 0, "L", true, 0, "")
		pdf.CellFormat(colWidth, 8, row[1], "1", 1, "R", false, 0, "")
	}

	pdf.Ln(10)
}

func (g *Generator) addProjectsSection(pdf *gofpdf.Fpdf, projects []models.Project) {
	// Section title
	pdf.SetFont("Arial", "B", 14)
	pdf.SetTextColor(0, 0, 0)
	pdf.Cell(0, 10, "Projects")
	pdf.Ln(12)

	if len(projects) == 0 {
		pdf.SetFont("Arial", "I", 10)
		pdf.Cell(0, 8, "No projects found")
		pdf.Ln(15)
		return
	}

	// Table header
	pdf.SetFont("Arial", "B", 10)
	pdf.SetFillColor(200, 200, 200)
	pdf.CellFormat(60, 8, "Name", "1", 0, "L", true, 0, "")
	pdf.CellFormat(40, 8, "ID", "1", 0, "L", true, 0, "")
	pdf.CellFormat(60, 8, "Description", "1", 0, "L", true, 0, "")
	pdf.CellFormat(30, 8, "Enabled", "1", 1, "C", true, 0, "")

	// Table data
	pdf.SetFont("Arial", "", 9)
	pdf.SetFillColor(255, 255, 255)

	for _, project := range projects {
		description := project.Description
		if len(description) > 30 {
			description = description[:30] + "..."
		}

		enabledText := "No"
		if project.Enabled {
			enabledText = "Yes"
		}

		pdf.CellFormat(60, 6, g.truncateString(project.Name, 25), "1", 0, "L", false, 0, "")
		pdf.CellFormat(40, 6, g.truncateString(project.ID, 15), "1", 0, "L", false, 0, "")
		pdf.CellFormat(60, 6, description, "1", 0, "L", false, 0, "")
		pdf.CellFormat(30, 6, enabledText, "1", 1, "C", false, 0, "")
	}

	pdf.Ln(10)
}

func (g *Generator) addResourcesByType(pdf *gofpdf.Fpdf, resources []models.Resource) {
	// Group resources by type
	typeGroups := make(map[string][]models.Resource)
	for _, resource := range resources {
		typeGroups[resource.Type] = append(typeGroups[resource.Type], resource)
	}

	// Section title
	pdf.SetFont("Arial", "B", 14)
	pdf.SetTextColor(0, 0, 0)
	pdf.Cell(0, 10, "Resources by Type")
	pdf.Ln(12)

	// Sort types for consistent output
	var types []string
	for resourceType := range typeGroups {
		types = append(types, resourceType)
	}
	sort.Strings(types)

	for _, resourceType := range types {
		resources := typeGroups[resourceType]

		// Type subsection
		pdf.SetFont("Arial", "B", 12)
		pdf.Cell(0, 8, fmt.Sprintf("%s (%d)", g.getTypeDisplayName(resourceType), len(resources)))
		pdf.Ln(10)

		// Group by project within type
		projectGroups := make(map[string]int)
		for _, resource := range resources {
			projectGroups[resource.ProjectName]++
		}

		pdf.SetFont("Arial", "", 10)
		for projectName, count := range projectGroups {
			pdf.CellFormat(10, 6, "", "", 0, "L", false, 0, "") // Indent
			pdf.CellFormat(0, 6, fmt.Sprintf("%s: %d", projectName, count), "", 1, "L", false, 0, "")
		}
		pdf.Ln(5)
	}

	pdf.Ln(5)
}

func (g *Generator) addResourcesTable(pdf *gofpdf.Fpdf, resources []models.Resource) {
	// Add new page for detailed table
	pdf.AddPage()

	// Section title
	pdf.SetFont("Arial", "B", 14)
	pdf.SetTextColor(0, 0, 0)
	pdf.Cell(0, 10, "Detailed Resources")
	pdf.Ln(12)

	if len(resources) == 0 {
		pdf.SetFont("Arial", "I", 10)
		pdf.Cell(0, 8, "No resources found")
		return
	}

	// Table header
	pdf.SetFont("Arial", "B", 8)
	pdf.SetFillColor(200, 200, 200)
	pdf.CellFormat(40, 6, "Name", "1", 0, "L", true, 0, "")
	pdf.CellFormat(25, 6, "Type", "1", 0, "L", true, 0, "")
	pdf.CellFormat(35, 6, "Project", "1", 0, "L", true, 0, "")
	pdf.CellFormat(20, 6, "Status", "1", 0, "C", true, 0, "")
	pdf.CellFormat(25, 6, "Created", "1", 0, "C", true, 0, "")
	pdf.CellFormat(45, 6, "ID", "1", 1, "L", true, 0, "")

	// Sort resources by project, then by type, then by name
	sort.Slice(resources, func(i, j int) bool {
		if resources[i].ProjectName != resources[j].ProjectName {
			return resources[i].ProjectName < resources[j].ProjectName
		}
		if resources[i].Type != resources[j].Type {
			return resources[i].Type < resources[j].Type
		}
		return resources[i].Name < resources[j].Name
	})

	// Table data
	pdf.SetFont("Arial", "", 7)
	pdf.SetFillColor(255, 255, 255)

	for _, resource := range resources {
		name := resource.Name
		if name == "" {
			name = "unnamed"
		}

		createdDate := resource.CreatedAt.Format("2006-01-02")

		pdf.CellFormat(40, 5, g.truncateString(name, 20), "1", 0, "L", false, 0, "")
		pdf.CellFormat(25, 5, g.truncateString(g.getTypeDisplayName(resource.Type), 12), "1", 0, "L", false, 0, "")
		pdf.CellFormat(35, 5, g.truncateString(resource.ProjectName, 18), "1", 0, "L", false, 0, "")
		pdf.CellFormat(20, 5, g.truncateString(resource.Status, 10), "1", 0, "C", false, 0, "")
		pdf.CellFormat(25, 5, createdDate, "1", 0, "C", false, 0, "")
		pdf.CellFormat(45, 5, g.truncateString(resource.ID, 25), "1", 1, "L", false, 0, "")
	}
}

func (g *Generator) getTypeDisplayName(resourceType string) string {
	types := map[string]string{
		"server":         "Virtual Machine",
		"volume":         "Volume",
		"floating_ip":    "Floating IP",
		"router":         "Router",
		"load_balancer":  "Load Balancer",
		"vpn_service":    "VPN Service",
		"cluster":        "K8s Cluster",
	}

	if displayName, exists := types[resourceType]; exists {
		return displayName
	}
	return resourceType
}

func (g *Generator) truncateString(str string, maxLen int) string {
	if len(str) <= maxLen {
		return str
	}
	return str[:maxLen-3] + "..."
}
