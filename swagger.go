package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// rawSwagger holds the entire swagger document for $ref resolution.
var rawSwagger map[string]interface{}

// SwaggerSpec represents the top-level swagger.json structure.
type SwaggerSpec struct {
	Paths map[string]PathItem `json:"paths"`
}

// PathItem maps HTTP methods to operations.
type PathItem map[string]Operation

// Operation represents a single API operation.
type Operation struct {
	OperationID string              `json:"operationId"`
	Tags        []string            `json:"tags"`
	Parameters  []Parameter         `json:"parameters"`
	RequestBody *RequestBody        `json:"requestBody"` // OpenAPI 3.0
	Responses   map[string]Response `json:"responses"`
}

// Parameter represents a request parameter.
type Parameter struct {
	Name     string      `json:"name"`
	In       string      `json:"in"`
	Required bool        `json:"required"`
	Schema   interface{} `json:"schema"`
}

// RequestBody represents a request body (OpenAPI 3.0).
type RequestBody struct {
	Required bool                 `json:"required"`
	Content  map[string]MediaType `json:"content"`
}

// MediaType represents a media type with schema.
type MediaType struct {
	Schema interface{} `json:"schema"`
}

// Response represents an API response.
type Response struct {
	Description string               `json:"description"`
	Content     map[string]MediaType `json:"content"`  // OpenAPI 3.0
	Schema      interface{}          `json:"schema"`   // Swagger 2.0
}

// Endpoint is a resolved path+method with its operation details.
type Endpoint struct {
	Path        string
	Method      string
	OperationID string
	Tag         string
	Tags        []string
	Operation   Operation
	SpecHash    string
	HasBody     bool        // whether this endpoint accepts a request body
	BodySchema  interface{} // resolved body schema (for scaffold)
}

func parseSwagger(path string) ([]Endpoint, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading swagger file: %w", err)
	}

	// Store raw for $ref resolution
	if err := json.Unmarshal(data, &rawSwagger); err != nil {
		return nil, fmt.Errorf("parsing swagger JSON: %w", err)
	}

	var spec SwaggerSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("parsing swagger JSON: %w", err)
	}

	var endpoints []Endpoint
	for p, pathItem := range spec.Paths {
		for method, op := range pathItem {
			method = strings.ToLower(method)
			opID := op.OperationID
			if opID == "" {
				opID = deriveOperationID(method, p)
			}

			hasBody, bodySchema := extractBodyInfo(op)
			hash := computeSpecHash(p, method, op)

			tag := ""
			if len(op.Tags) > 0 {
				tag = op.Tags[0]
			}

			endpoints = append(endpoints, Endpoint{
				Path:        p,
				Method:      method,
				OperationID: opID,
				Tag:         tag,
				Tags:        op.Tags,
				Operation:   op,
				SpecHash:    hash,
				HasBody:     hasBody,
				BodySchema:  bodySchema,
			})
		}
	}

	sort.Slice(endpoints, func(i, j int) bool {
		if endpoints[i].Path != endpoints[j].Path {
			return endpoints[i].Path < endpoints[j].Path
		}
		return endpoints[i].Method < endpoints[j].Method
	})

	return endpoints, nil
}

// extractBodyInfo returns whether the endpoint has a body and the resolved schema.
func extractBodyInfo(op Operation) (bool, interface{}) {
	// OpenAPI 3.0: requestBody
	if op.RequestBody != nil {
		for _, mt := range op.RequestBody.Content {
			return true, resolveRef(mt.Schema)
		}
		return true, nil
	}

	// Swagger 2.0: parameter with in=body
	for _, p := range op.Parameters {
		if p.In == "body" {
			return true, resolveRef(p.Schema)
		}
	}

	return false, nil
}

// resolveRef resolves a $ref pointer in the swagger document.
func resolveRef(schema interface{}) interface{} {
	m, ok := schema.(map[string]interface{})
	if !ok {
		return schema
	}

	ref, ok := m["$ref"].(string)
	if !ok {
		return schema
	}

	// Parse #/definitions/Foo
	if !strings.HasPrefix(ref, "#/") {
		return schema
	}

	parts := strings.Split(ref[2:], "/")
	var current interface{} = rawSwagger
	for _, part := range parts {
		cm, ok := current.(map[string]interface{})
		if !ok {
			return schema
		}
		current = cm[part]
	}

	if current != nil {
		return current
	}
	return schema
}

// deriveOperationID generates an operationId from method + path.
func deriveOperationID(method, path string) string {
	p := strings.TrimPrefix(path, "/api/")
	p = strings.TrimPrefix(p, "v1/")
	p = strings.TrimPrefix(p, "v2/")

	parts := strings.Split(p, "/")
	var words []string
	for _, part := range parts {
		if part == "" {
			continue
		}
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			param := part[1 : len(part)-1]
			words = append(words, "By"+capitalize(param))
			continue
		}
		segments := strings.Split(part, "-")
		for _, seg := range segments {
			words = append(words, capitalize(seg))
		}
	}

	return method + strings.Join(words, "")
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

type hashInput struct {
	Path           string      `json:"path"`
	Method         string      `json:"method"`
	Parameters     []hashParam `json:"parameters"`
	RequestBody    interface{} `json:"requestBody,omitempty"`
	ResponseSchema interface{} `json:"responseSchema,omitempty"`
}

type hashParam struct {
	Name     string      `json:"name"`
	In       string      `json:"in"`
	Required bool        `json:"required"`
	Schema   interface{} `json:"schema,omitempty"`
}

// computeRequestBodyHash hashes only the requestBody schema (not path/method/parameters/responses).
// Returns SHA256 first 8 bytes hex. Returns empty string if no requestBody.
func computeRequestBodyHash(ep Endpoint) string {
	var reqBody interface{}

	// OpenAPI 3.0: requestBody
	if ep.Operation.RequestBody != nil {
		reqBody = ep.Operation.RequestBody
	} else {
		// Swagger 2.0: parameter with in=body
		for _, p := range ep.Operation.Parameters {
			if p.In == "body" {
				reqBody = p.Schema
				break
			}
		}
	}

	if reqBody == nil {
		return ""
	}

	data, _ := json.Marshal(reqBody)
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:8])
}

func computeSpecHash(path, method string, op Operation) string {
	// Exclude body parameters from param list (they go into requestBody)
	var params []hashParam
	for _, p := range op.Parameters {
		if p.In == "body" {
			continue
		}
		params = append(params, hashParam{
			Name:     p.Name,
			In:       p.In,
			Required: p.Required,
			Schema:   p.Schema,
		})
	}
	sort.Slice(params, func(i, j int) bool {
		return params[i].Name < params[j].Name
	})

	// Request body: OpenAPI 3.0 or Swagger 2.0
	var reqBody interface{}
	if op.RequestBody != nil {
		reqBody = op.RequestBody
	} else {
		for _, p := range op.Parameters {
			if p.In == "body" {
				reqBody = p.Schema
				break
			}
		}
	}

	// Response schema: try both formats
	var respSchema interface{}
	for _, code := range []string{"200", "201"} {
		resp, ok := op.Responses[code]
		if !ok {
			continue
		}
		// Swagger 2.0
		if resp.Schema != nil {
			respSchema = resp.Schema
			break
		}
		// OpenAPI 3.0
		if resp.Content != nil {
			for _, mt := range resp.Content {
				respSchema = mt.Schema
				break
			}
		}
		break
	}

	input := hashInput{
		Path:           path,
		Method:         method,
		Parameters:     params,
		RequestBody:    reqBody,
		ResponseSchema: respSchema,
	}

	data, _ := json.Marshal(input)
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:8])
}
