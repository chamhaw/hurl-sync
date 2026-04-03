package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type syncCounts struct {
	deleted     int
	created     int
	moved       int
	overwritten int
	merged      int
}

func executeSync(endpoints []Endpoint, hurlFiles []HurlFile, hurlDir string, dryRun bool) int {
	hurlByOpID := make(map[string]HurlFile)
	for _, hf := range hurlFiles {
		if hf.OperationID != "" {
			hurlByOpID[hf.OperationID] = hf
		}
	}

	endpointByOpID := make(map[string]Endpoint)
	for _, ep := range endpoints {
		endpointByOpID[ep.OperationID] = ep
	}

	prefix := ""
	if dryRun {
		prefix = "[dry-run] "
	}

	var counts syncCounts

	// Process each endpoint (MISSING, STALE, MISPLACED)
	for _, ep := range endpoints {
		tag := ep.Tag
		if tag == "" {
			tag = "default"
		}
		expectedRel := filepath.Join(tagToDir(tag), toKebabCase(ep.OperationID)+".hurl")
		expectedPath := filepath.Join(hurlDir, expectedRel)

		hf, found := hurlByOpID[ep.OperationID]

		switch {
		case !found:
			// MISSING: create skeleton
			fmt.Printf("  %screate   %s  (MISSING)\n", prefix, expectedRel)
			if !dryRun {
				if err := syncCreate(ep, expectedPath, hurlDir); err != nil {
					fmt.Fprintf(os.Stderr, "error creating %s: %v\n", expectedRel, err)
					return 1
				}
			}
			counts.created++

		case hf.SpecHash != ep.SpecHash:
			// STALE: check request-hash to decide overwrite vs merge
			actualRel, _ := filepath.Rel(hurlDir, hf.Path)
			newReqHash := computeRequestBodyHash(ep)

			if newReqHash == "" || hf.RequestHash == "" || hf.RequestHash == newReqHash {
				// Situation A: request-hash unchanged (or no requestBody)
				fmt.Printf("  %soverwrite %s  (STALE, request body unchanged)\n", prefix, actualRel)
				if !dryRun {
					if err := syncOverwrite(ep, hf.Path); err != nil {
						fmt.Fprintf(os.Stderr, "error overwriting %s: %v\n", actualRel, err)
						return 1
					}
				}
				counts.overwritten++
			} else {
				// Situation B: request-hash changed, merge
				fmt.Printf("  %smerge    %s  (STALE, request body changed)\n", prefix, actualRel)
				if !dryRun {
					if err := syncMerge(ep, hf.Path, newReqHash); err != nil {
						fmt.Fprintf(os.Stderr, "error merging %s: %v\n", actualRel, err)
						return 1
					}
				}
				counts.merged++
			}

			// After overwrite/merge, check if file is also misplaced
			if hf.Path != expectedPath {
				expectedDir, _ := filepath.Rel(hurlDir, filepath.Dir(expectedPath))
				fmt.Printf("  %smove     %s → %s  (also moved to %s)\n", prefix, actualRel, expectedRel, expectedDir)
				if !dryRun {
					if err := os.MkdirAll(filepath.Dir(expectedPath), 0o755); err != nil {
						fmt.Fprintf(os.Stderr, "error moving %s: %v\n", actualRel, err)
						return 1
					}
					if err := os.Rename(hf.Path, expectedPath); err != nil {
						fmt.Fprintf(os.Stderr, "error moving %s: %v\n", actualRel, err)
						return 1
					}
					// Move companion .json response file if exists
					srcJSON := strings.TrimSuffix(hf.Path, ".hurl") + ".json"
					dstJSON := strings.TrimSuffix(expectedPath, ".hurl") + ".json"
					if _, err := os.Stat(srcJSON); err == nil {
						os.Rename(srcJSON, dstJSON)
					}
					// Remove source directory if empty
					srcDir := filepath.Dir(hf.Path)
					if entries, err := os.ReadDir(srcDir); err == nil && len(entries) == 0 {
						os.Remove(srcDir)
					}
				}
				counts.moved++
			}

		default:
			// spec-hash matches, check location
			actualRel, _ := filepath.Rel(hurlDir, hf.Path)
			if actualRel != expectedRel {
				// MISPLACED: move
				fmt.Printf("  %smove     %s → %s  (MISPLACED)\n", prefix, actualRel, expectedRel)
				if !dryRun {
					if err := syncMove(hf.Path, expectedPath, ep); err != nil {
						fmt.Fprintf(os.Stderr, "error moving %s: %v\n", actualRel, err)
						return 1
					}
				}
				counts.moved++
			}
			// OK: skip
		}
	}

	// Check for orphan hurl files
	for _, hf := range hurlFiles {
		if hf.OperationID == "" {
			continue
		}
		if _, found := endpointByOpID[hf.OperationID]; !found {
			rel, _ := filepath.Rel(hurlDir, hf.Path)
			fmt.Printf("  %sdelete   %s  (ORPHAN)\n", prefix, rel)
			if !dryRun {
				if err := os.Remove(hf.Path); err != nil {
					fmt.Fprintf(os.Stderr, "error deleting %s: %v\n", rel, err)
					return 1
				}
			}
			counts.deleted++
		}
	}

	// Summary
	fmt.Printf("\n--- sync summary ---\n")
	fmt.Printf("deleted:     %d\n", counts.deleted)
	fmt.Printf("created:     %d\n", counts.created)
	fmt.Printf("moved:       %d\n", counts.moved)
	fmt.Printf("overwritten: %d\n", counts.overwritten)
	fmt.Printf("merged:      %d\n", counts.merged)

	return 0
}

func syncCreate(ep Endpoint, path, _ string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	content := generateHurlSkeleton(ep, "{{base_url}}")
	return os.WriteFile(path, []byte(content), 0o644)
}

func syncOverwrite(ep Endpoint, path string) error {
	content := generateHurlSkeleton(ep, "{{base_url}}")
	return os.WriteFile(path, []byte(content), 0o644)
}

func syncMove(src, dst string, ep Endpoint) error {
	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.Rename(src, dst); err != nil {
		return err
	}
	// Move companion .json response file if exists
	srcJSON := strings.TrimSuffix(src, ".hurl") + ".json"
	dstJSON := strings.TrimSuffix(dst, ".hurl") + ".json"
	if _, err := os.Stat(srcJSON); err == nil {
		os.Rename(srcJSON, dstJSON)
	}
	// Remove source directory if empty
	srcDir := filepath.Dir(src)
	if entries, err := os.ReadDir(srcDir); err == nil && len(entries) == 0 {
		os.Remove(srcDir)
	}
	// Update spec-hash and request-hash in file header
	return updateFileHashes(dst, ep)
}

func updateFileHashes(path string, ep Endpoint) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
	var result []string
	hashWritten := false
	reqHashWritten := false
	reqHash := computeRequestBodyHash(ep)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# spec-hash:") {
			result = append(result, fmt.Sprintf("# spec-hash: %s", ep.SpecHash))
			hashWritten = true
			// Insert request-hash right after spec-hash if needed
			if reqHash != "" && !reqHashWritten {
				result = append(result, fmt.Sprintf("# request-hash: %s", reqHash))
				reqHashWritten = true
			}
			continue
		}
		if strings.HasPrefix(trimmed, "# request-hash:") {
			// Skip old request-hash (already handled above)
			continue
		}
		result = append(result, line)
	}

	if !hashWritten {
		// Shouldn't happen, but handle gracefully
		return nil
	}

	return os.WriteFile(path, []byte(strings.Join(result, "\n")), 0o644)
}

func syncMerge(ep Endpoint, path, newReqHash string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	content := string(data)
	lines := strings.Split(content, "\n")

	// 1. Update header hashes
	var header []string
	bodyStart := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#") {
			bodyStart = i
			break
		}
		if strings.HasPrefix(trimmed, "# spec-hash:") {
			header = append(header, fmt.Sprintf("# spec-hash: %s", ep.SpecHash))
			if newReqHash != "" {
				header = append(header, fmt.Sprintf("# request-hash: %s", newReqHash))
			}
		} else if strings.HasPrefix(trimmed, "# request-hash:") {
			// Skip old request-hash (handled with spec-hash)
			continue
		} else {
			header = append(header, line)
		}
	}

	// 2. Parse old body fields from the file
	oldFields := parseBodyFields(lines[bodyStart:])

	// 3. Get new fields from endpoint schema
	newFields := getSchemaFields(ep.BodySchema)

	// 4. Merge
	mergedBody := mergeBodyFields(oldFields, newFields)

	// 5. Reconstruct: header + method line + content-type + merged body + rest (Options, HTTP, Asserts)
	var result []string
	result = append(result, header...)

	// Find and include everything from method line to body start, and after body end
	restLines := lines[bodyStart:]
	inBody := false
	bodyDone := false
	braceDepth := 0
	var beforeBody []string
	var afterBody []string

	for _, line := range restLines {
		trimmed := strings.TrimSpace(line)
		if !inBody && !bodyDone {
			if trimmed == "{" {
				inBody = true
				braceDepth = 1
				beforeBody = append(beforeBody, line[:len(line)-len(trimmed)]+"{") // preserve pre-brace content
				continue
			}
			beforeBody = append(beforeBody, line)
			continue
		}
		if inBody {
			for _, ch := range trimmed {
				if ch == '{' {
					braceDepth++
				} else if ch == '}' {
					braceDepth--
				}
			}
			if braceDepth <= 0 {
				inBody = false
				bodyDone = true
			}
			continue
		}
		// After body
		afterBody = append(afterBody, line)
	}

	result = append(result, beforeBody...)
	if mergedBody != "" {
		result = append(result, mergedBody)
	}
	result = append(result, afterBody...)

	return os.WriteFile(path, []byte(strings.Join(result, "\n")), 0o644)
}

// bodyField represents a field parsed from a hurl request body.
type bodyField struct {
	name      string
	typeName  string
	valueLine string // the "key": value line
	comment   string // the # fieldName: type line
	removed   bool   // true if marked as REMOVED
}

// parseBodyFields parses field comment+value pairs from hurl file lines (within the JSON body).
func parseBodyFields(lines []string) []bodyField {
	var fields []bodyField
	inBody := false
	braceDepth := 0
	var pendingComment string
	var pendingType string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if !inBody {
			if trimmed == "{" {
				inBody = true
				braceDepth = 1
				continue
			}
			continue
		}

		// Track brace depth
		for _, ch := range trimmed {
			if ch == '{' {
				braceDepth++
			} else if ch == '}' {
				braceDepth--
			}
		}
		if braceDepth <= 0 {
			break
		}
		if braceDepth > 1 {
			continue // skip nested objects
		}

		// Comment line: # fieldName: type
		if strings.HasPrefix(trimmed, "# REMOVED:") {
			// Extract field name from removed line
			removedPart := strings.TrimPrefix(trimmed, "# REMOVED:")
			removedPart = strings.TrimSpace(removedPart)
			name := extractFieldName(removedPart)
			if name != "" {
				fields = append(fields, bodyField{
					name:      name,
					valueLine: line,
					removed:   true,
				})
			}
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			commentContent := strings.TrimSpace(strings.TrimPrefix(trimmed, "#"))
			parts := strings.SplitN(commentContent, ":", 2)
			if len(parts) == 2 {
				pendingComment = strings.TrimSpace(parts[0])
				pendingType = strings.TrimSpace(parts[1])
				pendingType = strings.TrimSuffix(pendingType, " (required)")
			}
			continue
		}

		// Value line: "fieldName": value
		name := extractFieldName(trimmed)
		if name != "" {
			fields = append(fields, bodyField{
				name:      name,
				typeName:  pendingType,
				valueLine: line,
				comment:   fmt.Sprintf("  # %s: %s", pendingComment, pendingType),
			})
			pendingComment = ""
			pendingType = ""
		}
	}

	return fields
}

func extractFieldName(line string) string {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "\"") {
		return ""
	}
	// Find closing quote
	end := strings.Index(line[1:], "\"")
	if end < 0 {
		return ""
	}
	return line[1 : end+1]
}

// getSchemaFields extracts field names and types from a JSON schema.
func getSchemaFields(schema interface{}) map[string]string {
	result := make(map[string]string)
	m, ok := schema.(map[string]interface{})
	if !ok {
		return result
	}

	props, ok := m["properties"].(map[string]interface{})
	if !ok {
		return result
	}

	for name, prop := range props {
		typeName := "unknown"
		if pm, ok := prop.(map[string]interface{}); ok {
			if t, ok := pm["type"].(string); ok {
				typeName = t
			}
		}
		result[name] = typeName
	}

	return result
}

func mergeBodyFields(oldFields []bodyField, newSchema map[string]string) string {
	// Build sets
	oldByName := make(map[string]bodyField)
	var oldOrder []string
	for _, f := range oldFields {
		if !f.removed {
			oldByName[f.name] = f
			oldOrder = append(oldOrder, f.name)
		}
	}

	// Find new fields (in schema but not in old)
	var newNames []string
	for name := range newSchema {
		if _, exists := oldByName[name]; !exists {
			newNames = append(newNames, name)
		}
	}
	sortStrings(newNames)

	// Build merged body
	var b strings.Builder
	b.WriteString("{\n")

	// Existing fields: keep or mark removed
	var keptFields []string
	for _, name := range oldOrder {
		if _, inNew := newSchema[name]; inNew {
			keptFields = append(keptFields, name)
		}
	}
	// Add new fields
	allFields := append(keptFields, newNames...)

	// Removed fields
	var removedLines []string
	for _, name := range oldOrder {
		if _, inNew := newSchema[name]; !inNew {
			f := oldByName[name]
			valueLine := strings.TrimSpace(f.valueLine)
			valueLine = strings.TrimSuffix(valueLine, ",")
			removedLines = append(removedLines, fmt.Sprintf("  # REMOVED: %s", valueLine))
		}
	}

	totalLines := len(allFields) + len(removedLines)
	lineNum := 0

	for _, name := range allFields {
		lineNum++
		isLast := lineNum == totalLines && len(removedLines) == 0 || (lineNum == len(allFields) && len(removedLines) == 0)

		typeName := newSchema[name]
		fmt.Fprintf(&b, "  # %s: %s\n", name, typeName)

		comma := ","
		if isLast {
			comma = ""
		}

		if f, exists := oldByName[name]; exists {
			// Preserve user's value
			val := extractFieldValue(f.valueLine)
			fmt.Fprintf(&b, "  %s: %s%s\n", jsonQuote(name), val, comma)
		} else {
			// New field with TODO value
			val := todoValueForType(typeName)
			fmt.Fprintf(&b, "  %s: %s%s\n", jsonQuote(name), val, comma)
		}
	}

	for _, rl := range removedLines {
		b.WriteString(rl + "\n")
	}

	b.WriteString("}")
	return b.String()
}

func extractFieldValue(line string) string {
	line = strings.TrimSpace(line)
	// Find ": " after the key
	idx := strings.Index(line, ":")
	if idx < 0 {
		return `"TODO"`
	}
	val := strings.TrimSpace(line[idx+1:])
	val = strings.TrimSuffix(val, ",")
	return val
}
