variable "supernet_cidr" {
  type        = string
  description = "The target IP space assigned to the allocation container pool"
}

variable "allocation_strategy" {
  type        = string
  description = "The mathematical gap selection algorithm (FIRST, BEST, SPARSE)"
}

variable "active_allocations" {
  type        = map(object({
    prefix_size     = number
    reserve_sibling = bool
  }))
  description = "The layout matrix of requested subnet footprints"
}

