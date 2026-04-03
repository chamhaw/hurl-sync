package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// HurlFile represents a parsed hurl file with its header metadata.
type HurlFile struct {
	Path        string
	OperationID string
	Tag         string
	SpecHash    string
	RequestHash string
}

func scanHurlFiles(dir string) ([]HurlFile, error) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil, nil
	}

	var files []HurlFile

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip flows/ subdirectory
		rel, _ := filepath.Rel(dir, path)
		if info.IsDir() && rel == "flows" {
			return filepath.SkipDir
		}

		if info.IsDir() || !strings.HasSuffix(info.Name(), ".hurl") {
			return nil
		}

		hf, err := parseHurlHeader(path)
		if err != nil {
			return nil // skip unparseable files
		}
		files = append(files, hf)
		return nil
	})

	return files, err
}

func parseHurlHeader(path string) (HurlFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return HurlFile{}, err
	}
	defer f.Close()

	hf := HurlFile{Path: path}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "#") {
			break
		}
		line = strings.TrimPrefix(line, "#")
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "operationId:") {
			hf.OperationID = strings.TrimSpace(strings.TrimPrefix(line, "operationId:"))
		} else if strings.HasPrefix(line, "tag:") {
			hf.Tag = strings.TrimSpace(strings.TrimPrefix(line, "tag:"))
		} else if strings.HasPrefix(line, "spec-hash:") {
			hf.SpecHash = strings.TrimSpace(strings.TrimPrefix(line, "spec-hash:"))
		} else if strings.HasPrefix(line, "request-hash:") {
			hf.RequestHash = strings.TrimSpace(strings.TrimPrefix(line, "request-hash:"))
		}
	}

	return hf, scanner.Err()
}

// toKebabCase converts camelCase/PascalCase to kebab-case.
func toKebabCase(s string) string {
	var result []rune
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 && len(result) > 0 && result[len(result)-1] != '-' {
				result = append(result, '-')
			}
			result = append(result, r+32) // toLower
		} else if r == ' ' {
			if len(result) > 0 && result[len(result)-1] != '-' {
				result = append(result, '-')
			}
		} else {
			result = append(result, r)
		}
	}
	return string(result)
}

// tagToDir converts a swagger tag to a directory name (kebab-case).
func tagToDir(tag string) string {
	return toKebabCase(tag)
}
