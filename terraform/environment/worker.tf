# Copyright 2021 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

# Config for vuln worker.

################################################################
# Inputs.

variable "env" {
  description = "environment name"
  type        = string
}

variable "project" {
  description = "GCP project"
  type        = string
}

variable "region" {
  description = "GCP region"
  type        = string
}

variable "use_profiler" {
  description = "use Stackdriver Profiler"
  type        = bool
}

variable "min_frontend_instances" {
  description = "minimum number of frontend instances"
  type        = number
}

variable "client_id" {
  description = "OAuth 2 client ID (visit APIs & Services > Credentials)"
  type = string
}

variable "client_secret" {
  description = "OAuth 2 client ID (visit APIs & Services > Credentials, click on client)"
  type = string
}


################################################################
# Cloud Run service.

resource "google_cloud_run_service" "worker" {

  lifecycle {
    ignore_changes = [
      # When we deploy, we may use different clients at different versions.
      # Ignore those changes.
      template[0].metadata[0].annotations["run.googleapis.com/client-name"],
      template[0].metadata[0].annotations["run.googleapis.com/client-version"]
    ]
  }

  name     = "${var.env}-vuln-worker"
  project  = var.project
  location = var.region

  template {
    spec {
      containers {
	# Don't hardcode the image here; get it from GCP. See the "data" block
	# below for more.
	image = data.google_cloud_run_service.worker.template[0].spec[0].containers[0].image
        env {
          name  = "GOOGLE_CLOUD_PROJECT"
          value = var.project
	}
	env {
	  name = "VULN_WORKER_NAMESPACE"
	  value = var.env
	}
	env {
	  name = "VULN_WORKER_REPORT_ERRORS"
	  value = true
	}
	env {
	  name = "VULN_WORKER_ISSUE_REPO"
	  value = var.env == "dev"? "": "golang/vulndb"
	}
	env{
          name  = "VULN_WORKER_USE_PROFILER"
          value = var.use_profiler
        }
        resources {
          limits = {
            "cpu"    = "1000m"
            "memory" = "2Gi"
          }
        }
      }

      service_account_name = "frontend@${var.project}.iam.gserviceaccount.com"
    }

    metadata {
      annotations = {
        "autoscaling.knative.dev/minScale"  = var.min_frontend_instances
        "autoscaling.knative.dev/maxScale"  = "1"
      }
    }
  }
  autogenerate_revision_name = true

  traffic {
    latest_revision = true
    percent         = 100
  }
}

# We deploy new images with gcloud, not terraform, so we need to
# make sure that "terraform apply" doesn't change the deployed image
# to whatever is in this file. (The image attribute is required in
# a Cloud Run config; it can't be empty.)
#
# We use this data source is used to determine the deployed image.
data "google_cloud_run_service" "worker" {
  name     = "${var.env}-vuln-worker"
  project  = var.project
  location = var.region
}

################################################################
# Load balancer for Cloud Run service.

resource "google_compute_region_network_endpoint_group" "worker" {
  count = var.client_id == ""? 0: 1
  name         = "${var.env}-vuln-worker-neg"
  network_endpoint_type = "SERVERLESS"
  project = var.project
  region = var.region
  cloud_run {
    service = google_cloud_run_service.worker.name
  }
}

module "worker_lb" {
  count = var.client_id == ""? 0: 1
  source  = "GoogleCloudPlatform/lb-http/google//modules/serverless_negs"
  version = "~> 6.1.1"

  name = "${var.env}-vuln-worker-lb"
  project = var.project

  ssl                             = true
  managed_ssl_certificate_domains = ["${var.env}-vuln-worker.go.dev"]
  https_redirect                  = true

  backends = {
    default = {
      description = null
      groups = [
        {
	  group = google_compute_region_network_endpoint_group.worker[0].id
        }
      ]
      enable_cdn              = false
      security_policy         = null
      custom_request_headers  = null
      custom_response_headers = null

      iap_config = {
        enable               = true
        oauth2_client_id     = var.client_id
        oauth2_client_secret = var.client_secret
      }
      log_config = {
        enable      = false
        sample_rate = null
      }
    }
  }
}

output "load_balancer_ip" {
  value = var.client_id == ""? "": module.worker_lb[0].external_ip
}
