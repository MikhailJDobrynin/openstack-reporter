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

	// Add detailed resources by project and type
	g.addDetailedResourcesByProject(pdf, report.Resources)

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
		{"Networks", strconv.Itoa(summary.TotalNetworks)},
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

func (g *Generator) addDetailedResourcesByProject(pdf *gofpdf.Fpdf, resources []models.Resource) {
	// Add new page for detailed resources
	pdf.AddPage()

	// Section title
	pdf.SetFont("Arial", "B", 14)
	pdf.SetTextColor(0, 0, 0)
	pdf.Cell(0, 10, "Detailed Resources")
	pdf.Ln(15)

	if len(resources) == 0 {
		pdf.SetFont("Arial", "I", 10)
		pdf.Cell(0, 8, "No resources found")
		return
	}

	// Group resources by project
	projectGroups := make(map[string][]models.Resource)
	for _, resource := range resources {
		projectGroups[resource.ProjectName] = append(projectGroups[resource.ProjectName], resource)
	}

	// Sort projects alphabetically
	var projects []string
	for projectName := range projectGroups {
		projects = append(projects, projectName)
	}
	sort.Strings(projects)

	for _, projectName := range projects {
		resources := projectGroups[projectName]

		// Project subsection
		pdf.SetFont("Arial", "B", 12)
		pdf.Cell(0, 8, fmt.Sprintf("Project: %s (%d resources)", projectName, len(resources)))
		pdf.Ln(10)

		// Group resources by type within project
		typeGroups := make(map[string][]models.Resource)
		for _, resource := range resources {
			typeGroups[resource.Type] = append(typeGroups[resource.Type], resource)
		}

		// Sort types for consistent output
		var types []string
		for resourceType := range typeGroups {
			types = append(types, resourceType)
		}
		sort.Strings(types)

		for _, resourceType := range types {
			resources := typeGroups[resourceType]

			// Type subsection
			pdf.SetFont("Arial", "B", 10)
			pdf.Cell(0, 8, fmt.Sprintf("%s (%d)", g.getTypeDisplayName(resourceType), len(resources)))
			pdf.Ln(10)

			// Sort resources by creation date
			sort.Slice(resources, func(i, j int) bool {
				return resources[i].CreatedAt.Before(resources[j].CreatedAt)
			})

			// Table header
			pdf.SetFont("Arial", "B", 9)
			pdf.SetFillColor(200, 200, 200)
			pdf.CellFormat(80, 7, "Name", "1", 0, "L", true, 0, "")
			pdf.CellFormat(30, 7, "Status", "1", 0, "C", true, 0, "")
			pdf.CellFormat(35, 7, "Created", "1", 1, "C", true, 0, "")

			// Table data
			pdf.SetFont("Arial", "", 8)
			pdf.SetFillColor(255, 255, 255)

			for _, resource := range resources {
				name := resource.Name
				if name == "" {
					name = "unnamed"
				}

				createdDate := resource.CreatedAt.Format("2006-01-02")

				// Add subnet info to name for networks
				displayName := name
				if resourceType == "network" {
					if network, ok := resource.Properties.(models.Network); ok {
						if len(network.Subnets) > 0 {
							subnetInfo := ""
							for i, subnet := range network.Subnets {
								if i >= 2 {
									subnetInfo += fmt.Sprintf(" (+%d)", len(network.Subnets)-2)
									break
								}
								if i > 0 {
									subnetInfo += ", "
								}
								subnetInfo += subnet.CIDR
							}
							displayName = fmt.Sprintf("%s\nSubnets: %s", name, subnetInfo)
						} else {
							displayName = fmt.Sprintf("%s\nNo subnets", name)
						}
					}
				}

				// Увеличиваем высоту ячейки для сетей с подсетями
				cellHeight := 6.0
				if resourceType == "network" {
					cellHeight = 8.0
				}
				
				pdf.CellFormat(80, cellHeight, g.truncateString(displayName, 60), "1", 0, "L", false, 0, "")
				pdf.CellFormat(30, cellHeight, g.truncateString(resource.Status, 12), "1", 0, "C", false, 0, "")
				pdf.CellFormat(35, cellHeight, createdDate, "1", 1, "C", false, 0, "")
			}
			pdf.Ln(5)
		}
		pdf.Ln(5)
	}
}

func (g *Generator) getTypeDisplayName(resourceType string) string {
	types := map[string]string{
		"server":         "Virtual Machine",
		"volume":         "Volume",
		"floating_ip":    "Floating IP",
		"router":         "Router",
		"network":        "Network",
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
