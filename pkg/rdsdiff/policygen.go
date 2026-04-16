// SPDX-License-Identifier:Apache-2.0

package rdsdiff

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// RunPolicyGen copies source-crs into CRSPath under configRoot, runs the PolicyGenerator binary
// for each SNO policy file, and writes generated YAMLs to generatedDir.
// configRoot is the effective configuration root (e.g. .../telco-ran/configuration).
// policyGeneratorPath is the path to the PolicyGenerator binary.
func RunPolicyGen(configRoot, policyGeneratorPath, generatedDir string) error {
	configRoot = filepath.Clean(configRoot)
	generatedDir = filepath.Clean(generatedDir)
	sourceCRS := filepath.Join(configRoot, SourceCRSPath)
	crsPathDir := filepath.Join(configRoot, CRSPath)
	if err := os.MkdirAll(generatedDir, 0o750); err != nil {
		return fmt.Errorf("mkdir generated dir: %w", err)
	}
	// Copy source-crs into CRSPath so PolicyGenerator finds it
	destSourceCRS := filepath.Join(crsPathDir, "source-crs")
	if err := os.RemoveAll(destSourceCRS); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove dest source-crs: %w", err)
	}
	if err := copyDir(sourceCRS, destSourceCRS); err != nil {
		return fmt.Errorf("copy source-crs to %s: %w", destSourceCRS, err)
	}
	for _, policyFile := range ListOfCRsForSNO {
		policyPath := filepath.Join(crsPathDir, policyFile)
		if _, err := os.Stat(policyPath); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("stat %s: %w", policyPath, err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		cmd := exec.CommandContext(ctx, policyGeneratorPath, policyPath) // #nosec G204 -- paths from config root
		cmd.Dir = crsPathDir
		out, err := cmd.CombinedOutput()
		cancel()
		if err != nil {
			return fmt.Errorf("policyGenerator %s: %w\n%s", policyFile, err, string(out))
		}
		base := strings.TrimSuffix(policyFile, filepath.Ext(policyFile))
		outPath := filepath.Join(generatedDir, base+"-generated.yaml")
		if err := writeCleanFile(outPath, out); err != nil {
			return fmt.Errorf("write PolicyGenerator output %s: %w", outPath, err)
		}
	}
	return nil
}

func copyDir(src, dst string) error {
	entries, err := readCleanDir(src)
	if err != nil {
		return fmt.Errorf("read dir %s: %w", src, err)
	}
	if err := os.MkdirAll(dst, 0o750); err != nil {
		return fmt.Errorf("mkdir %s: %w", dst, err)
	}
	for _, e := range entries {
		srcPath, err := safeJoinPath(src, e.Name())
		if err != nil {
			return fmt.Errorf("unsafe entry in %s: %w", src, err)
		}
		dstPath, err := safeJoinPath(dst, e.Name())
		if err != nil {
			return fmt.Errorf("unsafe entry in %s: %w", dst, err)
		}
		if e.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
			continue
		}
		data, err := readCleanFile(srcPath)
		if err != nil {
			return fmt.Errorf("read %s: %w", srcPath, err)
		}
		if err := writeCleanFile(dstPath, data); err != nil {
			return fmt.Errorf("write %s: %w", dstPath, err)
		}
	}
	return nil
}
