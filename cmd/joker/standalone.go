package main

// standalone.go — standalone binary support.
//
// Produces self-contained executables by appending Clojure source to a copy
// of the joker binary. At startup, the binary checks for an embedded payload
// and auto-executes it.
//
// Format:
//   [joker binary][source bytes][8-byte LE source length][4-byte magic "JKRB"]
//
// Usage:
//   joker compile <source.clj> -o <output>
//   ./output [args...]

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

const standaloneMagic = "JKRB"
const standaloneFooterSize = 12 // 8 bytes length + 4 bytes magic

// checkEmbeddedSource checks if the current executable has an embedded
// Clojure source payload. Returns the source string and true if found.
func checkEmbeddedSource() (string, bool) {
	exe, err := os.Executable()
	if err != nil {
		return "", false
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return "", false
	}
	f, err := os.Open(exe)
	if err != nil {
		return "", false
	}
	defer f.Close()

	// Read the footer
	fi, err := f.Stat()
	if err != nil || fi.Size() < int64(standaloneFooterSize) {
		return "", false
	}

	footer := make([]byte, standaloneFooterSize)
	_, err = f.ReadAt(footer, fi.Size()-int64(standaloneFooterSize))
	if err != nil {
		return "", false
	}

	// Check magic
	if string(footer[8:12]) != standaloneMagic {
		return "", false
	}

	// Read source length
	srcLen := binary.LittleEndian.Uint64(footer[0:8])
	if srcLen == 0 || int64(srcLen) > fi.Size()-int64(standaloneFooterSize) {
		return "", false
	}

	// Read source
	src := make([]byte, srcLen)
	_, err = f.ReadAt(src, fi.Size()-int64(standaloneFooterSize)-int64(srcLen))
	if err != nil {
		return "", false
	}

	return string(src), true
}

// compileStandalone produces a standalone binary from a source file.
func compileStandalone(sourceFile string, outputFile string) error {
	// Read source
	src, err := os.ReadFile(sourceFile)
	if err != nil {
		return fmt.Errorf("cannot read source file: %w", err)
	}
	if len(src) == 0 {
		return fmt.Errorf("source file is empty")
	}

	// Find our own executable
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot find own executable: %w", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return fmt.Errorf("cannot resolve executable path: %w", err)
	}

	// Read the runtime binary (strip any existing embedded source)
	runtimeBin, err := os.ReadFile(exe)
	if err != nil {
		return fmt.Errorf("cannot read runtime binary: %w", err)
	}

	// Strip existing payload if present
	runtimeBin = stripEmbeddedPayload(runtimeBin)

	// Create output
	out, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("cannot create output file: %w", err)
	}
	defer out.Close()

	// Write runtime binary
	if _, err := out.Write(runtimeBin); err != nil {
		return fmt.Errorf("write runtime: %w", err)
	}

	// Write source
	if _, err := out.Write(src); err != nil {
		return fmt.Errorf("write source: %w", err)
	}

	// Write footer: [8-byte LE source length][4-byte magic]
	footer := make([]byte, standaloneFooterSize)
	binary.LittleEndian.PutUint64(footer[0:8], uint64(len(src)))
	copy(footer[8:12], standaloneMagic)
	if _, err := out.Write(footer); err != nil {
		return fmt.Errorf("write footer: %w", err)
	}

	// Make executable on Unix
	if runtime.GOOS != "windows" {
		if err := out.Chmod(0755); err != nil {
			return fmt.Errorf("chmod: %w", err)
		}
	}

	return nil
}

// stripEmbeddedPayload removes an existing JKRB payload from a binary.
func stripEmbeddedPayload(bin []byte) []byte {
	if len(bin) < standaloneFooterSize {
		return bin
	}
	footer := bin[len(bin)-standaloneFooterSize:]
	if string(footer[8:12]) != standaloneMagic {
		return bin
	}
	srcLen := binary.LittleEndian.Uint64(footer[0:8])
	trimSize := int(srcLen) + standaloneFooterSize
	if trimSize > len(bin) {
		return bin
	}
	return bin[:len(bin)-trimSize]
}

// copyFile copies src to dst, preserving permissions.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}

	info, err := in.Stat()
	if err != nil {
		return err
	}
	return out.Chmod(info.Mode())
}
