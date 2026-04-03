package main

import (
	"fmt"
	"path/filepath"
)

type checkStatus string

const (
	statusOK        checkStatus = "OK"
	statusMissing   checkStatus = "MISSING"
	statusStale     checkStatus = "STALE"
	statusMisplaced checkStatus = "MISPLACED"
	statusOrphan    checkStatus = "ORPHAN"
)

type checkResult struct {
	OperationID string
	Status      checkStatus
	Detail      string
}

func executeCheck(endpoints []Endpoint, hurlFiles []HurlFile, hurlDir string) int {
	// Build lookup: operationId → HurlFile
	hurlByOpID := make(map[string]HurlFile)
	for _, hf := range hurlFiles {
		if hf.OperationID != "" {
			hurlByOpID[hf.OperationID] = hf
		}
	}

	// Build lookup: operationId → Endpoint
	endpointByOpID := make(map[string]Endpoint)
	for _, ep := range endpoints {
		endpointByOpID[ep.OperationID] = ep
	}

	var results []checkResult
	counts := map[checkStatus]int{}

	// Check each swagger endpoint
	for _, ep := range endpoints {
		hf, found := hurlByOpID[ep.OperationID]
		var r checkResult
		switch {
		case !found:
			r = checkResult{
				OperationID: ep.OperationID,
				Status:      statusMissing,
				Detail:      fmt.Sprintf("%s %s", ep.Method, ep.Path),
			}
		case hf.SpecHash != ep.SpecHash:
			r = checkResult{
				OperationID: ep.OperationID,
				Status:      statusStale,
				Detail:      fmt.Sprintf("expected %s, got %s", ep.SpecHash, hf.SpecHash),
			}
		default:
			// spec-hash matches, check if file is in the correct tag directory
			tag := ep.Tag
			if tag == "" {
				tag = "default"
			}
			expectedRel := filepath.Join(tagToDir(tag), toKebabCase(ep.OperationID)+".hurl")
			actualRel, _ := filepath.Rel(hurlDir, hf.Path)

			if actualRel != expectedRel {
				r = checkResult{
					OperationID: ep.OperationID,
					Status:      statusMisplaced,
					Detail:      fmt.Sprintf("move %s → %s", actualRel, expectedRel),
				}
			} else {
				r = checkResult{
					OperationID: ep.OperationID,
					Status:      statusOK,
				}
			}
		}
		results = append(results, r)
		counts[r.Status]++
	}

	// Check for orphan hurl files
	for _, hf := range hurlFiles {
		if hf.OperationID == "" {
			continue
		}
		if _, found := endpointByOpID[hf.OperationID]; !found {
			results = append(results, checkResult{
				OperationID: hf.OperationID,
				Status:      statusOrphan,
				Detail:      hf.Path,
			})
			counts[statusOrphan]++
		}
	}

	// Print results
	for _, r := range results {
		switch r.Status {
		case statusOK:
			fmt.Printf("  %-10s %s\n", r.Status, r.OperationID)
		default:
			fmt.Printf("  %-10s %s  (%s)\n", r.Status, r.OperationID, r.Detail)
		}
	}

	// Summary
	total := len(endpoints)
	fmt.Printf("\n--- summary ---\n")
	fmt.Printf("total endpoints: %d\n", total)
	fmt.Printf("  OK:        %d\n", counts[statusOK])
	fmt.Printf("  MISSING:   %d\n", counts[statusMissing])
	fmt.Printf("  STALE:     %d\n", counts[statusStale])
	fmt.Printf("  MISPLACED: %d\n", counts[statusMisplaced])
	fmt.Printf("  ORPHAN:    %d\n", counts[statusOrphan])

	// Exit code: 1 if MISSING, ORPHAN, or MISPLACED
	if counts[statusMissing] > 0 || counts[statusOrphan] > 0 || counts[statusMisplaced] > 0 {
		return 1
	}
	return 0
}
