# Azure VNet Subnet Topology Example
# Leverages the cidrblock provider to orchestrate deterministic VNet and subnet topologies

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

# Provider configuration implementing production safety guardrails
provider "cidrblock" {
  # Safety Guardrail: Explicitly blocks any terraform apply passes that attempt
  # to delete active subnet keys from an existing pool resource.
  prevent_subnet_destruction = true
}

provider "azurerm" {
  features {}
}

# Define the IPAM calculation pool for the Azure landing zone
resource "cidrblock_pool" "azure_subnets" {
  cidr                = "10.10.0.0/16"
  organization        = var.organization
  project             = var.project_id
  network             = "vnet-primary"
  allocation_strategy = "FIRST"

  allocations = {
    # PASS 1: Evaluated First due to FFD Size Sorting (Largest footprint: /22).
    # Claims the lowest naturally aligned free gap at 10.10.0.0/22.
    # Spawns a left-hand aligned buddy reservation at 10.10.4.0/22 for zonal expansion.
    # Total combined footprint consumed: 10.10.0.0 to 10.10.7.255
    app_tier = {
      prefix_size     = 22
      reserve_sibling = true
    }

    # PASS 2: Evaluated Second due to FFD Size Sorting (Smaller footprint: /24).
    # Scans from the bottom of the pool, skipping past the active app_tier 
    # and its locked buddy block space.
    # Securely claims the next available aligned block boundary at 10.10.8.0/24.
    web_tier = {
      prefix_size     = 24
      reserve_sibling = false
    }
  }
}

# Core Azure Resource Group Container
resource "azurerm_resource_group" "main" {
  name     = "rg-${var.project_id}-networking"
  location = var.azure_location
}

# Base Azure Virtual Network (VNet) Container
resource "azurerm_virtual_network" "main" {
  name                = "vnet-primary"
  resource_group_name = azurerm_resource_group.main.name
  location            = azurerm_resource_group.main.location
  address_space       = [cidrblock_pool.azure_subnets.cidr]
}

# Active Application Subnet (Primary Zone)
resource "azurerm_subnet" "app_active" {
  name                 = "snet-app-active"
  resource_group_name  = azurerm_resource_group.main.name
  virtual_network_name = azurerm_virtual_network.main.name
  address_prefixes     = [cidrblock_pool.azure_subnets.allocations["app_tier"].allocated_cidr]
}

# High-Availability Expansion Subnet (Secondary Zone)
# Consumes the guaranteed uncollided forward sibling block calculated by the engine
resource "azurerm_subnet" "app_expansion" {
  name                 = "snet-app-ha-expansion"
  resource_group_name  = azurerm_resource_group.main.name
  virtual_network_name = azurerm_virtual_network.main.name
  address_prefixes     = [cidrblock_pool.azure_subnets.allocations["app_tier"].sibling_cidr]
}

# DMZ Public Web Subnet
resource "azurerm_subnet" "web" {
  name                 = "snet-web"
  resource_group_name  = azurerm_resource_group.main.name
  virtual_network_name = azurerm_virtual_network.main.name
  address_prefixes     = [cidrblock_pool.azure_subnets.allocations["web_tier"].allocated_cidr]
}

# ==========================================
# Topology Outputs
# ==========================================

output "vnet_id" {
  value = azurerm_virtual_network.main.id
}

output "app_active_cidr" {
  value = azurerm_subnet.app_active.address_prefixes[0]
}

output "app_expansion_reserved_sibling_cidr" {
  value       = azurerm_subnet.app_expansion.address_prefixes[0]
  description = "Verified, left-hand aligned buddy sibling used for seamless multi-zone clustering"
}

output "web_cidr" {
  value       = azurerm_subnet.web.address_prefixes[0]
  description = "Calculated CIDR block for public web tier"
}

