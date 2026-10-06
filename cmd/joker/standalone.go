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
	"encoding/json"
	"fmt"
	corewasm "github.com/rcarmo/go-joker/v42/core/wasm"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

const standaloneMagic = "JKRB"
const standaloneFooterSize = 12 // 8 bytes length + 4 bytes magic

// checkEmbeddedSource checks if the current executable has an embedded
// Clojure source payload. Returns the source string and true if found.
func checkEmbeddedSource() (string, bool) { source, _, ok := checkEmbeddedProgram(); return source, ok }

const standaloneMetadataPrefix = "\x00JKR-META-1\x00"

type standaloneMetadata struct {
	Source string `json:"source"`
	Engine string `json:"wasmEngine"`
}

func decodeStandalonePayload(data []byte) (string, string, bool) {
	if len(data) >= len(standaloneMetadataPrefix) && string(data[:len(standaloneMetadataPrefix)]) == standaloneMetadataPrefix {
		var p standaloneMetadata
		if json.Unmarshal(data[len(standaloneMetadataPrefix):], &p) != nil || p.Source == "" {
			return "", "", false
		}
		return p.Source, p.Engine, true
	}
	return string(data), "", true
}
func checkEmbeddedProgram() (srcText string, engine string, ok bool) {
	exe, err := os.Executable()
	if err != nil {
		return "", "", false
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return "", "", false
	}
	f, err := os.Open(exe)
	if err != nil {
		return "", "", false
	}
	defer func() {
		if err := f.Close(); err != nil {
			srcText = ""
			ok = false
		}
	}()

	// Read the footer
	fi, err := f.Stat()
	if err != nil || fi.Size() < int64(standaloneFooterSize) {
		return "", "", false
	}

	footer := make([]byte, standaloneFooterSize)
	_, err = f.ReadAt(footer, fi.Size()-int64(standaloneFooterSize))
	if err != nil {
		return "", "", false
	}

	// Check magic
	if string(footer[8:12]) != standaloneMagic {
		return "", "", false
	}

	// Read source length
	srcLen := binary.LittleEndian.Uint64(footer[0:8])
	if srcLen == 0 || srcLen > uint64(fi.Size()-int64(standaloneFooterSize)) {
		return "", "", false
	}

	// Read source
	src := make([]byte, srcLen)
	_, err = f.ReadAt(src, fi.Size()-int64(standaloneFooterSize)-int64(srcLen))
	if err != nil {
		return "", "", false
	}

	return decodeStandalonePayload(src)
}

func writeStandaloneChunk(w io.Writer, label string, data []byte) error {
	n, err := w.Write(data)
	if err != nil {
		return fmt.Errorf("write %s: %w", label, err)
	}
	if n != len(data) {
		return fmt.Errorf("write %s: short write: %d of %d bytes", label, n, len(data))
	}
	return nil
}

// compileStandalone produces a standalone binary from a source file.
func compileStandalone(sourceFile, outputFile string) error {
	return compileStandaloneEngine(sourceFile, outputFile, "")
}
func compileStandaloneEngine(sourceFile, outputFile, engine string) (err error) {
	// Read source
	src, err := os.ReadFile(sourceFile)
	if err != nil {
		return fmt.Errorf("cannot read source file: %w", err)
	}
	if len(src) == 0 {
		return fmt.Errorf("source file is empty")
	}
	if engine != "" {
		var e error
		engine, e = corewasm.NormalizeEngine(engine)
		if e != nil {
			return e
		}
		data, e := json.Marshal(standaloneMetadata{Source: string(src), Engine: engine})
		if e != nil {
			return e
		}
		src = append([]byte(standaloneMetadataPrefix), data...)
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

	if sourceInfo, e := os.Stat(sourceFile); e == nil {
		if outputInfo, e := os.Stat(outputFile); e == nil && os.SameFile(sourceInfo, outputInfo) {
			return fmt.Errorf("output must not overwrite the source file")
		}
	}
	if executableInfo, e := os.Stat(exe); e == nil {
		if outputInfo, e := os.Stat(outputFile); e == nil && os.SameFile(executableInfo, outputInfo) {
			return fmt.Errorf("output must not overwrite the running executable")
		}
	}
	// Read the runtime binary (strip any existing embedded source)
	runtimeBin, err := os.ReadFile(exe)
	if err != nil {
		return fmt.Errorf("cannot read runtime binary: %w", err)
	}

	// Strip existing payload if present
	runtimeBin = stripEmbeddedPayload(runtimeBin)

	// Create output
	out, err := os.CreateTemp(filepath.Dir(outputFile), ".joker-compile-*")
	if err != nil {
		return fmt.Errorf("cannot create output file: %w", err)
	}
	tempPath := out.Name()
	closed := false
	defer func() {
		if !closed {
			if closeErr := out.Close(); err == nil && closeErr != nil {
				err = closeErr
			}
		}
		if removeErr := os.Remove(tempPath); removeErr != nil && !os.IsNotExist(removeErr) && err == nil {
			err = removeErr
		}
	}()

	// Write runtime binary
	if err := writeStandaloneChunk(out, "runtime", runtimeBin); err != nil {
		return err
	}

	// Write source
	if err := writeStandaloneChunk(out, "source", src); err != nil {
		return err
	}

	// Write footer: [8-byte LE source length][4-byte magic]
	footer := make([]byte, standaloneFooterSize)
	binary.LittleEndian.PutUint64(footer[0:8], uint64(len(src)))
	copy(footer[8:12], standaloneMagic)
	if err := writeStandaloneChunk(out, "footer", footer); err != nil {
		return err
	}

	// Make executable on Unix
	if runtime.GOOS != "windows" {
		if err := out.Chmod(0755); err != nil {
			return fmt.Errorf("chmod: %w", err)
		}
	}

	closeErr := out.Close()
	closed = true
	if closeErr != nil {
		return fmt.Errorf("close executable: %w", closeErr)
	}
	if err := os.Rename(tempPath, outputFile); err != nil {
		return fmt.Errorf("replace executable: %w", err)
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
	if srcLen > uint64(len(bin)-standaloneFooterSize) {
		return bin
	}
	trimSize := int(srcLen) + standaloneFooterSize
	return bin[:len(bin)-trimSize]
}

// copyFile copies src to dst, preserving permissions.
func copyFile(src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := in.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := out.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}

	info, err := in.Stat()
	if err != nil {
		return err
	}
	return out.Chmod(info.Mode())
}
