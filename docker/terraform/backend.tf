terraform {
  backend "consul" {
    address = "hashicorp-consul-server.tf-system:8500"
    scheme  = "http"
    path    = "terraform/${NAMESPACE}/${DEPLOYMENT}.tfstate"
  }
}
