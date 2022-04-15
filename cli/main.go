package main

import (
	"github.com/isaaguilar/terraform-operator/cli/cmd"
)

var version string

func main() {
	cmd.Execute(version)
}
