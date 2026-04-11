variable "aws_region" {
  description = "AWS region"
  type        = string
  default     = "us-west-2"
}

variable "project_name" {
  description = "Project name used for resource naming"
  type        = string
  default     = "go-cicd-observability"
}

variable "environment" {
  description = "Environment name"
  type        = string
  default     = "dev"
}
