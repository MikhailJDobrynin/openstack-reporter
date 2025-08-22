package models

import "time"

// Resource represents a generic OpenStack resource
type Resource struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Type         string            `json:"type"`
	ProjectID    string            `json:"project_id"`
	ProjectName  string            `json:"project_name"`
	Status       string            `json:"status"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	Properties   interface{}       `json:"properties,omitempty"`
}

// Project represents OpenStack project
type Project struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	DomainID    string `json:"domain_id"`
	Enabled     bool   `json:"enabled"`
}

// Server represents OpenStack compute instance
type Server struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Status       string            `json:"status"`
	FlavorName   string            `json:"flavor_name"`
	FlavorID     string            `json:"flavor_id"`
	Networks     map[string]string `json:"networks"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

// Volume represents OpenStack block storage volume
type Volume struct {
	ID             string               `json:"id"`
	Name           string               `json:"name"`
	Status         string               `json:"status"`
	Size           int                  `json:"size"`
	VolumeType     string               `json:"volume_type"`
	Bootable       bool                 `json:"bootable"`
	Attachments    []VolumeAttachment   `json:"attachments"`
	AttachedTo     string               `json:"attached_to,omitempty"`
	CreatedAt      time.Time            `json:"created_at"`
}

// VolumeAttachment represents volume attachment details
type VolumeAttachment struct {
	ServerID     string `json:"server_id"`
	ServerName   string `json:"server_name,omitempty"`
	Device       string `json:"device,omitempty"`
}

// LoadBalancer represents OpenStack load balancer
type LoadBalancer struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	Description       string    `json:"description"`
	ProvisioningStatus string   `json:"provisioning_status"`
	OperatingStatus   string    `json:"operating_status"`
	VipAddress        string    `json:"vip_address"`
	VipSubnetID       string    `json:"vip_subnet_id"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// FloatingIP represents OpenStack floating IP
type FloatingIP struct {
	ID                   string    `json:"id"`
	FloatingIP           string    `json:"floating_ip"`
	Status               string    `json:"status"`
	FixedIP              string    `json:"fixed_ip,omitempty"`
	PortID               string    `json:"port_id,omitempty"`
	AttachedResourceName string    `json:"attached_resource_name,omitempty"`
	FloatingNetworkID    string    `json:"floating_network_id"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// VPNService represents OpenStack VPN service
type VPNService struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	Status          string    `json:"status"`
	RouterID        string    `json:"router_id"`
	SubnetID        string    `json:"subnet_id"`
	PeerID          string    `json:"peer_id,omitempty"`
	PeerAddress     string    `json:"peer_address,omitempty"`
	AuthMode        string    `json:"auth_mode,omitempty"`
	IKEVersion      string    `json:"ike_version,omitempty"`
	MTU             int       `json:"mtu,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// Cluster represents Kubernetes cluster
type Cluster struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	Status            string    `json:"status"`
	ClusterTemplateID string    `json:"cluster_template_id"`
	NodeCount         int       `json:"node_count"`
	MasterCount       int       `json:"master_count"`
	KeyPair           string    `json:"keypair"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// Router represents OpenStack network router
type Router struct {
	ID                  string                 `json:"id"`
	Name                string                 `json:"name"`
	Status              string                 `json:"status"`
	AdminStateUp        bool                   `json:"admin_state_up"`
	ExternalGatewayInfo map[string]interface{} `json:"external_gateway_info,omitempty"`
	Routes              []interface{}          `json:"routes,omitempty"`
	CreatedAt           time.Time              `json:"created_at"`
	UpdatedAt           time.Time              `json:"updated_at"`
}

// Network represents OpenStack network
type Network struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Status       string    `json:"status"`
	AdminStateUp bool      `json:"admin_state_up"`
	Shared       bool      `json:"shared"`
	External     bool      `json:"external"`
	NetworkType  string    `json:"network_type"`
	Subnets      []string  `json:"subnets"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ResourceReport represents the complete report structure
type ResourceReport struct {
	GeneratedAt time.Time  `json:"generated_at"`
	Projects    []Project  `json:"projects"`
	Resources   []Resource `json:"resources"`
	Summary     Summary    `json:"summary"`
}

// Summary provides counts by resource type
type Summary struct {
	TotalProjects      int `json:"total_projects"`
	TotalServers       int `json:"total_servers"`
	TotalVolumes       int `json:"total_volumes"`
	TotalLoadBalancers int `json:"total_load_balancers"`
	TotalFloatingIPs   int `json:"total_floating_ips"`
	TotalVPNServices   int `json:"total_vpn_services"`
	TotalClusters      int `json:"total_clusters"`
	TotalRouters       int `json:"total_routers"`
	TotalNetworks      int `json:"total_networks"`
}
