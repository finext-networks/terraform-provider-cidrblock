# ==============================================================================
# Terraform Core & Provider Definitions
# ==============================================================================

terraform {
  required_version = ">= 1.5.0"
  required_providers {
    cidrblock = {
      source = "finext-networks/cidrblock"
    }
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 3.0" # Employs standard v3.x/v4.x Azure RM toolchains
    }
  }
}

# Provider orchestration implementing advanced cloud state protections
provider "cidrblock" {
  # Safety Switch: Actively blocks any 'terraform apply' execution that attempts
  # to drop an active subnet key from the allocations map, protecting live 
  # downstream Azure Virtual Network interfaces and private endpoints.
  prevent_subnet_destruction = true
}

provider "azurerm" {
  features {}
}

# ==============================================================================
# Structural Input Variables
# ==============================================================================

variable "azure_region" {
  type    = string
  default = "eastus2"
}

variable "environment" {
  type    = string
  default = "production"
}

# ==============================================================================
# Step 1: The Core IPAM Allocation Pool
# ==============================================================================

resource "cidrblock_pool" "vnet_matrix" {
  cidr                = "10.200.0.0/16"
  organization        = "finext"
  project             = "azure-landing-zone"
  network             = "vnet-primary"
  allocation_strategy = "BEST" # Packs networks efficiently using best-fit binary tracking

  # ADVANCED TRACKING: The underlying engine automatically sorts keys by network size
  # descending (largest blocks first) to completely wipe out dead-space allocation gaps.
  allocations = {
    # PASS 1: Sorted First (Largest block: /21 = 2048 IPs).
    # Evaluated first due to FFD sorting, landing on the base: 10.200.0.0/21.
    # Spawns its mathematical Availability Zone 2 shadow node-pool companion 
    # cleanly at 10.200.8.0/21. Total space locked: 10.200.0.0 - 10.200.15.255.
    aks_pods_tier = {
      prefix_size     = 21
      reserve_sibling = true
    }

    # PASS 2: Sorted Second (Medium block: /24 = 256 IPs). Tied with public_app_gw.
    # Wins alphabetical tie-breaker. Aligns on next free block boundary: 10.200.16.0/24.
    # Reserves its companion regional high-availability tier at 10.200.17.0/24.
    app_service_integration = {
      prefix_size     = 24
      reserve_sibling = true
    }

    # PASS 3: Sorted Third (Medium block: /24 = 256 IPs).
    # Evaluated next. Safely skips past active app_service_integration boundaries 
    # to claim 10.200.18.0/24. No sibling reservation required.
    public_app_gw = {
      prefix_size     = 24
      reserve_sibling = false
    }

    # PASS 4: Sorted Fourth (Smallest block: /26 = 64 IPs).
    # Matches strict Azure Bastion minimum requirement constraints. 
    # Claims the clean, isolated boundary starting at 10.200.19.0/26.
    bastion_tier = {
      prefix_size     = 26
      reserve_sibling = false
    }
  }
}

# ==============================================================================
# Step 2: Real-Time Telemetry via Hydrated Data Source
# ==============================================================================

# By passing the resource's computed 4-part composite ID token, this data source 
# executes in-process immediately after the resource change, capturing calculated engine logs.
data "cidrblock_pool" "telemetry" {
  id = cidrblock_pool.vnet_matrix.id
}

# ==============================================================================
# Step 3: Materialize Network Topologies inside Azure
# ==============================================================================

resource "azurerm_resource_group" "network_holder" {
  name     = "rg-${var.environment}-networking"
  location = var.azure_region
}

resource "azurerm_virtual_network" "hub_vnet" {
  name                = "vnet-${var.environment}-core"
  resource_group_name = azurerm_resource_group.network_holder.name
  location            = azurerm_resource_group.network_holder.location
  address_space       = [cidrblock_pool.vnet_matrix.cidr]

  tags = {
    Environment = var.environment
    ManagedBy   = "IPAM Engine"
  }
}

# --- Azure Kubernetes Service (AKS) Pod Subnets (Multi-Zone) ---

resource "azurerm_subnet" "aks_zone_1" {
  name                 = "snet-aks-nodepool-z1"
  resource_group_name  = azurerm_resource_group.network_holder.name
  virtual_network_name = azurerm_virtual_network.hub_vnet.name
  address_prefixes     = [cidrblock_pool.vnet_matrix.allocations["aks_pods_tier"].allocated_cidr]
}

resource "azurerm_subnet" "aks_zone_2" {
  name                 = "snet-aks-nodepool-z2"
  resource_group_name  = azurerm_resource_group.network_holder.name
  virtual_network_name = azurerm_virtual_network.hub_vnet.name
  address_prefixes     = [cidrblock_pool.vnet_matrix.allocations["aks_pods_tier"].sibling_cidr]
}

# --- Azure App Service VNet Integration Subnets (High Availability) ---

resource "azurerm_subnet" "app_service_primary" {
  name                 = "snet-appservice-regional-01"
  resource_group_name  = azurerm_resource_group.network_holder.name
  virtual_network_name = azurerm_virtual_network.hub_vnet.name
  address_prefixes     = [cidrblock_pool.vnet_matrix.allocations["app_service_integration"].allocated_cidr]

  delegation {
    name = "appservice-delegation"
    service_delegation {
      name    = "Microsoft.Web/serverFarms"
      actions = ["Microsoft.Network/virtualNetworks/subnets/action"]
    }
  }
}

resource "azurerm_subnet" "app_service_secondary" {
  name                 = "snet-appservice-regional-02"
  resource_group_name  = azurerm_resource_group.network_holder.name
  virtual_network_name = azurerm_virtual_network.hub_vnet.name
  address_prefixes     = [cidrblock_pool.vnet_matrix.allocations["app_service_integration"].sibling_cidr]

  delegation {
    name = "appservice-delegation"
    service_delegation {
      name    = "Microsoft.Web/serverFarms"
      actions = ["Microsoft.Network/virtualNetworks/subnets/action"]
    }
  }
}

# --- Azure Application Gateway Layer v2 ---

resource "azurerm_subnet" "app_gateway" {
  name                 = "snet-application-gateway"
  resource_group_name  = azurerm_resource_group.network_holder.name
  virtual_network_name = azurerm_virtual_network.hub_vnet.name
  address_prefixes     = [cidrblock_pool.vnet_matrix.allocations["public_app_gw"].allocated_cidr]
}

# --- Azure Bastion Dedicated Management Subnet ---

resource "azurerm_subnet" "bastion" {
  # CRITICAL: Azure requires this exact literal naming convention for Bastion deployments
  name                 = "AzureBastionSubnet" 
  resource_group_name  = azurerm_resource_group.network_holder.name
  virtual_network_name = azurerm_virtual_network.hub_vnet.name
  address_prefixes     = [cidrblock_pool.vnet_matrix.allocations["bastion_tier"].allocated_cidr]
}

# ==============================================================================
# Step 4: Verification & Capacity Planning Outputs
# ==============================================================================

output "azure_vnet_metadata" {
  description = "The verified baseline networking blocks deployed to Microsoft Azure."
  value = {
    vnet_id          = azurerm_virtual_network.hub_vnet.id
    address_space    = azurerm_virtual_network.hub_vnet.address_space[0]
    composite_key_id = data.cidrblock_pool.telemetry.id
  }
}

output "ipam_pool_metrics" {
  description = "Live structural matrix capacities compiled out of the engine registry map."
  value = {
    total_ips     = data.cidrblock_pool.telemetry.metrics.total_ips
    allocated_ips = data.cidrblock_pool.telemetry.metrics.allocated_ips
    reserved_ips  = data.cidrblock_pool.telemetry.metrics.reserved_ips
    available_ips = data.cidrblock_pool.telemetry.metrics.available_ips
  }
}

output "unallocated_expansion_slices" {
  description = "Contiguous unallocated chunks discovered inside the matrix. Use these boundaries for future Azure scale operations."
  value       = data.cidrblock_pool.telemetry.available_slices
}

