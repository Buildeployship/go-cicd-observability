job "echo" {
  datacenters = ["dc1"]
  type        = "service"

  group "echo" {
    count = 1

    network {
      mode = "bridge"
      port "http" {
        static = 9090
      }
    }

    service {
      name = "echo"
      port = "9090"

      connect {
        sidecar_service {}
      }

      check {
        type     = "http"
        path     = "/"
        interval = "10s"
        timeout  = "2s"
        expose   = true
      }
    }

    task "echo" {
      driver = "docker"

      config {
        image = "hashicorp/http-echo"
        args  = ["-listen=:9090", "-text=echo-healthy"]
      }

      resources {
        cpu    = 50
        memory = 64
      }
    }
  }
}
