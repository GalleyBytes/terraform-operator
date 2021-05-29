terraform {
  backend "consul" {
    address = "hashicorp-consul-server.tf-system:8500"
    scheme  = "http"
    path    = "terraform/${TFO_NAMESPACE}/${TFO_RUNNER}.tfstate"
  }
}
