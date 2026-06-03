# Azure Virtual Network Subnet Topology Example
# Uses the cidrblock provider to safely calculate Azure VNet address spaces

terraform {
  required_providers {
    cidrblock = {
      source = "finext-networks/cidrblock"
    }
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 3.0"
    }
  }
}

provider "azurerm" {
  features {}
}

resource "azurerm_resource_group" "rg" {
  name     = "rg-core-networks"
  location = var.azure_location
}

# Define the IPAM calculation pool for the Azure core subscription subscription
resource "cidrblock_pool" "azure_subnets" {
  cidr                = "172.16.0.0/16"
  organization        = var.organization
  project             = var.project_id
  network             = "vnet-hub"
  allocation_strategy = "FIRST" # Keys evaluated alphabetically: aks_tier, mgmt_tier

  allocations = {
    # 1. Processed First Alphabetically: Sized at /22, claims 172.16.0.0/22.
    # Valid: Left-hand aligned block, automatically reserving 172.16.4.0/22 for secondary clusters.
    aks_tier = {
      prefix_size     = 22
      reserve_sibling = true
    }

    # 2. Processed Second: /24 sizing scans from bottom up.
    # Skips active AKS footprint + its reserved expansion sibling block (172.16.0.0 to 172.16.7.255).
    # Claims the next aligned address slot at 172.16.8.0/24.
    mgmt_tier = {
      prefix_size     = 24
      reserve_sibling = false
    }
  }
}

# Base Azure Virtual Network Container Space
resource "azurerm_virtual_network" "vnet" {
  name                = "vnet-hub-primary"
  location            = azurerm_resource_group.rg.location
  resource_group_name = azurerm_resource_group.rg.name
  address_space       = [cidrblock_pool.azure_subnets.cidr]
}

# Primary AKS Cluster Node Pool Subnet
resource "azurerm_subnet" "aks_primary" {
  name                 = "snet-aks-primary"
  resource_group_name  = azurerm_resource_group.rg.name
  virtual_network_name = azurerm_virtual_network.vnet.name
  address_prefixes     = [cidrblock_pool.azure_subnets.allocations["aks_tier"].allocated_cidr]
}

# Secondary AKS Failover/Expansion Subnet
# Directly maps out-of-band to the collision-proof companion sibling calculated by our engine
resource "azurerm_subnet" "aks_secondary_sibling" {
  name                 = "snet-aks-secondary-reserved"
  resource_group_name  = azurerm_resource_group.rg.name
  virtual_network_name = azurerm_virtual_network.vnet.name
  address_prefixes     = [cidrblock_pool.azure_subnets.allocations["aks_tier"].sibling_cidr]
}

# Dedicated Shared Management Services Subnet
resource "azurerm_subnet" "mgmt" {
  name                 = "snet-mgmt-services"
  resource_group_name  = azurerm_resource_group.rg.name
  virtual_network_name = azurerm_virtual_network.vnet.name
  address_prefixes     = [cidrblock_pool.azure_subnets.allocations["mgmt_tier"].allocated_cidr]
}

output "vnet_name" {
  value = azurerm_virtual_network.vnet.name
}

output "aks_primary_prefix" {
  value = azurerm_subnet.aks_primary.address_prefixes[0]
}

output "aks_secondary_sibling_prefix" {
  value       = azurerm_subnet.aks_secondary_sibling.address_prefixes[0]
  description = "Dedicated left-hand aligned buddy reservation reserved natively for horizontal scaling safely"
}

