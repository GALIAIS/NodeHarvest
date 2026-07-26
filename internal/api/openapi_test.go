package api

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestOpenAPICoversRegisteredAPIRoutes(t *testing.T) {
	specData, err := os.ReadFile("../../docs/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		OpenAPI string                    `yaml:"openapi"`
		Paths   map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(specData, &spec); err != nil {
		t.Fatalf("parse OpenAPI: %v", err)
	}
	if !strings.HasPrefix(spec.OpenAPI, "3.1.") {
		t.Fatalf("OpenAPI version=%q", spec.OpenAPI)
	}

	serverData, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	routePattern := regexp.MustCompile(`HandleFunc\("(GET|POST|PATCH|DELETE) (/api[^"]+)"`)
	for _, match := range routePattern.FindAllStringSubmatch(string(serverData), -1) {
		method, path := strings.ToLower(match[1]), match[2]
		item, ok := spec.Paths[path]
		if !ok {
			t.Errorf("OpenAPI is missing %s %s", match[1], path)
			continue
		}
		if _, referenced := item["$ref"]; !referenced {
			if _, ok := item[method]; !ok {
				t.Errorf("OpenAPI path %s is missing method %s", path, method)
			}
		}
	}
}
