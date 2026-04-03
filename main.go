package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "check":
		exitCode := runCheck(os.Args[2:])
		os.Exit(exitCode)
	case "sync":
		exitCode := runSync(os.Args[2:])
		os.Exit(exitCode)
	case "--help", "-h", "help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `hurl-sync — keep hurl test files in sync with swagger spec

Usage:
  hurl-sync check    --swagger <path> --dir <path>
  hurl-sync sync     --swagger <path> --dir <path> [--dry-run]

Commands:
  check      Compare swagger endpoints against hurl files and report coverage
  sync       Auto-fix MISSING/ORPHAN/STALE/MISPLACED hurl files
`)
}

func runCheck(args []string) int {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	swaggerPath := fs.String("swagger", "", "path to swagger.json")
	hurlDir := fs.String("dir", "", "path to hurl files directory")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	if *swaggerPath == "" || *hurlDir == "" {
		fmt.Fprintln(os.Stderr, "error: --swagger and --dir are required")
		fs.Usage()
		return 1
	}

	spec, err := parseSwagger(*swaggerPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing swagger: %v\n", err)
		return 1
	}

	hurlFiles, err := scanHurlFiles(*hurlDir)
	if err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "error scanning hurl files: %v\n", err)
		return 1
	}

	return executeCheck(spec, hurlFiles, *hurlDir)
}

func runSync(args []string) int {
	fs := flag.NewFlagSet("sync", flag.ExitOnError)
	swaggerPath := fs.String("swagger", "", "path to swagger.json")
	hurlDir := fs.String("dir", "", "path to hurl files directory")
	dryRun := fs.Bool("dry-run", false, "preview changes without writing")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	if *swaggerPath == "" || *hurlDir == "" {
		fmt.Fprintln(os.Stderr, "error: --swagger and --dir are required")
		fs.Usage()
		return 1
	}

	spec, err := parseSwagger(*swaggerPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing swagger: %v\n", err)
		return 1
	}

	hurlFiles, err := scanHurlFiles(*hurlDir)
	if err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "error scanning hurl files: %v\n", err)
		return 1
	}

	return executeSync(spec, hurlFiles, *hurlDir, *dryRun)
}
