package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

func generateHurlSkeleton(ep Endpoint, baseURL string) string {
	var b strings.Builder

	// Header comments
	fmt.Fprintf(&b, "# operationId: %s\n", ep.OperationID)
	if ep.Tag != "" {
		fmt.Fprintf(&b, "# tag: %s\n", ep.Tag)
	}
	fmt.Fprintf(&b, "# spec-hash: %s\n", ep.SpecHash)
	if reqHash := computeRequestBodyHash(ep); reqHash != "" {
		fmt.Fprintf(&b, "# request-hash: %s\n", reqHash)
	}

	// Convert path params: {id} → {{id}}
	urlPath := convertPathParams(ep.Path)
	method := strings.ToUpper(ep.Method)

	fmt.Fprintf(&b, "%s %s%s\n", method, baseURL, urlPath)

	// Content-Type and request body
	if ep.HasBody {
		fmt.Fprintf(&b, "Content-Type: application/json\n")
		fmt.Fprintf(&b, "\n")
		body := buildBodySkeleton(ep.BodySchema)
		b.WriteString(body)
	}

	// [Options] output — save response body to same directory as .hurl file
	outputFile := toKebabCase(ep.OperationID) + ".json"
	fmt.Fprintf(&b, "\n[Options]\n")
	fmt.Fprintf(&b, "output: %s\n", outputFile)

	// Expected response
	respCode := guessSuccessCode(ep)
	fmt.Fprintf(&b, "\nHTTP %s\n", respCode)
	fmt.Fprintf(&b, "[Asserts]\n")
	fmt.Fprintf(&b, "status == %s\n", respCode)

	return b.String()
}

func convertPathParams(path string) string {
	var result strings.Builder
	i := 0
	for i < len(path) {
		if path[i] == '{' {
			j := strings.IndexByte(path[i:], '}')
			if j < 0 {
				result.WriteByte(path[i])
				i++
				continue
			}
			paramName := path[i+1 : i+j]
			fmt.Fprintf(&result, "{{%s}}", paramName)
			i = i + j + 1
		} else {
			result.WriteByte(path[i])
			i++
		}
	}
	return result.String()
}

func buildBodySkeleton(schema interface{}) string {
	if schema == nil {
		return "{}\n"
	}

	m, ok := schema.(map[string]interface{})
	if !ok {
		return "{}\n"
	}

	return renderSchemaBody(m)
}

func renderSchemaBody(schema map[string]interface{}) string {
	props, ok := schema["properties"].(map[string]interface{})
	if !ok || len(props) == 0 {
		return "{}\n"
	}

	// Get required fields
	requiredSet := make(map[string]bool)
	if req, ok := schema["required"].([]interface{}); ok {
		for _, r := range req {
			if s, ok := r.(string); ok {
				requiredSet[s] = true
			}
		}
	}

	// Collect and sort field names for deterministic output
	names := make([]string, 0, len(props))
	for name := range props {
		names = append(names, name)
	}
	sortStrings(names)

	var b strings.Builder
	b.WriteString("{\n")
	for i, name := range names {
		propSchema, _ := props[name].(map[string]interface{})
		typeName := "unknown"
		if propSchema != nil {
			if t, ok := propSchema["type"].(string); ok {
				typeName = t
			}
		}

		reqTag := ""
		if requiredSet[name] {
			reqTag = " (required)"
		}

		fmt.Fprintf(&b, "  # %s: %s%s\n", name, typeName, reqTag)

		val := todoValueForType(typeName)
		comma := ","
		if i == len(names)-1 {
			comma = ""
		}
		fmt.Fprintf(&b, "  %s: %s%s\n", jsonQuote(name), val, comma)
	}
	b.WriteString("}\n")
	return b.String()
}

func todoValueForType(typeName string) string {
	switch typeName {
	case "integer", "number":
		return "0"
	case "boolean":
		return "false"
	case "array":
		return "[]"
	case "object":
		return "{}"
	default:
		return `"TODO"`
	}
}

func jsonQuote(s string) string {
	data, _ := json.Marshal(s)
	return string(data)
}

func guessSuccessCode(ep Endpoint) string {
	for _, code := range []string{"200", "201", "202", "204"} {
		if _, ok := ep.Operation.Responses[code]; ok {
			return code
		}
	}
	return "200"
}

// sortStrings sorts a slice of strings in place (avoids importing sort).
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
