package openstack

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/blockstorage/v3/volumes"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/flavors"
	"github.com/gophercloud/gophercloud/openstack/identity/v3/projects"
	"github.com/gophercloud/gophercloud/openstack/loadbalancer/v2/loadbalancers"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/layer3/floatingips"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/layer3/routers"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/vpnaas/services"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/vpnaas/siteconnections"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/networks"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/subnets"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/ports"

	"openstack-reporter/internal/models"
)

// ProgressReporter interface for sending progress updates
type ProgressReporter interface {
	SendProgress(msgType, message string, currentStep, totalSteps int, project, resourceType string, count int, summary map[string]int)
}

// ProgressMessage represents a progress update message
type ProgressMessage struct {
	Type        string         `json:"type"`
	Message     string         `json:"message"`
	CurrentStep int            `json:"current_step,omitempty"`
	TotalSteps  int            `json:"total_steps,omitempty"`
	Project     string         `json:"project,omitempty"`
	ResourceType string        `json:"resource_type,omitempty"`
	Count       int            `json:"count,omitempty"`
	Summary     map[string]int `json:"summary,omitempty"`
}

// ChannelProgressReporter implements ProgressReporter using channels
type ChannelProgressReporter struct {
	progressChan chan ProgressMessage
}

func NewChannelProgressReporter(progressChan chan ProgressMessage) *ChannelProgressReporter {
	return &ChannelProgressReporter{
		progressChan: progressChan,
	}
}

func (r *ChannelProgressReporter) SendProgress(msgType, message string, currentStep, totalSteps int, project, resourceType string, count int, summary map[string]int) {
	if r.progressChan == nil {
		return
	}

	progressMsg := ProgressMessage{
		Type:         msgType,
		Message:      message,
		CurrentStep:  currentStep,
		TotalSteps:   totalSteps,
		Project:      project,
		ResourceType: resourceType,
		Count:        count,
		Summary:      summary,
	}

	select {
	case r.progressChan <- progressMsg:
	default:
		// Channel full, skip this update
	}
}

type Client struct {
	provider         *gophercloud.ProviderClient
	computeClient    *gophercloud.ServiceClient
	blockstorageClient *gophercloud.ServiceClient
	networkClient    *gophercloud.ServiceClient
	identityClient   *gophercloud.ServiceClient
	loadbalancerClient *gophercloud.ServiceClient
	containerClient  *gophercloud.ServiceClient
}

// NewClient creates a new OpenStack client
func NewClient() (*Client, error) {
	projectName := os.Getenv("OS_PROJECT_NAME")

	// If no project specified, use a default project for initialization
	// The actual multi-project logic will happen in GetAllResources()
	if projectName == "" {
		projectName = "infra" // Use a known project for initialization
		fmt.Printf("DEBUG: No OS_PROJECT_NAME specified, using '%s' for client initialization\n", projectName)
	}

	opts := gophercloud.AuthOptions{
		IdentityEndpoint: os.Getenv("OS_AUTH_URL"),
		Username:         os.Getenv("OS_USERNAME"),
		Password:         os.Getenv("OS_PASSWORD"),
		DomainName:       os.Getenv("OS_USER_DOMAIN_NAME"),
		TenantName:       projectName,
	}

	// Handle insecure connections
	var provider *gophercloud.ProviderClient
	var err error

	if os.Getenv("OS_INSECURE") == "true" {
		config := &tls.Config{InsecureSkipVerify: true}
		transport := &http.Transport{TLSClientConfig: config}
		client := &http.Client{Transport: transport}
		provider, err = openstack.NewClient(opts.IdentityEndpoint)
		if err != nil {
			return nil, fmt.Errorf("failed to create provider client: %w", err)
		}
		provider.HTTPClient = *client
		err = openstack.Authenticate(provider, opts)
	} else {
		provider, err = openstack.AuthenticatedClient(opts)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create authenticated client: %w", err)
	}

	computeClient, err := openstack.NewComputeV2(provider, gophercloud.EndpointOpts{
		Region: os.Getenv("OS_REGION_NAME"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create compute client: %w", err)
	}

	blockstorageClient, err := openstack.NewBlockStorageV3(provider, gophercloud.EndpointOpts{
		Region: os.Getenv("OS_REGION_NAME"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create block storage client: %w", err)
	}

	networkClient, err := openstack.NewNetworkV2(provider, gophercloud.EndpointOpts{
		Region: os.Getenv("OS_REGION_NAME"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create network client: %w", err)
	}

	identityClient, err := openstack.NewIdentityV3(provider, gophercloud.EndpointOpts{
		Region: os.Getenv("OS_REGION_NAME"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create identity client: %w", err)
	}

	loadbalancerClient, err := openstack.NewLoadBalancerV2(provider, gophercloud.EndpointOpts{
		Region: os.Getenv("OS_REGION_NAME"),
	})
	if err != nil {
		// Load balancer service might not be available
		loadbalancerClient = nil
	}

	containerClient, err := openstack.NewContainerInfraV1(provider, gophercloud.EndpointOpts{
		Region: os.Getenv("OS_REGION_NAME"),
	})
	if err != nil {
		// Container service might not be available
		containerClient = nil
	}

	return &Client{
		provider:           provider,
		computeClient:      computeClient,
		blockstorageClient: blockstorageClient,
		networkClient:      networkClient,
		identityClient:     identityClient,
		loadbalancerClient: loadbalancerClient,
		containerClient:    containerClient,
	}, nil
}

// GetAllResources fetches all resources from OpenStack
func (c *Client) GetAllResources() (*models.ResourceReport, error) {
	report := &models.ResourceReport{
		GeneratedAt: time.Now(),
		Resources:   []models.Resource{},
	}

	// Check if user wants all projects or specific project
	projectName := strings.TrimSpace(os.Getenv("OS_PROJECT_NAME"))
	if projectName != "" {
		// Single project mode - use current client
		fmt.Printf("DEBUG: Single project mode: %s\n", projectName)
		currentProject, err := c.getCurrentProject()
		if err != nil {
			return nil, fmt.Errorf("failed to get current project: %w", err)
		}
		report.Projects = []models.Project{currentProject}

		// Create project name mapping
		projectNames := make(map[string]string)
		projectNames[currentProject.ID] = currentProject.Name

		// Get resources for current project
		return c.collectResourcesForProjects(report, projectNames)
	}

	// Multi-project mode - get all projects via API with domain-scoped token
	fmt.Printf("DEBUG: Multi-project mode - getting all accessible projects via API\n")
	allProjects, err := c.getProjectsViaAPI()
	if err != nil {
		fmt.Printf("DEBUG: API project list failed: %v\n", err)
		// Fallback to CLI method
		fmt.Printf("DEBUG: Trying CLI fallback...\n")
		allProjects, err = getProjectsViaCommand()
		if err != nil {
			fmt.Printf("DEBUG: CLI project list also failed: %v\n", err)
			// Final fallback to current project
			currentProject, fallbackErr := c.getCurrentProject()
			if fallbackErr != nil {
				return nil, fmt.Errorf("failed to get projects: %w", err)
			}
			report.Projects = []models.Project{currentProject}
			projectNames := make(map[string]string)
			projectNames[currentProject.ID] = currentProject.Name
			return c.collectResourcesForProjects(report, projectNames)
		}
	}

	report.Projects = allProjects
	fmt.Printf("DEBUG: Found %d projects, collecting resources from each\n", len(allProjects))

	// Collect resources from each project separately
	var allResources []models.Resource
	totalProjects := len(allProjects)

	for i, project := range allProjects {
		fmt.Printf("🔍 [%d/%d] Collecting resources from project: %s (%s)\n", i+1, totalProjects, project.Name, project.ID)

		projectResources, err := getResourcesForProject(project)
		if err != nil {
			fmt.Printf("❌ Failed to get resources for project %s: %v\n", project.Name, err)
			continue // Skip this project, continue with others
		}

		fmt.Printf("✅ Found %d resources in project %s\n", len(projectResources), project.Name)
		allResources = append(allResources, projectResources...)
	}

	report.Resources = allResources
	report.Summary = c.calculateSummary(report.Resources, len(report.Projects))

	fmt.Printf("\n🎯 SUMMARY: Total %d resources collected from %d projects\n", len(allResources), len(allProjects))

	// Show breakdown by resource type
	typeCount := make(map[string]int)
	for _, resource := range allResources {
		typeCount[resource.Type]++
	}

	fmt.Printf("📊 Resource breakdown:\n")
	for resourceType, count := range typeCount {
		fmt.Printf("   - %s: %d\n", resourceType, count)
	}

	return report, nil
}

// GetAllResourcesWithProgress fetches all resources from OpenStack with progress updates
func (c *Client) GetAllResourcesWithProgress(progressChan chan ProgressMessage) (*models.ResourceReport, error) {
	reporter := NewChannelProgressReporter(progressChan)

	report := &models.ResourceReport{
		GeneratedAt: time.Now(),
		Resources:   []models.Resource{},
	}

	// Check if user wants all projects or specific project
	projectName := strings.TrimSpace(os.Getenv("OS_PROJECT_NAME"))
	if projectName != "" {
		// Single project mode - use current client
		reporter.SendProgress("progress", "Single project mode: "+projectName, 0, 0, "", "", 0, nil)
		currentProject, err := c.getCurrentProject()
		if err != nil {
			return nil, fmt.Errorf("failed to get current project: %w", err)
		}
		report.Projects = []models.Project{currentProject}

		// Create project name mapping
		projectNames := make(map[string]string)
		projectNames[currentProject.ID] = currentProject.Name

		// Get resources for current project
		return c.collectResourcesForProjectsWithProgress(report, projectNames, reporter)
	}

	// Multi-project mode - get all projects via API with domain-scoped token
	reporter.SendProgress("progress", "Multi-project mode - getting accessible projects", 0, 0, "", "", 0, nil)
	fmt.Printf("DEBUG: Attempting to get projects via API...\n")
	allProjects, err := c.getProjectsViaAPI()
	if err != nil {
		fmt.Printf("DEBUG: API project list failed: %v\n", err)
		reporter.SendProgress("progress", "API project list failed, trying CLI fallback", 0, 0, "", "", 0, nil)
		fmt.Printf("DEBUG: Attempting to get projects via CLI...\n")
		allProjects, err = getProjectsViaCommand()
		if err != nil {
			fmt.Printf("DEBUG: CLI project list failed: %v\n", err)
			fmt.Printf("DEBUG: Using single project fallback mode\n")
			reporter.SendProgress("progress", "CLI project list failed, using fallback", 0, 0, "", "", 0, nil)
			// Final fallback to current project
			currentProject, fallbackErr := c.getCurrentProject()
			if fallbackErr != nil {
				return nil, fmt.Errorf("failed to get projects: %w", err)
			}
			report.Projects = []models.Project{currentProject}
			projectNames := make(map[string]string)
			projectNames[currentProject.ID] = currentProject.Name
			return c.collectResourcesForProjectsWithProgress(report, projectNames, reporter)
		}
	}

	report.Projects = allProjects
	fmt.Printf("DEBUG: Successfully found %d projects via API/CLI, entering true multi-project mode\n", len(allProjects))
	reporter.SendProgress("progress", fmt.Sprintf("Found %d projects, starting resource collection", len(allProjects)), 0, len(allProjects), "", "", 0, nil)

	// Collect resources from each project separately
	var allResources []models.Resource
	totalProjects := len(allProjects)

	for i, project := range allProjects {
		reporter.SendProgress("project_start", fmt.Sprintf("Collecting resources from project: %s", project.Name), i+1, totalProjects, project.Name, "", 0, nil)

		projectResources, err := getResourcesForProjectWithProgress(project, reporter)
		if err != nil {
			reporter.SendProgress("project_error", fmt.Sprintf("Failed to get resources for project %s: %v", project.Name, err), i+1, totalProjects, project.Name, "", 0, nil)
			continue // Skip this project, continue with others
		}

		reporter.SendProgress("project_complete", fmt.Sprintf("Found %d resources in project %s", len(projectResources), project.Name), i+1, totalProjects, project.Name, "", len(projectResources), nil)
		allResources = append(allResources, projectResources...)
	}

	report.Resources = allResources
	report.Summary = c.calculateSummary(report.Resources, len(report.Projects))

	// Send final summary
	typeCount := make(map[string]int)
	for _, resource := range allResources {
		typeCount[resource.Type]++
	}

	reporter.SendProgress("summary", fmt.Sprintf("Total %d resources collected from %d projects", len(allResources), len(allProjects)), totalProjects, totalProjects, "", "", len(allResources), typeCount)

	return report, nil
}

func (c *Client) getAllProjects() ([]models.Project, error) {
	// Try to list all projects the user has access to
	fmt.Printf("DEBUG: Attempting to list all accessible projects...\n")
	allPages, err := projects.List(c.identityClient, projects.ListOpts{}).AllPages()
	if err != nil {
		fmt.Printf("DEBUG: Failed to list projects: %v\n", err)
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}

	projectList, err := projects.ExtractProjects(allPages)
	if err != nil {
		return nil, fmt.Errorf("failed to extract projects: %w", err)
	}

	var result []models.Project
	for _, project := range projectList {
		result = append(result, models.Project{
			ID:          project.ID,
			Name:        project.Name,
			Description: project.Description,
			DomainID:    project.DomainID,
			Enabled:     project.Enabled,
		})
	}

	if len(result) == 0 {
		fmt.Printf("DEBUG: No projects found for user\n")
		return nil, fmt.Errorf("no projects accessible to user")
	}

	fmt.Printf("DEBUG: Found %d accessible projects: ", len(result))
	for i, project := range result {
		if i > 0 {
			fmt.Printf(", ")
		}
		fmt.Printf("%s (%s)", project.Name, project.ID)
	}
	fmt.Printf("\n")

	return result, nil
}

func (c *Client) getCurrentProject() (models.Project, error) {
	// Get current token to extract project info
	authResult := c.provider.GetAuthResult()
	if authResult == nil {
		return models.Project{}, fmt.Errorf("no authentication result available")
	}

	// Use simple approach - get project from environment or use fallback
	projectID := os.Getenv("OS_PROJECT_ID")
	projectName := os.Getenv("OS_PROJECT_NAME")

	if projectName == "" {
		projectName = "Current Project"
	}

		if projectID == "" {
		projectID = "current-project"
	}

	// Try to get project details only if we have an ID
	if projectID != "current-project" {
		project, err := projects.Get(c.identityClient, projectID).Extract()
		if err == nil {
			return models.Project{
				ID:          project.ID,
				Name:        project.Name,
				Description: project.Description,
				DomainID:    project.DomainID,
				Enabled:     project.Enabled,
			}, nil
		}
	}

	// Fallback to basic info
	return models.Project{
		ID:          projectID,
		Name:        projectName,
		Description: "Current working project",
		Enabled:     true,
	}, nil
}

func (c *Client) getServers(projectNames map[string]string) ([]models.Resource, error) {
	// Get current project info for fallback
	currentProject, _ := c.getCurrentProject()

	// Check if user wants all projects or specific project
	var listOpts servers.ListOpts
	projectName := strings.TrimSpace(os.Getenv("OS_PROJECT_NAME"))
	if projectName == "" {
		// No specific project requested - get all accessible projects
		fmt.Printf("DEBUG: No specific project set, using AllTenants=true\n")
		listOpts = servers.ListOpts{AllTenants: true}
	} else {
		// Specific project requested - get only current project resources
		fmt.Printf("DEBUG: Project '%s' specified, getting project-scoped resources\n", projectName)
		listOpts = servers.ListOpts{}
	}

	allPages, err := servers.List(c.computeClient, listOpts).AllPages()
	if err != nil && listOpts.AllTenants {
		// Fallback to current tenant only if AllTenants fails
		fmt.Printf("DEBUG: AllTenants failed, falling back to current project only\n")
		listOpts = servers.ListOpts{}
		allPages, err = servers.List(c.computeClient, listOpts).AllPages()
	}
	if err != nil {
		return nil, err
	}

	serverList, err := servers.ExtractServers(allPages)
	if err != nil {
		return nil, err
	}

	var resources []models.Resource
	for _, server := range serverList {
		created := server.Created
		updated := server.Updated

		// Get project name, fallback to current project if not found
		projectName := projectNames[server.TenantID]
		projectID := server.TenantID
		if projectName == "" {
			projectName = currentProject.Name
			projectID = currentProject.ID
		}

				// Get detailed flavor information
		flavorName, flavorID := c.getFlavorDetails(server.Flavor)

		resources = append(resources, models.Resource{
			ID:          server.ID,
			Name:        server.Name,
			Type:        "server",
			ProjectID:   projectID,
			ProjectName: projectName,
			Status:      server.Status,
			CreatedAt:   created,
			UpdatedAt:   updated,
			Properties: models.Server{
				ID:         server.ID,
				Name:       server.Name,
				Status:     server.Status,
				FlavorName: flavorName,
				FlavorID:   flavorID,
				Networks:   extractNetworks(server.Addresses),
				CreatedAt:  created,
				UpdatedAt:  updated,
			},
		})
	}

	return resources, nil
}

func (c *Client) getVolumes(projectNames map[string]string) ([]models.Resource, error) {
	currentProject, _ := c.getCurrentProject()

	// Check if user wants all projects or specific project
	var listOpts volumes.ListOpts
	projectName := strings.TrimSpace(os.Getenv("OS_PROJECT_NAME"))
	if projectName == "" {
		// No specific project requested - get all accessible projects
		fmt.Printf("DEBUG: No specific project set, using AllTenants=true for volumes\n")
		listOpts = volumes.ListOpts{AllTenants: true}
	} else {
		// Specific project requested - get only current project resources
		fmt.Printf("DEBUG: Project '%s' specified, getting project-scoped volumes\n", projectName)
		listOpts = volumes.ListOpts{}
	}

	allPages, err := volumes.List(c.blockstorageClient, listOpts).AllPages()
	if err != nil && listOpts.AllTenants {
		// Fallback to current tenant only if AllTenants fails
		fmt.Printf("DEBUG: AllTenants failed for volumes, falling back to current project only\n")
		listOpts = volumes.ListOpts{}
		allPages, err = volumes.List(c.blockstorageClient, listOpts).AllPages()
	}
	if err != nil {
		return nil, err
	}

	volumeList, err := volumes.ExtractVolumes(allPages)
	if err != nil {
		return nil, err
	}

	var resources []models.Resource
	for _, volume := range volumeList {
		created := volume.CreatedAt

		// Use current project as fallback when we can't determine the actual project
		projectID := currentProject.ID
		projectName := currentProject.Name

		// Get detailed attachment information including server names
		attachments := c.getVolumeAttachments(volume.Attachments)
		attachedTo := ""
		if len(attachments) > 0 {
			attachedTo = attachments[0].ServerName
		}

		resources = append(resources, models.Resource{
			ID:          volume.ID,
			Name:        volume.Name,
			Type:        "volume",
			ProjectID:   projectID,
			ProjectName: projectName,
			Status:      volume.Status,
			CreatedAt:   created,
			Properties: models.Volume{
				ID:          volume.ID,
				Name:        volume.Name,
				Status:      volume.Status,
				Size:        volume.Size,
				VolumeType:  volume.VolumeType,
				Bootable:    volume.Bootable == "true",
				Attachments: attachments,
				AttachedTo:  attachedTo,
				CreatedAt:   created,
			},
		})
	}

	return resources, nil
}

func (c *Client) getVolumesForProject(projectID, projectName string) ([]models.Resource, error) {
	// Use TenantID filter to get volumes for specific project
	listOpts := volumes.ListOpts{
		AllTenants: true,
		TenantID:   projectID,
	}

	allPages, err := volumes.List(c.blockstorageClient, listOpts).AllPages()
	if err != nil {
		return nil, err
	}

	volumeList, err := volumes.ExtractVolumes(allPages)
	if err != nil {
		return nil, err
	}

	var resources []models.Resource
	for _, volume := range volumeList {
		created := volume.CreatedAt

		// Get detailed attachment information including server names
		attachments := c.getVolumeAttachments(volume.Attachments)
		attachedTo := ""
		if len(attachments) > 0 {
			attachedTo = attachments[0].ServerName
		}

		resources = append(resources, models.Resource{
			ID:          volume.ID,
			Name:        volume.Name,
			Type:        "volume",
			ProjectID:   projectID,
			ProjectName: projectName,
			Status:      volume.Status,
			CreatedAt:   created,
			Properties: models.Volume{
				ID:          volume.ID,
				Name:        volume.Name,
				Status:      volume.Status,
				Size:        volume.Size,
				VolumeType:  volume.VolumeType,
				Bootable:    volume.Bootable == "true",
				Attachments: attachments,
				AttachedTo:  attachedTo,
				CreatedAt:   created,
			},
		})
	}

	fmt.Printf("DEBUG: Found %d volumes for project %s (%s)\n", len(resources), projectName, projectID)
	return resources, nil
}

func (c *Client) getFloatingIPs(projectNames map[string]string) ([]models.Resource, error) {
	// Get current project info for fallback
	currentProject, _ := c.getCurrentProject()
	allPages, err := floatingips.List(c.networkClient, floatingips.ListOpts{}).AllPages()
	if err != nil {
		return nil, err
	}

	floatingIPList, err := floatingips.ExtractFloatingIPs(allPages)
	if err != nil {
		return nil, err
	}

	var resources []models.Resource
	for _, fip := range floatingIPList {
		created := fip.CreatedAt
		updated := fip.UpdatedAt

		// Get project name, fallback to current project if not found
		projectName := projectNames[fip.TenantID]
		projectID := fip.TenantID
		if projectName == "" {
			projectName = currentProject.Name
			projectID = currentProject.ID
		}

		// Get attached resource name if floating IP is attached
		attachedResourceName := c.getAttachedResourceName(fip.PortID)

		resources = append(resources, models.Resource{
			ID:          fip.ID,
			Name:        fip.FloatingIP,
			Type:        "floating_ip",
			ProjectID:   projectID,
			ProjectName: projectName,
			Status:      fip.Status,
			CreatedAt:   created,
			UpdatedAt:   updated,
			Properties: models.FloatingIP{
				ID:                   fip.ID,
				FloatingIP:           fip.FloatingIP,
				Status:               fip.Status,
				FixedIP:              fip.FixedIP,
				PortID:               fip.PortID,
				AttachedResourceName: attachedResourceName,
				FloatingNetworkID:    fip.FloatingNetworkID,
				CreatedAt:            created,
				UpdatedAt:            updated,
			},
		})
	}

	return resources, nil
}

func (c *Client) getRouters(projectNames map[string]string) ([]models.Resource, error) {
	// Get current project info for fallback
	currentProject, _ := c.getCurrentProject()
	allPages, err := routers.List(c.networkClient, routers.ListOpts{}).AllPages()
	if err != nil {
		return nil, err
	}

	routerList, err := routers.ExtractRouters(allPages)
	if err != nil {
		return nil, err
	}

	var resources []models.Resource
	for _, router := range routerList {
		created := time.Now() // Router API may not provide created time
		updated := time.Now()

		// Get project name, fallback to current project if not found
		projectName := projectNames[router.TenantID]
		projectID := router.TenantID
		if projectName == "" {
			projectName = currentProject.Name
			projectID = currentProject.ID
		}

		resources = append(resources, models.Resource{
			ID:          router.ID,
			Name:        router.Name,
			Type:        "router",
			ProjectID:   projectID,
			ProjectName: projectName,
			Status:      router.Status,
			CreatedAt:   created,
			UpdatedAt:   updated,
			Properties: models.Router{
				ID:                  router.ID,
				Name:                router.Name,
				Status:              router.Status,
				AdminStateUp:        router.AdminStateUp,
				ExternalGatewayInfo: convertGatewayInfo(router.GatewayInfo),
				Routes:              convertRoutes(router.Routes),
				CreatedAt:           created,
				UpdatedAt:           updated,
			},
		})
	}

	return resources, nil
}

func (c *Client) getNetworks(projectNames map[string]string) ([]models.Resource, error) {
	// Get current project info for fallback
	currentProject, _ := c.getCurrentProject()
	allPages, err := networks.List(c.networkClient, networks.ListOpts{}).AllPages()
	if err != nil {
		return nil, err
	}

	networkList, err := networks.ExtractNetworks(allPages)
	if err != nil {
		return nil, err
	}

	var resources []models.Resource
	for _, network := range networkList {
		created := network.CreatedAt
		updated := network.UpdatedAt

		// Get project name, fallback to current project if not found
		projectName := projectNames[network.TenantID]
		projectID := network.TenantID
		if projectName == "" {
			projectName = currentProject.Name
			projectID = currentProject.ID
		}

		// Get detailed subnet information
		subnets := c.getSubnetsForNetwork(network.ID)

		resources = append(resources, models.Resource{
			ID:          network.ID,
			Name:        network.Name,
			Type:        "network",
			ProjectID:   projectID,
			ProjectName: projectName,
			Status:      network.Status,
			CreatedAt:   created,
			UpdatedAt:   updated,
			Properties: models.Network{
				ID:           network.ID,
				Name:         network.Name,
				Status:       network.Status,
				AdminStateUp: network.AdminStateUp,
				Shared:       network.Shared,
				External:     false, // Default value, not available in basic Network struct
				NetworkType:  "local", // Default value, not available in basic Network struct
				Subnets:      subnets,
				CreatedAt:    created,
				UpdatedAt:    updated,
			},
		})
	}

	return resources, nil
}

func (c *Client) getLoadBalancers(projectNames map[string]string) ([]models.Resource, error) {
	if c.loadbalancerClient == nil {
		return []models.Resource{}, nil
	}

	// Get current project info for fallback
	currentProject, _ := c.getCurrentProject()

	allPages, err := loadbalancers.List(c.loadbalancerClient, loadbalancers.ListOpts{}).AllPages()
	if err != nil {
		return nil, err
	}

	lbList, err := loadbalancers.ExtractLoadBalancers(allPages)
	if err != nil {
		return nil, err
	}

	var resources []models.Resource
	for _, lb := range lbList {
		created := lb.CreatedAt
		updated := lb.UpdatedAt

		// Get project name, fallback to current project if not found
		projectName := projectNames[lb.ProjectID]
		projectID := lb.ProjectID
		if projectName == "" {
			projectName = currentProject.Name
			projectID = currentProject.ID
		}

		resources = append(resources, models.Resource{
			ID:          lb.ID,
			Name:        lb.Name,
			Type:        "load_balancer",
			ProjectID:   projectID,
			ProjectName: projectName,
			Status:      lb.ProvisioningStatus,
			CreatedAt:   created,
			UpdatedAt:   updated,
			Properties: models.LoadBalancer{
				ID:                 lb.ID,
				Name:               lb.Name,
				Description:        lb.Description,
				ProvisioningStatus: lb.ProvisioningStatus,
				OperatingStatus:    lb.OperatingStatus,
				VipAddress:         lb.VipAddress,
				VipSubnetID:        lb.VipSubnetID,
				CreatedAt:          created,
				UpdatedAt:          updated,
			},
		})
	}

	return resources, nil
}

func (c *Client) getSubnetsForNetwork(networkID string) []models.Subnet {
	// List subnets for the specific network
	listOpts := subnets.ListOpts{
		NetworkID: networkID,
	}

	allPages, err := subnets.List(c.networkClient, listOpts).AllPages()
	if err != nil {
		fmt.Printf("DEBUG: Failed to get subnets for network %s: %v\n", networkID, err)
		return []models.Subnet{}
	}

	subnetList, err := subnets.ExtractSubnets(allPages)
	if err != nil {
		fmt.Printf("DEBUG: Failed to extract subnets for network %s: %v\n", networkID, err)
		return []models.Subnet{}
	}

	var result []models.Subnet
	for _, subnet := range subnetList {
		result = append(result, models.Subnet{
			ID:        subnet.ID,
			Name:      subnet.Name,
			CIDR:      subnet.CIDR,
			GatewayIP: subnet.GatewayIP,
		})
	}

	return result
}

func (c *Client) getVPNServices(projectNames map[string]string) ([]models.Resource, error) {
	// Get current project info for fallback
	currentProject, _ := c.getCurrentProject()
	allPages, err := services.List(c.networkClient, services.ListOpts{}).AllPages()
	if err != nil {
		return []models.Resource{}, nil // VPN might not be available
	}

	vpnList, err := services.ExtractServices(allPages)
	if err != nil {
		return []models.Resource{}, nil
	}

	var resources []models.Resource
	for _, vpn := range vpnList {
		created := time.Now() // VPN API may not provide created time

		// Get project name, fallback to current project if not found
		projectName := projectNames[vpn.TenantID]
		projectID := vpn.TenantID
		if projectName == "" {
			projectName = currentProject.Name
			projectID = currentProject.ID
		}

		// Get VPN peer ID if available
		peerID := c.getVPNPeerID(vpn.ID)

		resources = append(resources, models.Resource{
			ID:          vpn.ID,
			Name:        vpn.Name,
			Type:        "vpn_service",
			ProjectID:   projectID,
			ProjectName: projectName,
			Status:      vpn.Status,
			CreatedAt:   created,
			Properties: models.VPNService{
				ID:          vpn.ID,
				Name:        vpn.Name,
				Description: vpn.Description,
				Status:      vpn.Status,
				RouterID:    vpn.RouterID,
				SubnetID:    vpn.SubnetID,
				PeerID:      peerID,
				CreatedAt:   created,
			},
		})
	}

	return resources, nil
}

func (c *Client) getVPNConnections(projectNames map[string]string) ([]models.Resource, error) {
	// Get current project info for fallback
	currentProject, _ := c.getCurrentProject()

		// Get VPN IPSec site connections
	allPages, err := siteconnections.List(c.networkClient, siteconnections.ListOpts{}).AllPages()
	if err != nil {
		return []models.Resource{}, fmt.Errorf("failed to list VPN site connections: %w", err)
	}

	connectionList, err := siteconnections.ExtractConnections(allPages)
	if err != nil {
		return []models.Resource{}, fmt.Errorf("failed to extract VPN site connections: %w", err)
	}

	var resources []models.Resource
	for _, conn := range connectionList {
		created := time.Now() // VPN Connection API may not provide created time

		// Get project name, fallback to current project if not found
		projectName := projectNames[conn.TenantID]
		projectID := conn.TenantID
		if projectName == "" {
			projectName = currentProject.Name
			projectID = currentProject.ID
		}

		// Use connection name as VPN name
		name := conn.Name
		if name == "" {
			name = fmt.Sprintf("vpn-connection-%s", conn.ID[:8])
		}

		resources = append(resources, models.Resource{
			ID:          conn.ID,
			Name:        name,
			Type:        "vpn_service",
			ProjectID:   projectID,
			ProjectName: projectName,
			Status:      conn.Status,
			CreatedAt:   created,
			Properties: models.VPNService{
				ID:              conn.ID,
				Name:            name,
				Description:     conn.Description,
				Status:          conn.Status,
				RouterID:        conn.VPNServiceID, // Link to parent VPN service
				SubnetID:        "",                // Not available in site connection
				PeerID:          conn.PeerID,
				PeerAddress:     conn.PeerAddress,
				AuthMode:        conn.AuthMode,
				IKEVersion:      "",               // Available in IKE policy, not connection
				MTU:             conn.MTU,
				CreatedAt:       created,
			},
		})
	}

	return resources, nil
}

func (c *Client) getClusters(projectNames map[string]string) ([]models.Resource, error) {
	if c.containerClient == nil {
		return []models.Resource{}, nil
	}

	// Note: This is a placeholder as container infra API might vary
	// Implementation depends on the specific OpenStack deployment
	return []models.Resource{}, nil
}

func (c *Client) calculateSummary(resources []models.Resource, totalProjects int) models.Summary {
	summary := models.Summary{
		TotalProjects: totalProjects,
	}

	for _, resource := range resources {
		switch resource.Type {
		case "server":
			summary.TotalServers++
		case "volume":
			summary.TotalVolumes++
		case "load_balancer":
			summary.TotalLoadBalancers++
		case "floating_ip":
			summary.TotalFloatingIPs++
		case "vpn_service":
			summary.TotalVPNServices++
		case "cluster":
			summary.TotalClusters++
		case "router":
			summary.TotalRouters++
		case "network":
			summary.TotalNetworks++
		}
	}

	return summary
}

// Helper functions
func extractNetworks(addresses interface{}) map[string]string {
	networks := make(map[string]string)
	if addressMap, ok := addresses.(map[string]interface{}); ok {
		for netName, addrs := range addressMap {
			if addrList, ok := addrs.([]interface{}); ok && len(addrList) > 0 {
				if addrInfo, ok := addrList[0].(map[string]interface{}); ok {
					if addr, exists := addrInfo["addr"]; exists {
						networks[netName] = addr.(string)
					}
				}
			}
		}
	}
	return networks
}

func convertGatewayInfo(gatewayInfo routers.GatewayInfo) map[string]interface{} {
	result := make(map[string]interface{})
	result["network_id"] = gatewayInfo.NetworkID
	result["enable_snat"] = gatewayInfo.EnableSNAT
	if len(gatewayInfo.ExternalFixedIPs) > 0 {
		result["external_fixed_ips"] = gatewayInfo.ExternalFixedIPs
	}
	return result
}

func convertRoutes(routes []routers.Route) []interface{} {
	result := make([]interface{}, len(routes))
	for i, route := range routes {
		routeMap := map[string]interface{}{
			"destination": route.DestinationCIDR,
			"nexthop":     route.NextHop,
		}
		result[i] = routeMap
	}
	return result
}

// getFlavorDetails gets detailed flavor information
func (c *Client) getFlavorDetails(flavorRef interface{}) (string, string) {
	if flavorRef == nil {
		return "Unknown", ""
	}

	// Try to get flavor ID first
	var flavorID string
	if flavorMap, ok := flavorRef.(map[string]interface{}); ok {
		if id, exists := flavorMap["id"]; exists {
			flavorID = id.(string)
		}
	}

	if flavorID == "" {
		return "Unknown", ""
	}

	// Get flavor details from API
	flavor, err := flavors.Get(c.computeClient, flavorID).Extract()
	if err != nil {
		return "Unknown", flavorID
	}

	return flavor.Name, flavorID
}



// getVolumeAttachments gets detailed attachment information including server names
func (c *Client) getVolumeAttachments(attachments interface{}) []models.VolumeAttachment {
	var result []models.VolumeAttachment

	if attachments == nil {
		return result
	}

	// Handle both []interface{} and []volumes.Attachment types
	if attachSlice, ok := attachments.([]volumes.Attachment); ok {
		// Direct volumes.Attachment slice
		for _, attach := range attachSlice {
			attachment := models.VolumeAttachment{
				ServerID: attach.ServerID,
				Device:   attach.Device,
			}

			// Get server name
			if attach.ServerID != "" {
				attachment.ServerName = c.getServerName(attach.ServerID)
			}

			result = append(result, attachment)
		}
	} else if attachList, ok := attachments.([]interface{}); ok {
		// Interface slice (fallback)
		for _, attach := range attachList {
			if attachInfo, ok := attach.(map[string]interface{}); ok {
				attachment := models.VolumeAttachment{}

				if serverID, exists := attachInfo["server_id"]; exists {
					attachment.ServerID = serverID.(string)
					attachment.ServerName = c.getServerName(attachment.ServerID)
				}

				if device, exists := attachInfo["device"]; exists {
					attachment.Device = device.(string)
				}

				result = append(result, attachment)
			}
		}
	}

	return result
}

// getServerName gets server name by ID
func (c *Client) getServerName(serverID string) string {
	if serverID == "" {
		return ""
	}

	server, err := servers.Get(c.computeClient, serverID).Extract()
	if err != nil {
		return serverID // Return ID if can't get name
	}

	return server.Name
}

// getAttachedResourceName gets the name of resource attached to a port
func (c *Client) getAttachedResourceName(portID string) string {
	if portID == "" {
		return ""
	}

	port, err := ports.Get(c.networkClient, portID).Extract()
	if err != nil {
		return ""
	}

	// If port has device_id, try to get the device name
	if port.DeviceID != "" {
		// Try to get server name first
		if port.DeviceOwner == "compute:nova" {
			serverName := c.getServerName(port.DeviceID)
			if serverName != "" {
				return serverName
			}
		}
		// For other device types, return device ID
		return port.DeviceID
	}

	return ""
}

// getVPNPeerID gets VPN peer ID from IPSec connections
func (c *Client) getVPNPeerID(vpnServiceID string) string {
	// This is a simplified approach - in real implementation you might want to
	// look at IPSec connections or other VPN-related resources
	// For now, return empty as this requires more complex VPN API calls
	return ""
}

// getProjectsViaAPI gets project list using domain-scoped API token
func (c *Client) getProjectsViaAPI() ([]models.Project, error) {
	// Create a domain-scoped client for project listing
	domainClient, err := c.createDomainScopedClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create domain-scoped client: %w", err)
	}

	// List all projects in the domain
	fmt.Printf("DEBUG: Attempting to list projects with domain-scoped token...\n")
	allPages, err := projects.List(domainClient.identityClient, projects.ListOpts{}).AllPages()
	if err != nil {
		fmt.Printf("DEBUG: Failed to list projects via API: %v\n", err)
		return nil, fmt.Errorf("failed to list projects via API: %w", err)
	}

	projectList, err := projects.ExtractProjects(allPages)
	if err != nil {
		return nil, fmt.Errorf("failed to extract projects: %w", err)
	}

	var result []models.Project
	for _, project := range projectList {
		result = append(result, models.Project{
			ID:          project.ID,
			Name:        project.Name,
			Description: project.Description,
			DomainID:    project.DomainID,
			Enabled:     project.Enabled,
		})
	}

	fmt.Printf("DEBUG: Found %d projects via API: ", len(result))
	for i, project := range result {
		if i > 0 {
			fmt.Printf(", ")
		}
		fmt.Printf("%s", project.Name)
	}
	fmt.Printf("\n")

	return result, nil
}

// createDomainScopedClient creates a domain-scoped OpenStack client for project listing
func (c *Client) createDomainScopedClient() (*Client, error) {
	opts := gophercloud.AuthOptions{
		IdentityEndpoint: os.Getenv("OS_AUTH_URL"),
		Username:         os.Getenv("OS_USERNAME"),
		Password:         os.Getenv("OS_PASSWORD"),
		DomainName:       os.Getenv("OS_USER_DOMAIN_NAME"),
		// No TenantName = domain-scoped token
		Scope: &gophercloud.AuthScope{
			DomainName: os.Getenv("OS_USER_DOMAIN_NAME"),
		},
	}

	var provider *gophercloud.ProviderClient
	var err error

	if os.Getenv("OS_INSECURE") == "true" {
		config := &tls.Config{InsecureSkipVerify: true}
		transport := &http.Transport{TLSClientConfig: config}
		client := &http.Client{Transport: transport}
		provider, err = openstack.NewClient(opts.IdentityEndpoint)
		if err != nil {
			return nil, fmt.Errorf("failed to create provider client: %w", err)
		}
		provider.HTTPClient = *client
		err = openstack.Authenticate(provider, opts)
	} else {
		provider, err = openstack.AuthenticatedClient(opts)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create domain-scoped authenticated client: %w", err)
	}

	identityClient, err := openstack.NewIdentityV3(provider, gophercloud.EndpointOpts{
		Region: os.Getenv("OS_REGION_NAME"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create identity client: %w", err)
	}

	return &Client{
		provider:       provider,
		identityClient: identityClient,
		// Only identity client needed for project listing
	}, nil
}

// getProjectsViaCommand gets project list using OpenStack CLI (fallback method)
func getProjectsViaCommand() ([]models.Project, error) {
	// Use openstack CLI to get project list (works even without identity:list_projects API permission)
	cmd := exec.Command("openstack", "project", "list", "-f", "json")

	// Set environment variables for the command
	cmd.Env = os.Environ()

	// Debug: print environment variables being passed to CLI
	fmt.Printf("DEBUG: CLI environment variables:\n")
	for _, env := range cmd.Env {
		if strings.HasPrefix(env, "OS_") {
			fmt.Printf("  %s\n", env)
		}
	}

	// Capture both stdout and stderr for better error diagnosis
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("DEBUG: CLI command failed with output: %s\n", string(output))
		return nil, fmt.Errorf("failed to execute 'openstack project list': %w (output: %s)", err, string(output))
	}

	// Parse JSON output
	var projects []struct {
		ID          string `json:"ID"`
		Name        string `json:"Name"`
		Description string `json:"Description"`
		Enabled     bool   `json:"Enabled"`
	}

	if err := json.Unmarshal(output, &projects); err != nil {
		return nil, fmt.Errorf("failed to parse project list JSON: %w", err)
	}

	var result []models.Project
	for _, project := range projects {
		result = append(result, models.Project{
			ID:          project.ID,
			Name:        project.Name,
			Description: project.Description,
			Enabled:     project.Enabled,
		})
	}

	return result, nil
}

// getResourcesForProject creates a new client for specific project and gets its resources
func getResourcesForProject(project models.Project) ([]models.Resource, error) {
	// Create a new client specifically for this project
	projectClient, err := createClientForProject(project.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to create client for project %s: %w", project.Name, err)
	}

	var resources []models.Resource
	projectNames := make(map[string]string)
	projectNames[project.ID] = project.Name

	// Get all resource types for this project with detailed logging
	fmt.Printf("   📋 Collecting servers...")
	serverResources, err := projectClient.getServersForSingleProject(projectNames)
	if err == nil {
		resources = append(resources, serverResources...)
		fmt.Printf(" %d found\n", len(serverResources))
	} else {
		fmt.Printf(" failed: %v\n", err)
	}

	fmt.Printf("   💾 Collecting volumes...")
	volumeResources, err := projectClient.getVolumesForSingleProject(projectNames)
	if err == nil {
		resources = append(resources, volumeResources...)
		fmt.Printf(" %d found\n", len(volumeResources))
	} else {
		fmt.Printf(" failed: %v\n", err)
	}

	fmt.Printf("   🌐 Collecting floating IPs...")
	floatingIPResources, err := projectClient.getFloatingIPs(projectNames)
	if err == nil {
		resources = append(resources, floatingIPResources...)
		fmt.Printf(" %d found\n", len(floatingIPResources))
	} else {
		fmt.Printf(" failed: %v\n", err)
	}

	fmt.Printf("   🔀 Collecting routers...")
	routerResources, err := projectClient.getRouters(projectNames)
	if err == nil {
		resources = append(resources, routerResources...)
		fmt.Printf(" %d found\n", len(routerResources))
	} else {
		fmt.Printf(" failed: %v\n", err)
	}

	fmt.Printf("   🌐 Collecting networks...")
	networkResources, err := projectClient.getNetworks(projectNames)
	if err == nil {
		resources = append(resources, networkResources...)
		fmt.Printf(" %d found\n", len(networkResources))
	} else {
		fmt.Printf(" failed: %v\n", err)
	}

	if projectClient.loadbalancerClient != nil {
		fmt.Printf("   ⚖️  Collecting load balancers...")
		lbResources, err := projectClient.getLoadBalancers(projectNames)
		if err == nil {
			resources = append(resources, lbResources...)
			fmt.Printf(" %d found\n", len(lbResources))
		} else {
			fmt.Printf(" failed: %v\n", err)
		}
	}

	fmt.Printf("   🔒 Collecting VPN connections...")
	vpnResources, err := projectClient.getVPNConnections(projectNames)
	if err == nil {
		resources = append(resources, vpnResources...)
		fmt.Printf(" %d found\n", len(vpnResources))
	} else {
		fmt.Printf(" failed: %v\n", err)
	}

	if projectClient.containerClient != nil {
		fmt.Printf("   ☸️  Collecting K8s clusters...")
		clusterResources, err := projectClient.getClusters(projectNames)
		if err == nil {
			resources = append(resources, clusterResources...)
			fmt.Printf(" %d found\n", len(clusterResources))
		} else {
			fmt.Printf(" failed: %v\n", err)
		}
	}

	return resources, nil
}

// getResourcesForProjectWithProgress creates a new client for specific project and gets its resources with progress
func getResourcesForProjectWithProgress(project models.Project, reporter ProgressReporter) ([]models.Resource, error) {
	// Create a new client specifically for this project
	projectClient, err := createClientForProject(project.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to create client for project %s: %w", project.Name, err)
	}

	var resources []models.Resource
	projectNames := make(map[string]string)
	projectNames[project.ID] = project.Name

	// Get all resource types for this project with detailed progress reporting
	reporter.SendProgress("resource_start", "Collecting servers", 0, 0, project.Name, "servers", 0, nil)
	serverResources, err := projectClient.getServersForSingleProject(projectNames)
	if err == nil {
		resources = append(resources, serverResources...)
		reporter.SendProgress("resource_complete", "Servers collected", 0, 0, project.Name, "servers", len(serverResources), nil)
	} else {
		reporter.SendProgress("resource_error", fmt.Sprintf("Failed to collect servers: %v", err), 0, 0, project.Name, "servers", 0, nil)
	}

	reporter.SendProgress("resource_start", "Collecting volumes", 0, 0, project.Name, "volumes", 0, nil)
	volumeResources, err := projectClient.getVolumesForSingleProject(projectNames)
	if err == nil {
		resources = append(resources, volumeResources...)
		reporter.SendProgress("resource_complete", "Volumes collected", 0, 0, project.Name, "volumes", len(volumeResources), nil)
	} else {
		reporter.SendProgress("resource_error", fmt.Sprintf("Failed to collect volumes: %v", err), 0, 0, project.Name, "volumes", 0, nil)
	}

	reporter.SendProgress("resource_start", "Collecting floating IPs", 0, 0, project.Name, "floating_ips", 0, nil)
	floatingIPResources, err := projectClient.getFloatingIPs(projectNames)
	if err == nil {
		resources = append(resources, floatingIPResources...)
		reporter.SendProgress("resource_complete", "Floating IPs collected", 0, 0, project.Name, "floating_ips", len(floatingIPResources), nil)
	} else {
		reporter.SendProgress("resource_error", fmt.Sprintf("Failed to collect floating IPs: %v", err), 0, 0, project.Name, "floating_ips", 0, nil)
	}

	reporter.SendProgress("resource_start", "Collecting routers", 0, 0, project.Name, "routers", 0, nil)
	routerResources, err := projectClient.getRouters(projectNames)
	if err == nil {
		resources = append(resources, routerResources...)
		reporter.SendProgress("resource_complete", "Routers collected", 0, 0, project.Name, "routers", len(routerResources), nil)
	} else {
		reporter.SendProgress("resource_error", fmt.Sprintf("Failed to collect routers: %v", err), 0, 0, project.Name, "routers", 0, nil)
	}

	reporter.SendProgress("resource_start", "Collecting networks", 0, 0, project.Name, "networks", 0, nil)
	networkResources, err := projectClient.getNetworks(projectNames)
	if err == nil {
		resources = append(resources, networkResources...)
		reporter.SendProgress("resource_complete", "Networks collected", 0, 0, project.Name, "networks", len(networkResources), nil)
	} else {
		reporter.SendProgress("resource_error", fmt.Sprintf("Failed to collect networks: %v", err), 0, 0, project.Name, "networks", 0, nil)
	}

	// Always send load balancer progress (even if client is nil)
	reporter.SendProgress("resource_start", "Collecting load balancers", 0, 0, project.Name, "load_balancers", 0, nil)
	if projectClient.loadbalancerClient != nil {
		lbResources, err := projectClient.getLoadBalancers(projectNames)
		if err == nil {
			resources = append(resources, lbResources...)
			reporter.SendProgress("resource_complete", "Load balancers collected", 0, 0, project.Name, "load_balancers", len(lbResources), nil)
		} else {
			reporter.SendProgress("resource_error", fmt.Sprintf("Failed to collect load balancers: %v", err), 0, 0, project.Name, "load_balancers", 0, nil)
		}
	} else {
		reporter.SendProgress("resource_complete", "Load balancers collected", 0, 0, project.Name, "load_balancers", 0, nil)
	}

	reporter.SendProgress("resource_start", "Collecting VPN connections", 0, 0, project.Name, "vpn_connections", 0, nil)
	vpnResources, err := projectClient.getVPNConnections(projectNames)
	if err == nil {
		resources = append(resources, vpnResources...)
		reporter.SendProgress("resource_complete", "VPN connections collected", 0, 0, project.Name, "vpn_connections", len(vpnResources), nil)
	} else {
		reporter.SendProgress("resource_error", fmt.Sprintf("Failed to collect VPN connections: %v", err), 0, 0, project.Name, "vpn_connections", 0, nil)
	}

	// Always send K8s clusters progress (even if client is nil)
	reporter.SendProgress("resource_start", "Collecting K8s clusters", 0, 0, project.Name, "k8s_clusters", 0, nil)
	if projectClient.containerClient != nil {
		clusterResources, err := projectClient.getClusters(projectNames)
		if err == nil {
			resources = append(resources, clusterResources...)
			reporter.SendProgress("resource_complete", "K8s clusters collected", 0, 0, project.Name, "k8s_clusters", len(clusterResources), nil)
		} else {
			reporter.SendProgress("resource_error", fmt.Sprintf("Failed to collect K8s clusters: %v", err), 0, 0, project.Name, "k8s_clusters", 0, nil)
		}
	} else {
		reporter.SendProgress("resource_complete", "K8s clusters collected", 0, 0, project.Name, "k8s_clusters", 0, nil)
	}

	return resources, nil
}

// collectResourcesForProjectsWithProgress collects resources using current client with progress (single project mode)
func (c *Client) collectResourcesForProjectsWithProgress(report *models.ResourceReport, projectNames map[string]string, reporter ProgressReporter) (*models.ResourceReport, error) {
	// Get all resource types with progress updates
	reporter.SendProgress("resource_start", "Collecting servers", 0, 0, "", "servers", 0, nil)
	serverResources, err := c.getServers(projectNames)
	if err != nil {
		return nil, fmt.Errorf("failed to get servers: %w", err)
	}
	report.Resources = append(report.Resources, serverResources...)
	reporter.SendProgress("resource_complete", "Servers collected", 0, 0, "", "servers", len(serverResources), nil)

	reporter.SendProgress("resource_start", "Collecting volumes", 0, 0, "", "volumes", 0, nil)
	volumeResources, err := c.getVolumes(projectNames)
	if err != nil {
		return nil, fmt.Errorf("failed to get volumes: %w", err)
	}
	report.Resources = append(report.Resources, volumeResources...)
	reporter.SendProgress("resource_complete", "Volumes collected", 0, 0, "", "volumes", len(volumeResources), nil)

	reporter.SendProgress("resource_start", "Collecting floating IPs", 0, 0, "", "floating_ips", 0, nil)
	floatingIPResources, err := c.getFloatingIPs(projectNames)
	if err != nil {
		return nil, fmt.Errorf("failed to get floating IPs: %w", err)
	}
	report.Resources = append(report.Resources, floatingIPResources...)
	reporter.SendProgress("resource_complete", "Floating IPs collected", 0, 0, "", "floating_ips", len(floatingIPResources), nil)

	reporter.SendProgress("resource_start", "Collecting routers", 0, 0, "", "routers", 0, nil)
	routerResources, err := c.getRouters(projectNames)
	if err != nil {
		return nil, fmt.Errorf("failed to get routers: %w", err)
	}
	report.Resources = append(report.Resources, routerResources...)
	reporter.SendProgress("resource_complete", "Routers collected", 0, 0, "", "routers", len(routerResources), nil)

	reporter.SendProgress("resource_start", "Collecting networks", 0, 0, "", "networks", 0, nil)
	networkResources, err := c.getNetworks(projectNames)
	if err != nil {
		return nil, fmt.Errorf("failed to get networks: %w", err)
	}
	report.Resources = append(report.Resources, networkResources...)
	reporter.SendProgress("resource_complete", "Networks collected", 0, 0, "", "networks", len(networkResources), nil)

	// Optional services
	if c.loadbalancerClient != nil {
		reporter.SendProgress("resource_start", "Collecting load balancers", 0, 0, "", "load_balancers", 0, nil)
		lbResources, err := c.getLoadBalancers(projectNames)
		if err == nil {
			report.Resources = append(report.Resources, lbResources...)
			reporter.SendProgress("resource_complete", "Load balancers collected", 0, 0, "", "load_balancers", len(lbResources), nil)
		}
	}

	reporter.SendProgress("resource_start", "Collecting VPN connections", 0, 0, "", "vpn_connections", 0, nil)
	vpnResources, err := c.getVPNConnections(projectNames)
	if err == nil {
		report.Resources = append(report.Resources, vpnResources...)
		reporter.SendProgress("resource_complete", "VPN connections collected", 0, 0, "", "vpn_connections", len(vpnResources), nil)
	}

	if c.containerClient != nil {
		reporter.SendProgress("resource_start", "Collecting K8s clusters", 0, 0, "", "k8s_clusters", 0, nil)
		clusterResources, err := c.getClusters(projectNames)
		if err == nil {
			report.Resources = append(report.Resources, clusterResources...)
			reporter.SendProgress("resource_complete", "K8s clusters collected", 0, 0, "", "k8s_clusters", len(clusterResources), nil)
		}
	}

	// Calculate summary
	report.Summary = c.calculateSummary(report.Resources, len(report.Projects))

	return report, nil
}

// createClientForProject creates a new OpenStack client for specific project
func createClientForProject(projectName string) (*Client, error) {
	opts := gophercloud.AuthOptions{
		IdentityEndpoint: os.Getenv("OS_AUTH_URL"),
		Username:         os.Getenv("OS_USERNAME"),
		Password:         os.Getenv("OS_PASSWORD"),
		DomainName:       os.Getenv("OS_USER_DOMAIN_NAME"),
		TenantName:       projectName, // Use specific project name
	}

	var provider *gophercloud.ProviderClient
	var err error

	if os.Getenv("OS_INSECURE") == "true" {
		config := &tls.Config{InsecureSkipVerify: true}
		transport := &http.Transport{TLSClientConfig: config}
		client := &http.Client{Transport: transport}
		provider, err = openstack.NewClient(opts.IdentityEndpoint)
		if err != nil {
			return nil, fmt.Errorf("failed to create provider client: %w", err)
		}
		provider.HTTPClient = *client
		err = openstack.Authenticate(provider, opts)
	} else {
		provider, err = openstack.AuthenticatedClient(opts)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create authenticated client for project %s: %w", projectName, err)
	}

	computeClient, err := openstack.NewComputeV2(provider, gophercloud.EndpointOpts{
		Region: os.Getenv("OS_REGION_NAME"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create compute client: %w", err)
	}

	blockstorageClient, err := openstack.NewBlockStorageV3(provider, gophercloud.EndpointOpts{
		Region: os.Getenv("OS_REGION_NAME"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create block storage client: %w", err)
	}

	networkClient, err := openstack.NewNetworkV2(provider, gophercloud.EndpointOpts{
		Region: os.Getenv("OS_REGION_NAME"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create network client: %w", err)
	}

	identityClient, err := openstack.NewIdentityV3(provider, gophercloud.EndpointOpts{
		Region: os.Getenv("OS_REGION_NAME"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create identity client: %w", err)
	}

	loadbalancerClient, err := openstack.NewLoadBalancerV2(provider, gophercloud.EndpointOpts{
		Region: os.Getenv("OS_REGION_NAME"),
	})
	if err != nil {
		loadbalancerClient = nil
	}

	containerClient, err := openstack.NewContainerInfraV1(provider, gophercloud.EndpointOpts{
		Region: os.Getenv("OS_REGION_NAME"),
	})
	if err != nil {
		containerClient = nil
	}

	return &Client{
		provider:           provider,
		computeClient:      computeClient,
		blockstorageClient: blockstorageClient,
		networkClient:      networkClient,
		identityClient:     identityClient,
		loadbalancerClient: loadbalancerClient,
		containerClient:    containerClient,
	}, nil
}

// collectResourcesForProjects collects resources using current client (single project mode)
func (c *Client) collectResourcesForProjects(report *models.ResourceReport, projectNames map[string]string) (*models.ResourceReport, error) {
	// Get all resource types
	serverResources, err := c.getServers(projectNames)
	if err != nil {
		return nil, fmt.Errorf("failed to get servers: %w", err)
	}
	report.Resources = append(report.Resources, serverResources...)

	volumeResources, err := c.getVolumes(projectNames)
	if err != nil {
		return nil, fmt.Errorf("failed to get volumes: %w", err)
	}
	report.Resources = append(report.Resources, volumeResources...)

	floatingIPResources, err := c.getFloatingIPs(projectNames)
	if err != nil {
		return nil, fmt.Errorf("failed to get floating IPs: %w", err)
	}
	report.Resources = append(report.Resources, floatingIPResources...)

	routerResources, err := c.getRouters(projectNames)
	if err != nil {
		return nil, fmt.Errorf("failed to get routers: %w", err)
	}
	report.Resources = append(report.Resources, routerResources...)

	networkResources, err := c.getNetworks(projectNames)
	if err != nil {
		return nil, fmt.Errorf("failed to get networks: %w", err)
	}
	report.Resources = append(report.Resources, networkResources...)

	// Optional services
	if c.loadbalancerClient != nil {
		lbResources, err := c.getLoadBalancers(projectNames)
		if err == nil {
			report.Resources = append(report.Resources, lbResources...)
		}
	}

	// Get VPN IPSec Site Connections (actual VPN tunnels with peer info)
	vpnResources, err := c.getVPNConnections(projectNames)
	if err == nil {
		report.Resources = append(report.Resources, vpnResources...)
	}

	if c.containerClient != nil {
		clusterResources, err := c.getClusters(projectNames)
		if err == nil {
			report.Resources = append(report.Resources, clusterResources...)
		}
	}

	// Calculate summary
	report.Summary = c.calculateSummary(report.Resources, len(report.Projects))

	return report, nil
}

// getServersForSingleProject gets servers without AllTenants (for per-project clients)
func (c *Client) getServersForSingleProject(projectNames map[string]string) ([]models.Resource, error) {
	// Always use project-scoped request (no AllTenants)
	listOpts := servers.ListOpts{}
	allPages, err := servers.List(c.computeClient, listOpts).AllPages()
	if err != nil {
		return nil, err
	}

	serverList, err := servers.ExtractServers(allPages)
	if err != nil {
		return nil, err
	}

	// Get the project name from the first entry in projectNames map
	var fallbackProjectName, fallbackProjectID string
	for id, name := range projectNames {
		fallbackProjectName = name
		fallbackProjectID = id
		break
	}

	var resources []models.Resource
	for _, server := range serverList {
		created := server.Created
		updated := server.Updated

		projectName := projectNames[server.TenantID]
		projectID := server.TenantID
		if projectName == "" {
			// Use the project name from the scoped client, not "Current Project"
			projectName = fallbackProjectName
			projectID = fallbackProjectID
		}

		flavorName, flavorID := c.getFlavorDetails(server.Flavor)

		resources = append(resources, models.Resource{
			ID:          server.ID,
			Name:        server.Name,
			Type:        "server",
			ProjectID:   projectID,
			ProjectName: projectName,
			Status:      server.Status,
			CreatedAt:   created,
			UpdatedAt:   updated,
			Properties: models.Server{
				ID:         server.ID,
				Name:       server.Name,
				Status:     server.Status,
				FlavorName: flavorName,
				FlavorID:   flavorID,
				Networks:   extractNetworks(server.Addresses),
				CreatedAt:  created,
				UpdatedAt:  updated,
			},
		})
	}

	return resources, nil
}

// getVolumesForSingleProject gets volumes without AllTenants (for per-project clients)
func (c *Client) getVolumesForSingleProject(projectNames map[string]string) ([]models.Resource, error) {
	// Always use project-scoped request (no AllTenants)
	listOpts := volumes.ListOpts{}
	allPages, err := volumes.List(c.blockstorageClient, listOpts).AllPages()
	if err != nil {
		return nil, err
	}

	volumeList, err := volumes.ExtractVolumes(allPages)
	if err != nil {
		return nil, err
	}

	// Get the project name from the first entry in projectNames map
	var projectID, projectName string
	for id, name := range projectNames {
		projectID = id
		projectName = name
		break
	}

	var resources []models.Resource
	for _, volume := range volumeList {
		created := volume.CreatedAt

		attachments := c.getVolumeAttachments(volume.Attachments)
		attachedTo := ""
		if len(attachments) > 0 {
			attachedTo = attachments[0].ServerName
		}

		resources = append(resources, models.Resource{
			ID:          volume.ID,
			Name:        volume.Name,
			Type:        "volume",
			ProjectID:   projectID,
			ProjectName: projectName,
			Status:      volume.Status,
			CreatedAt:   created,
			Properties: models.Volume{
				ID:          volume.ID,
				Name:        volume.Name,
				Status:      volume.Status,
				Size:        volume.Size,
				VolumeType:  volume.VolumeType,
				Bootable:    volume.Bootable == "true",
				Attachments: attachments,
				AttachedTo:  attachedTo,
				CreatedAt:   created,
			},
		})
	}

	return resources, nil
}
