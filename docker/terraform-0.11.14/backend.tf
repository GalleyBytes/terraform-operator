terraform {
  backend "consul" {
    address = "hashicorp-consul-server.default:8500"
    scheme  = "http"
    path    = "terraform/state/${DEPLOYMENT}.tfstate"
  }
}