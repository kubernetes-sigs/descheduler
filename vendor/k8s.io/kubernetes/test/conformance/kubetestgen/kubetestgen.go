/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/go-openapi/analysis"
	"github.com/go-openapi/loads"
	"github.com/go-openapi/spec"
	"gopkg.in/yaml.v2"
)

type options struct {
	schemaPath   string
	resource     string
	area         string
	behaviorsDir string
}

func parseFlags() *options {
	o := &options{}
	flag.StringVar(&o.schemaPath, "schema", "", "Path to the OpenAPI schema")
	flag.StringVar(&o.resource, "resource", ".*", "Resource name")
	flag.StringVar(&o.area, "area", "default", "Area name to use")
	flag.StringVar(&o.behaviorsDir, "dir", "../behaviors/", "Path to the behaviors directory")
	flag.Parse()
	return o
}

var defMap map[string]analysis.SchemaRef

func main() {
	defMap = make(map[string]analysis.SchemaRef)
	o := parseFlags()

	d, err := loads.JSONSpec(o.schemaPath)
	if err != nil {
		fmt.Printf("ERROR: %s\n", err.Error())
		os.Exit(1)
	}
	defs := d.Analyzer.AllDefinitions()
	sort.Slice(defs, func(i, j int) bool { return defs[i].Name < defs[j].Name })

	for _, d := range defs {
		if !d.TopLevel {
			continue
		}
		defMap[d.Ref.String()] = d
	}

	var suites []Suite
	var suiteMapping = make(map[string]*Suite)

	for _, v := range defs {
		if !v.TopLevel || o.resource != v.Name {
			continue
		}
		name := trimObjectName(v.Name)

		defaultsuite := Suite{
			Suite:       o.area + "/spec",
			Description: "Base suite for " + o.area,
			Behaviors:   []Behavior{},
		}

		_ = defaultsuite

		for p, propSchema := range v.Schema.Properties {
			id := o.area + p + "/"

			if propSchema.Ref.String() != "" || propSchema.Type[0] == "array" {
				if _, ok := suiteMapping[id]; !ok {
					newsuite := Suite{
						Suite:       o.area + "/" + p,
						Description: "Suite for " + o.area + "/" + p,
						Behaviors:   []Behavior{},
					}
					suiteMapping[id] = &newsuite
				}
				behaviors := suiteMapping[id].Behaviors
				behaviors = append(behaviors, schemaBehavior(o.area, name, p, propSchema)...)
				suiteMapping[id].Behaviors = behaviors
			} else {
				if _, ok := suiteMapping["default"]; !ok {
					newsuite := Suite{
						Suite:       o.area + "/spec",
						Description: "Base suite for " + o.area,
						Behaviors:   []Behavior{},
					}
					suiteMapping["default"] = &newsuite
				}

				behaviors := suiteMapping["default"].Behaviors
				behaviors = append(behaviors, schemaBehavior(o.area, name, p, propSchema)...)
				suiteMapping["default"].Behaviors = behaviors

			}
		}
		for _, v := range suiteMapping {
			suites = append(suites, *v)
		}

		break
	}

	var area Area = Area{o.area, suites}
	countFields(suites)
	printYAML(o.behaviorsDir+o.area, area)
}

func printYAML(fileName string, areaO Area) {
	f, err := os.Create(fileName + ".yaml")
	if err != nil {
		fmt.Printf("ERROR: %s\n", err.Error())
	}
	defer f.Close()
	y, err := yaml.Marshal(areaO)
	if err != nil {
		fmt.Printf("ERROR: %s\n", err.Error())
		os.Exit(1)
	}

	f.WriteString(string(y))
}

func countFields(suites []Suite) {
	var fieldsMapping map[string]int
	fieldsMapping = make(map[string]int)
	for _, suite := range suites {
		for _, behavior := range suite.Behaviors {
			if _, exists := fieldsMapping[behavior.APIType]; exists {
				fieldsMapping[behavior.APIType]++
			} else {
				fieldsMapping[behavior.APIType] = 1
			}
		}
	}
	for k, v := range fieldsMapping {
		fmt.Printf("Type %v, Count %v\n", k, v)
	}
}

func trimObjectName(name string) string {
	if strings.Index(name, "#/definitions/") == 0 {
		name = name[len("#/definitions/"):]
	}
	if strings.Index(name, "io.k8s.api.") == 0 {
		return name[len("io.k8s.api."):]
	}
	return name
}

func objectBehaviors(id string, s *spec.Schema) []Behavior {
	if strings.Contains(id, "openAPIV3Schema") || strings.Contains(id, "JSONSchema") || strings.Contains(s.Ref.String(), "JSONSchema") {
		return []Behavior{}
	}

	ref, ok := defMap[s.Ref.String()]
	if !ok {
		return []Behavior{}
	}

	return schemaBehaviors(id, trimObjectName(ref.Name), ref.Schema)
}

func schemaBehaviors(base, apiObject string, s *spec.Schema) []Behavior {
	var behaviors []Behavior
	for p, propSchema := range s.Properties {
		b := schemaBehavior(base, apiObject, p, propSchema)
		behaviors = append(behaviors, b...)
	}
	return behaviors
}

func schemaBehavior(base, apiObject, p string, propSchema spec.Schema) []Behavior {

	id := strings.Join([]string{base, p}, "/")
	if propSchema.Ref.String() != "" {
		if apiObject == trimObjectName(propSchema.Ref.String()) {
			return []Behavior{}
		}
		return objectBehaviors(id, &propSchema)
	}
	var b []Behavior
	switch propSchema.Type[0] {
	case "array":
		b = objectBehaviors(id, propSchema.Items.Schema)
	case "boolean":
		b = []Behavior{
			{
				ID:          id,
				APIObject:   apiObject,
				APIField:    p,
				APIType:     propSchema.Type[0],
				Description: "Boolean set to true. " + propSchema.Description,
			},
			{
				ID:          id,
				APIObject:   apiObject,
				APIField:    p,
				APIType:     propSchema.Type[0],
				Description: "Boolean set to false. " + propSchema.Description,
			},
		}
	default:
		b = []Behavior{{
			ID:          id,
			APIObject:   apiObject,
			APIField:    p,
			APIType:     propSchema.Type[0],
			Description: propSchema.Description,
		}}
	}
	return b
}
