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
	"github.com/gophercloud/gophercloud/openstack/identity/v3/projects"
	"github.com/gophercloud/gophercloud/openstack/loadbalancer/v2/loadbalancers"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/layer3/floatingips"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/layer3/routers"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/vpnaas/services"

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

	// Get current project info
	currentProject, err := c.getCurrentProject()
	if err != nil {
		return nil, fmt.Errorf("failed to get current project: %w", err)
	}
	report.Projects = []models.Project{currentProject}

	// Create project name mapping
	projectNames := map[string]string{
		currentProject.ID: currentProject.Name,
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

	vpnResources, err := c.getVPNServices(projectNames)
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
	allPages, err := servers.List(c.computeClient, servers.ListOpts{}).AllPages()
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
				FlavorName: getFlavorName(server.Flavor),
				ImageName:  getImageName(server.Image),
				Networks:   extractNetworks(server.Addresses),
				CreatedAt:  created,
				UpdatedAt:  updated,
			},
		})
	}

	return resources, nil
}

func (c *Client) getVolumes(projectNames map[string]string) ([]models.Resource, error) {
	// Get current project info for fallback
	currentProject, _ := c.getCurrentProject()
	allPages, err := volumes.List(c.blockstorageClient, volumes.ListOpts{}).AllPages()
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

		// Try to get project ID from volume metadata or use current project
		projectID := currentProject.ID
		projectName := currentProject.Name

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
				Attachments: extractAttachments(volume.Attachments),
				CreatedAt:   created,
			},
		})
	}

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
				ID:                fip.ID,
				FloatingIP:        fip.FloatingIP,
				Status:            fip.Status,
				FixedIP:           fip.FixedIP,
				PortID:            fip.PortID,
				FloatingNetworkID: fip.FloatingNetworkID,
				CreatedAt:         created,
				UpdatedAt:         updated,
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
				CreatedAt:   created,
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
func getImageName(image interface{}) string {
	if image == nil {
		return "Unknown"
	}
	if imageMap, ok := image.(map[string]interface{}); ok {
		if name, exists := imageMap["name"]; exists {
			if nameStr, ok := name.(string); ok {
				return nameStr
			}
		}
	}
	return "Unknown"
}

func getFlavorName(flavor interface{}) string {
	if flavor == nil {
		return "Unknown"
	}
	if flavorMap, ok := flavor.(map[string]interface{}); ok {
		if name, exists := flavorMap["original_name"]; exists {
			if nameStr, ok := name.(string); ok {
				return nameStr
			}
		}
		if name, exists := flavorMap["name"]; exists {
			if nameStr, ok := name.(string); ok {
				return nameStr
			}
		}
	}
	return "Unknown"
}

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

func extractAttachments(attachments interface{}) []string {
	var result []string
	if attachList, ok := attachments.([]interface{}); ok {
		for _, attach := range attachList {
			if attachInfo, ok := attach.(map[string]interface{}); ok {
				if serverID, exists := attachInfo["server_id"]; exists {
					result = append(result, serverID.(string))
				}
			}
		}
	}
	return result
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
