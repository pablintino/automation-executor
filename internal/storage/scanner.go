package storage

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/pablintino/automation-executor/internal/config"
	"io"
	"path/filepath"
	"time"

	"github.com/gabriel-vasile/mimetype"
	"github.com/gobwas/glob"
	"github.com/pablintino/automation-executor/logging"
	"gopkg.in/yaml.v3"
)

type ScanConfig struct {
	scanGlobs []glob.Glob
}

func NewScanConfig(scanPattern []string) (*ScanConfig, error) {
	scanConfig := &ScanConfig{scanGlobs: make([]glob.Glob, len(scanPattern))}
	for index, pattern := range scanPattern {
		globPattern, err := glob.Compile(pattern)
		if err != nil {
			return nil, err
		}
		scanConfig.scanGlobs[index] = globPattern
	}
	return scanConfig, nil
}

func (c *ScanConfig) isConfigured(path string) bool {
	for _, glob := range c.scanGlobs {
		if glob.Match(path) {
			return true
		}
	}
	return false
}

type ArtifactsScannerImpl struct {
	blockLoadSize uint32
}

func NewArtifactsScanner(config *config.StorageConfig) *ArtifactsScannerImpl {
	mimetype.SetLimit(config.LoadSize)
	return &ArtifactsScannerImpl{
		blockLoadSize: config.LoadSize,
	}
}

type ArtifactsScanner interface {
	ScanArchive(r io.Reader, config *ScanConfig) (*ArtifactScanResult, error)
}

const ArchiveScanFormatJson = "json"
const ArchiveScanFormatYAML = "yaml"

type ArtifactEntryScanResult struct {
	Result       map[string]interface{}
	Format       string
	Unrecognized bool
	Error        error
}

type ArtifactScanResult struct {
	Entries map[string]*ArtifactEntryScanResult
	Failed  bool
}

func (e *ArtifactsScannerImpl) ScanArchive(r io.Reader, scanConfig *ScanConfig) (result *ArtifactScanResult, err error) {
	t0 := time.Now()
	nFiles := 0
	defer func() {
		td := time.Since(t0)
		if err == nil {
			logging.Logger.Debugw("tarball scan finished", "file-count", nFiles, "duration", td)
		} else {
			logging.Logger.Errorw("error scanning archive tarball", "file-count", nFiles, "duration", td, "error", err)
		}
	}()
	zr, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("requires gzip-compressed body: %v", err)
	}
	tr := tar.NewReader(zr)
	result = &ArtifactScanResult{Entries: make(map[string]*ArtifactEntryScanResult), Failed: false}
	for {
		f, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("tar error: %v", err)
		}
		if f.Typeflag != tar.TypeReg {
			continue
		}
		path := filepath.FromSlash(f.Name)
		if !scanConfig.isConfigured(path) {
			logging.Logger.Debugw("skipping non-matching archive entry", "path", path)
			continue
		}

		entry := &ArtifactEntryScanResult{Result: make(map[string]interface{})}
		err = e.scanEntry(tr, entry)
		if err != nil {
			entry.Error = err
			result.Failed = true
			logging.Logger.Debugw("error scanning archive entry", "name", path, "error", err)
		}
		result.Entries[path] = entry
		nFiles++

	}
	return result, nil
}

func (e *ArtifactsScannerImpl) scanEntry(reader io.Reader, entry *ArtifactEntryScanResult) error {
	var isJson bool
	var isText bool
	var memBuf bytes.Buffer
	buffer := make([]byte, e.blockLoadSize)
	mimeComputed := false
	for {
		n, err := reader.Read(buffer)
		if err != nil && err != io.EOF {
			return err
		}
		if n == 0 {
			break
		}

		if nw, wErr := memBuf.Write(buffer[:n]); wErr != nil || nw != n {
			return errors.New("error writing memory buffer")
		}
		if !mimeComputed {
			mimeComputed = true
			mime := mimetype.Detect(buffer[:n])
			isJson = mime.Is("application/json")
			isText = mime.Is("text/plain")
			if !isText && !isJson {
				entry.Unrecognized = true
				return nil
			}
		}
	}

	if isJson {
		entry.Format = ArchiveScanFormatJson
		entry.Unrecognized = false
		if err := json.Unmarshal(memBuf.Bytes(), &entry.Result); err != nil {
			var sErr *json.SyntaxError
			if !errors.As(err, &sErr) {
				return err
			}
			// To avoid losing the error details upwards
			// create a new error with
			return fmt.Errorf("json syntax error: %s: %v", sErr.Error(), sErr.Offset)
		}
	} else if isText {
		if err := yaml.Unmarshal(memBuf.Bytes(), &entry.Result); err != nil {
			// For yaml, as we are not sure the context is text
			// just ignore errors and return nil
			entry.Unrecognized = true
			return nil
		}
		entry.Format = ArchiveScanFormatYAML
		entry.Unrecognized = false
	}

	return nil
}
