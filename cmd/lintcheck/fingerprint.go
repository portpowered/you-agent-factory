package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

type goPackage struct {
	ImportPath    string
	Dir           string
	GoFiles       []string
	CgoFiles      []string
	CFiles        []string
	CXXFiles      []string
	MFiles        []string
	HFiles        []string
	FFiles        []string
	SFiles        []string
	SwigFiles     []string
	SwigCXXFiles  []string
	SysoFiles     []string
	EmbedFiles    []string
	EmbedPatterns []string
	Module        *goModule
}

type goModule struct {
	Path    string
	Version string
	GoMod   string
	Dir     string
}

func prepareChecker(cfg config, stderr io.Writer) (string, error) {
	fingerprint, err := checkerFingerprint(cfg.goTool, cfg.packageRef, stderr)
	if err != nil {
		return "", err
	}

	cacheDir, err := filepath.Abs(cfg.cacheDir)
	if err != nil {
		return "", fmt.Errorf("resolve lint checker cache directory: %w", err)
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("create lint checker cache directory: %w", err)
	}

	baseName := cacheFileBase(cfg.packageRef, fingerprint)
	checkerPath := filepath.Join(cacheDir, baseName+executableSuffix())
	if isRegularFile(checkerPath) {
		return checkerPath, nil
	}

	temporary, err := os.CreateTemp(cacheDir, "."+baseName+"-*"+executableSuffix())
	if err != nil {
		return "", fmt.Errorf("create temporary lint checker executable: %w", err)
	}
	temporaryPath := temporary.Name()
	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporaryPath)
		return "", fmt.Errorf("close temporary lint checker executable: %w", err)
	}
	defer os.Remove(temporaryPath)

	if err := buildChecker(cfg.goTool, cfg.packageRef, temporaryPath, stderr); err != nil {
		return "", err
	}
	if err := os.Chmod(temporaryPath, 0o755); err != nil {
		return "", fmt.Errorf("make compiled lint checker executable: %w", err)
	}
	if err := os.Rename(temporaryPath, checkerPath); err != nil {
		if isRegularFile(checkerPath) {
			return checkerPath, nil
		}
		return "", fmt.Errorf("publish compiled lint checker executable: %w", err)
	}
	return checkerPath, nil
}

func buildChecker(goTool, packageRef, outputPath string, stderr io.Writer) error {
	args := []string{"build", "-o", outputPath, packageRef}
	if err := runCommand(goTool, args, io.Discard, stderr); err != nil {
		return fmt.Errorf("compile lint checker %s: %w", packageRef, err)
	}
	return nil
}

func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Size() > 0
}

func executableSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

func cacheFileBase(packageRef, fingerprint string) string {
	var name strings.Builder
	for _, character := range packageRef {
		switch {
		case character >= 'a' && character <= 'z':
			name.WriteRune(character)
		case character >= 'A' && character <= 'Z':
			name.WriteRune(character)
		case character >= '0' && character <= '9':
			name.WriteRune(character)
		default:
			name.WriteByte('-')
		}
	}
	return strings.Trim(name.String(), "-") + "-" + fingerprint
}

func checkerFingerprint(goTool, packageRef string, stderr io.Writer) (string, error) {
	packages, err := listGoPackages(goTool, packageRef, stderr)
	if err != nil {
		return "", err
	}
	environment, err := goBuildEnvironment(goTool, stderr)
	if err != nil {
		return "", err
	}

	inputFiles := make(map[string]struct{})
	for _, packageInfo := range packages {
		for _, path := range packageInfo.sourceFiles() {
			inputFiles[path] = struct{}{}
		}
		if packageInfo.Module != nil && packageInfo.Module.GoMod != "" {
			inputFiles[packageInfo.Module.GoMod] = struct{}{}
			inputFiles[filepath.Join(filepath.Dir(packageInfo.Module.GoMod), "go.sum")] = struct{}{}
		}
	}
	for _, key := range []string{"GOMOD", "GOWORK"} {
		if path := environment[key]; isHashableBuildFile(path) {
			inputFiles[path] = struct{}{}
			if key == "GOMOD" {
				inputFiles[filepath.Join(filepath.Dir(path), "go.sum")] = struct{}{}
			}
			if key == "GOWORK" {
				inputFiles[filepath.Join(filepath.Dir(path), "go.work.sum")] = struct{}{}
			}
		}
	}

	digest := sha256.New()
	writeHashField(digest, "package", packageRef)
	writeSortedEnvironment(digest, environment)
	writePackageMetadata(digest, packages)
	if err := writeInputFiles(digest, inputFiles); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func listGoPackages(goTool, packageRef string, stderr io.Writer) ([]goPackage, error) {
	output, err := runGoOutput(goTool, []string{"list", "-deps", "-json", packageRef}, stderr)
	if err != nil {
		return nil, fmt.Errorf("list lint checker build inputs: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(output))
	var packages []goPackage
	for {
		var packageInfo goPackage
		err := decoder.Decode(&packageInfo)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("decode lint checker build inputs: %w", err)
		}
		packages = append(packages, packageInfo)
	}
	if len(packages) == 0 {
		return nil, errors.New("list lint checker build inputs: go list returned no packages")
	}
	sort.Slice(packages, func(i, j int) bool {
		return packages[i].ImportPath < packages[j].ImportPath
	})
	return packages, nil
}

func goBuildEnvironment(goTool string, stderr io.Writer) (map[string]string, error) {
	keys := []string{
		"GOOS", "GOARCH", "GOAMD64", "GOARM", "GO386", "GOMIPS", "GOMIPS64",
		"GOPPC64", "GOWASM", "CGO_ENABLED", "CC", "CXX", "GOFLAGS", "GOEXPERIMENT",
		"GOVERSION", "GOMOD", "GOWORK",
	}
	args := append([]string{"env", "-json"}, keys...)
	output, err := runGoOutput(goTool, args, stderr)
	if err != nil {
		return nil, fmt.Errorf("read Go build environment: %w", err)
	}
	var environment map[string]string
	if err := json.Unmarshal(output, &environment); err != nil {
		return nil, fmt.Errorf("decode Go build environment: %w", err)
	}
	return environment, nil
}

func runGoOutput(goTool string, args []string, stderr io.Writer) ([]byte, error) {
	command := exec.Command(goTool, args...)
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = stderr
	if err := command.Run(); err != nil {
		return nil, commandFailure{err: err}
	}
	return output.Bytes(), nil
}

func (p goPackage) sourceFiles() []string {
	files := make([]string, 0)
	for _, paths := range [][]string{
		p.GoFiles, p.CgoFiles, p.CFiles, p.CXXFiles, p.MFiles, p.HFiles,
		p.FFiles, p.SFiles, p.SwigFiles, p.SwigCXXFiles, p.SysoFiles, p.EmbedFiles,
	} {
		for _, path := range paths {
			if !filepath.IsAbs(path) {
				path = filepath.Join(p.Dir, path)
			}
			files = append(files, filepath.Clean(path))
		}
	}
	sort.Strings(files)
	return files
}

func isHashableBuildFile(path string) bool {
	if path == "" || path == os.DevNull || strings.EqualFold(path, "NUL") {
		return false
	}
	return true
}

func writeHashField(digest hash.Hash, key, value string) {
	fmt.Fprintf(digest, "%s:%d:", key, len(value))
	_, _ = io.WriteString(digest, value)
}

func writeSortedEnvironment(digest hash.Hash, environment map[string]string) {
	keys := make([]string, 0, len(environment))
	for key := range environment {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		writeHashField(digest, "env-key", key)
		writeHashField(digest, "env-value", environment[key])
	}
}

func writePackageMetadata(digest hash.Hash, packages []goPackage) {
	for _, packageInfo := range packages {
		writeHashField(digest, "import-path", packageInfo.ImportPath)
		writeHashField(digest, "directory", filepath.ToSlash(packageInfo.Dir))
		for _, pattern := range packageInfo.EmbedPatterns {
			writeHashField(digest, "embed-pattern", pattern)
		}
		if packageInfo.Module != nil {
			writeHashField(digest, "module-path", packageInfo.Module.Path)
			writeHashField(digest, "module-version", packageInfo.Module.Version)
		}
	}
}

func writeInputFiles(digest hash.Hash, paths map[string]struct{}) error {
	sortedPaths := make([]string, 0, len(paths))
	for path := range paths {
		if _, err := os.Stat(path); err == nil {
			sortedPaths = append(sortedPaths, filepath.Clean(path))
			continue
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("stat lint checker build input %s: %w", path, err)
		}
	}
	sort.Strings(sortedPaths)
	for _, path := range sortedPaths {
		writeHashField(digest, "file", filepath.ToSlash(path))
		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("open lint checker build input %s: %w", path, err)
		}
		_, copyErr := io.Copy(digest, file)
		closeErr := file.Close()
		if copyErr != nil {
			return fmt.Errorf("read lint checker build input %s: %w", path, copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close lint checker build input %s: %w", path, closeErr)
		}
	}
	return nil
}
