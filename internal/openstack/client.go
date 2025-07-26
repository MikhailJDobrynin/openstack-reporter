package openstack

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
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
	"github.com/gophercloud/gophercloud/openstack/networking/v2/ports"

	"openstack-reporter/internal/models"
)

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
	opts := gophercloud.AuthOptions{
		IdentityEndpoint: os.Getenv("OS_AUTH_URL"),
		Username:         os.Getenv("OS_USERNAME"),
		Password:         os.Getenv("OS_PASSWORD"),
		DomainName:       os.Getenv("OS_USER_DOMAIN_NAME"),
		TenantName:       os.Getenv("OS_PROJECT_NAME"),
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

	// Try to get all accessible projects first
	allProjects, err := c.getAllProjects()
	if err != nil {
		// Fallback to current project only if we can't get all projects
		currentProject, fallbackErr := c.getCurrentProject()
		if fallbackErr != nil {
			return nil, fmt.Errorf("failed to get projects: %w", err)
		}
		report.Projects = []models.Project{currentProject}
	} else {
		report.Projects = allProjects
	}

	// Create project name mapping
	projectNames := make(map[string]string)
	for _, project := range report.Projects {
		projectNames[project.ID] = project.Name
	}

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
	report.Summary = c.calculateSummary(report.Resources, 1)

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

	// Try to get servers from all tenants first, fallback to current tenant only
	listOpts := servers.ListOpts{AllTenants: true}
	allPages, err := servers.List(c.computeClient, listOpts).AllPages()
	if err != nil {
		// Fallback to current tenant only if AllTenants fails
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
	var allResources []models.Resource
	currentProject, _ := c.getCurrentProject()

	// If we have multiple projects, try to get volumes for each project separately
	if len(projectNames) > 1 {
		for projectID, projectName := range projectNames {
			projectVolumes, err := c.getVolumesForProject(projectID, projectName)
			if err != nil {
				fmt.Printf("DEBUG: Failed to get volumes for project %s (%s): %v\n", projectName, projectID, err)
				continue // Skip this project and continue with others
			}
			allResources = append(allResources, projectVolumes...)
		}

		if len(allResources) > 0 {
			return allResources, nil
		}
	}

	// Fallback: get volumes without project filtering (current approach)
	listOpts := volumes.ListOpts{}
	allPages, err := volumes.List(c.blockstorageClient, listOpts).AllPages()
	if err != nil {
		return nil, err
	}

	volumeList, err := volumes.ExtractVolumes(allPages)
	if err != nil {
		return nil, err
	}

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

		allResources = append(allResources, models.Resource{
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

	return allResources, nil
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
