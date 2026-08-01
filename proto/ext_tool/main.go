package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

func bailIf(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func findPrettier() (string, error) {
	// Try common prettier command names
	names := []string{"prettier", "prettier.cmd", "npx"}
	for _, name := range names {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("prettier not found in PATH, please install: npm install -g prettier")
}

func main() {
	// Command-line arguments
	inputPath := flag.String("input", "", "Path to JS file (e.g., extensionHostProcess.js)")
	outputDir := flag.String("output", "", "Output directory for proto files (default: ./cursor_proto)")
	skipFormat := flag.Bool("skip-format", false, "Skip prettier formatting")
	strict := flag.Bool("strict", true, "Fail when extraction validation detects unresolved/placeholder output")
	flag.Parse()

	// If the -input flag is missing, try to take it from the positional argument
	if *inputPath == "" && flag.NArg() > 0 {
		*inputPath = flag.Arg(0)
	}

	if *inputPath == "" {
		fmt.Fprintln(os.Stderr, "Usage: ext -input <path-to-js-file> [-output <dir>] [-skip-format]")
		fmt.Fprintln(os.Stderr, "       ext <path-to-js-file>")
		fmt.Fprintln(os.Stderr, "\nExample:")
		fmt.Fprintln(os.Stderr, "  ext -input /path/to/extensionHostProcess.js")
		fmt.Fprintln(os.Stderr, "  ext C:\\Users\\xxx\\AppData\\Local\\Programs\\cursor\\resources\\app\\out\\vs\\workbench\\api\\node\\extensionHostProcess.js")
		os.Exit(1)
	}

	// Validate the input file
	info, err := os.Stat(*inputPath)
	bailIf(err)

	if info.IsDir() {
		bailIf(fmt.Errorf("expected %s to be file, is dir", *inputPath))
	}

	// Set the output directory
	if *outputDir == "" {
		wd, err := os.Getwd()
		bailIf(err)
		*outputDir = filepath.Join(wd, "cursor_proto")
	}

	// Copy to a temp file (without modifying the original)
	fmt.Println("Copying source file to temp directory...")
	originalFile, err := os.Open(*inputPath)
	bailIf(err)

	tempFile, err := os.CreateTemp(os.TempDir(), "cursor-source-*.js")
	bailIf(err)
	tempFileName := tempFile.Name()

	_, err = io.Copy(tempFile, originalFile)
	bailIf(err)

	bailIf(originalFile.Close())
	bailIf(tempFile.Close())

	fmt.Printf("Temp file: %s\n", tempFileName)

	// Format the temp file
	if !*skipFormat {
		prettierBin, err := findPrettier()
		if err != nil {
			fmt.Printf("Warning: %v\n", err)
			fmt.Println("Skipping formatting, extraction may be less accurate...")
		} else {
			fmt.Println("Formatting file (this may take a while)...")
			var prettierCmd *exec.Cmd
			if filepath.Base(prettierBin) == "npx" {
				prettierCmd = exec.Command(prettierBin, "prettier", "--write", tempFileName)
			} else {
				prettierCmd = exec.Command(prettierBin, "--write", tempFileName)
			}
			out, err := prettierCmd.CombinedOutput()
			if err != nil {
				fmt.Printf("Prettier output: %s\n", string(out))
				fmt.Println("Warning: formatting failed, continuing anyway...")
			} else {
				fmt.Println("Formatting complete")
			}
		}
	} else {
		fmt.Println("Skipping formatting (--skip-format)")
	}

	// Run the extractor
	fmt.Println("Extracting Proto definitions...")
	SetStrictMode(*strict)
	ExtractProtos(tempFileName, *outputDir)

	// Clean up the temp file
	os.Remove(tempFileName)

	fmt.Printf("\nOutput directory: %s\n", *outputDir)
}
