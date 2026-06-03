# Azure VNet Subnet Example
# Uses the cidrblock provider to allocate subnets for Azure Virtual Networks

terraform {
  required_providers {
    cidrblock = {
      source  = "finext-networks/cidrblock"
    }
    azurerm = {
      source  = "hashicorp/azurerm"
    }
  }
}

provider "azurerm" {
  features {}
}

# CIDR pool for VNet subnets
resource "cidrblock_pool" "vnet_subnets" {
  cidr         = "10.0.0.0/16"
  organization = var.organization
  project      = var.project_id
  network      = "vnet-main"

  allocations = {
    kubernetes = {
      prefix_size     = 23
      reserve_sibling = true
    }

    aci = {
      prefix_size     = 23
      reserve_sibling = true
    }

    bastion = {
      prefix_size     = 26
      reserve_sibling = false
    }

    app = {
      prefix_size     = 24
      reserve_sibling = false
    }

    data = {
      prefix_size     = 24
      reserve_sibling = false
    }
  }
}

# Resource Group
resource "azurerm_resource_group" "main" {
  name     = "${var.project_id}-rg"
  location = var.location
}

# Azure VNet
resource "azurerm_virtual_network" "main" {
  name                = "vnet-main"
  address_space       = ["10.0.0.0/16"]
  location            = azurerm_resource_group.main.location
  resource_group_name = azurerm_resource_group.main.name
}

# Kubernetes Subnet
resource "azurerm_subnet" "kubernetes" {
  name                 = "kubernetes"
  resource_group_name  = azurerm_resource_group.main.name
  virtual_network_name = azurerm_virtual_network.main.name
  address_prefixes     = [cidrblock_pool.vnet_subnets.allocations["kubernetes"].allocated_cidr]
}

# ACI Subnet
resource "azurerm_subnet" "aci" {
  name                 = "aci"
  resource_group_name  = azurerm_resource_group.main.name
  virtual_network_name = azurerm_virtual_network.main.name
  address_prefixes     = [cidrblock_pool.vnet_subnets.allocations["aci"].allocated_cidr]
}

# Bastion Subnet
resource "azurerm_subnet" "bastion" {
  name                 = "AzureBastionSubnet"
  resource_group_name  = azurerm_resource_group.main.name
  virtual_network_name = azurerm_virtual_network.main.name
  address_prefixes     = [cidrblock_pool.vnet_subnets.allocations["bastion"].allocated_cidr]
}

# App Subnet
resource "azurerm_subnet" "app" {
  name                 = "app"
  resource_group_name  = azurerm_resource_group.main.name
  virtual_network_name = azurerm_virtual_network.main.name
  address_prefixes     = [cidrblock_pool.vnet_subnets.allocations["app"].allocated_cidr]
}

# Data Subnet
resource "azurerm_subnet" "data" {
  name                 = "data"
  resource_group_name  = azurerm_resource_group.main.name
  virtual_network_name = azurerm_virtual_network.main.name
  address_prefixes     = [cidrblock_pool.vnet_subnets.allocations["data"].allocated_cidr]
}

output "vnet_id" {
  value = azurerm_virtual_network.main.id
}

output "subnets" {
  value = {
    kubernetes = cidrblock_pool.vnet_subnets.allocations["kubernetes"].allocated_cidr
    aci        = cidrblock_pool.vnet_subnets.allocations["aci"].allocated_cidr
    bastion    = cidrblock_pool.vnet_subnets.allocations["bastion"].allocated_cidr
    app        = cidrblock_pool.vnet_subnets.allocations["app"].allocated_cidr
    data       = cidrblock_pool.vnet_subnets.allocations["data"].allocated_cidr
  }
}
