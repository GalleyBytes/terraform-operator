package main

import (
	"flag"

	"github.com/galleybytes/terraform-operator/pkg/apis/tf/v1beta1"
)

var (
	templatefile string
	outputfile   string
)

func init() {
	flag.StringVar(&templatefile, "tpl", "", "Path to the template")
	flag.StringVar(&outputfile, "out", "", "Path to save rendered template")
	flag.Parse()
}

func main() {
	v1beta1.Generate(templatefile, outputfile)
}
