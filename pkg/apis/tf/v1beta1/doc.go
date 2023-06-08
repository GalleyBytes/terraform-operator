// Package v1beta1 contains API Schema definitions for the tf v1beta1 API group
// +k8s:deepcopy-gen=package,register
// +groupName=tf.galleybytes.com
package v1beta1

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"strings"
	"text/template"

	"github.com/go-openapi/jsonreference"
	"k8s.io/kube-openapi/pkg/common"
	"k8s.io/kube-openapi/pkg/validation/spec"
)

var pkgpath = "github.com/galleybytes/terraform-operator/pkg/apis/tf/v1beta1"

// +kubebuilder:object:generate=false
type Definition struct {
	Ref               spec.Ref `json:"ref"`
	Name              string   `json:"name"`
	Kind              string   `json:"api"`
	Group             string
	Version           string
	OpenAPIDefinition common.OpenAPIDefinition `json:"openAPIDefinition"`
}

// +kubebuilder:object:generate=false
type GenValues struct {
	Definitions map[string][]Definition `json:"definitions"`
}

// +kubebuilder:object:generate=false
type Api struct {
	Name    string
	Version string
	Group   string
}

func (a Api) String() string {
	return fmt.Sprintf("%s %s %s", a.Name, a.Version, a.Group)
}

func Generate(templatefile, outputfile string) {

	genValues := GenValues{
		Definitions: make(map[string][]Definition),
	}

	var callback common.ReferenceCallback = func(path string) spec.Ref {
		ref, err := jsonreference.New(path)
		if err != nil {
			log.Fatal(err)
		}
		return spec.Ref{
			Ref: ref,
		}
	}
	openAPIDefinitions := GetOpenAPIDefinitions(callback)

	for rawpath, openAPIDefinition := range openAPIDefinitions {
		if !isDependedOn(rawpath, openAPIDefinitions) {
			name := strings.Replace(rawpath, pkgpath+".", "", 1)
			api := Api{
				Name:    name,
				Version: SchemeGroupVersion.Version,
				Group:   SchemeGroupVersion.Group,
			}
			genValues.Definitions[api.String()] = []Definition{
				{
					Name:              api.String(),
					OpenAPIDefinition: openAPIDefinition,
					Kind:              api.Name,
					Group:             api.Group,
					Version:           api.Version,
				},
			}
		}
	}

	// For every independent item, generate a new list
	for api, v := range genValues.Definitions {
		var definitions []Definition = v
		dependencies := []string{api}
		for {
			addedNewDeps := false
			for _, definition := range v {
				for _, dependency := range definition.OpenAPIDefinition.Dependencies {
					name := strings.Replace(dependency, pkgpath+".", "", 1)
					api := Api{
						Name:    name,
						Version: SchemeGroupVersion.Version,
						Group:   SchemeGroupVersion.Group,
					}
					if shouldAppendDependency(api.String(), dependencies) {
						if _, found := openAPIDefinitions[dependency]; !found {
							continue
						}
						addedNewDeps = true
						dependencies = append(dependencies, api.String())
						// Strip the pkg name from the type

						definitions = append(definitions, Definition{
							Name:              api.String(),
							OpenAPIDefinition: openAPIDefinitions[dependency],
						})
					}
				}
			}
			v = definitions
			if !addedNewDeps {
				break
			}
		}
		genValues.Definitions[api] = definitions
	}

	tplfileb, err := ioutil.ReadFile(templatefile)
	if err != nil {
		log.Panic(err)
	}
	tplfile := string(tplfileb)
	tpl := template.Must(template.New("").Funcs(Custom).Parse(tplfile))
	var buf bytes.Buffer
	err = tpl.Execute(&buf, genValues)
	if err != nil {
		log.Panic(err)
	}

	err = ioutil.WriteFile(outputfile, buf.Bytes(), 0655)
	if err != nil {
		log.Panic(err)
	}
	log.Printf("%s written\n", outputfile)
}

func shouldAppendDependency(dep string, deps []string) bool {
	for _, d := range deps {
		if dep == d {
			return false
		}
	}
	return true
}

func isDependedOn(name string, openAPIDefinition map[string]common.OpenAPIDefinition) bool {
	for _, item := range openAPIDefinition {
		for _, dependency := range item.Dependencies {
			if dependency == name {
				return true
			}
		}
	}
	return false
}

var Custom = map[string]interface{}{
	"replace": replace,
	"reflink": reflink,
	"refname": refname,
}

func replace(old, new, src string) string {
	return strings.Replace(src, old, new, -1)
}
func refname(s spec.Schema) string {
	apipath := s.Ref.String()
	if strings.Contains(apipath, pkgpath) {
		return strings.Replace(apipath, pkgpath+".", "", 1)
	}
	return apipath
}
func reflink(s spec.Schema) string {
	apipath := s.Ref.String()
	if strings.Contains(apipath, pkgpath) {
		apipath = strings.Replace(apipath, pkgpath+".", "", 1)
		api := Api{
			Name:    apipath,
			Version: SchemeGroupVersion.Version,
			Group:   SchemeGroupVersion.Group,
		}
		return "#" + api.String()
	}

	if strings.Contains(apipath, "k8s.io") {
		// Make an assumption about the pkg link if host is k8s.io
		resourceName := strings.Split(apipath, ".")[len(strings.Split(apipath, "."))-1]
		path := strings.Split(apipath, ".")[0 : len(strings.Split(apipath, "."))-1]
		return "https://pkg.go.dev/" + strings.Join(path, ".") + "#" + resourceName
	}

	return "#" + apipath

}
func typeOf(src interface{}) string {
	return fmt.Sprintf("%T", src)
}
