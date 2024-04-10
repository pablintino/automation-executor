package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/pablintino/automation-executor/internal/config"
	"github.com/pablintino/automation-executor/logging"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

func TestArchiveScanner(t *testing.T) {
	logging.Initialize(false)
	defer logging.Release()

	data := []struct {
		path     string
		patterns []string
		results  map[string]*ArtifactEntryScanResult
		error    error
	}{
		{"testdata/working-file-1.tar.gz", []string{"*.yml"}, map[string]*ArtifactEntryScanResult{
			"yaml-file-1.yml": {Result: map[string]interface{}{
				"variable":  "test-value",
				"variable2": []string{"test-value"},
			}, Unrecognized: false, Format: ArchiveScanFormatYAML},
			"yaml-file-2.yml": {Result: map[string]interface{}{
				"variable3": "test-value-3",
				"variable4": map[string]interface{}{"test-value": "test-value-4"},
			}, Unrecognized: false, Format: ArchiveScanFormatYAML},
		}, nil},
		{"testdata/working-file-1.tar.gz", []string{"*.json"}, map[string]*ArtifactEntryScanResult{
			"json-file-1.json": {Result: map[string]interface{}{
				"test":  "test-value-1",
				"test2": []string{"test-value-2"},
			}, Unrecognized: false, Format: ArchiveScanFormatJson},
		}, nil},
		{"testdata/working-file-1.tar.gz", []string{"*.json", "*.yml"}, map[string]*ArtifactEntryScanResult{
			"yaml-file-1.yml": {Result: map[string]interface{}{
				"variable":  "test-value",
				"variable2": []string{"test-value"},
			}, Unrecognized: false, Format: ArchiveScanFormatYAML},
			"yaml-file-2.yml": {Result: map[string]interface{}{
				"variable3": "test-value-3",
				"variable4": map[string]interface{}{"test-value": "test-value-4"},
			}, Unrecognized: false, Format: ArchiveScanFormatYAML},
			"json-file-1.json": {Result: map[string]interface{}{
				"test":  "test-value-1",
				"test2": []string{"test-value-2"},
			}, Unrecognized: false, Format: ArchiveScanFormatJson},
		}, nil},
		{"testdata/working-file-1.tar.gz", []string{"*.ble"}, map[string]*ArtifactEntryScanResult{}, nil},
		{"testdata/faulty-files-1.tar.gz", []string{"*.yml", "*.json"}, map[string]*ArtifactEntryScanResult{
			"yaml-file-1.yml": {Result: map[string]interface{}{}, Unrecognized: true, Format: ""},
			"json-file-1.json": {
				Result: map[string]interface{}{}, Unrecognized: false, Format: ArchiveScanFormatJson,
				Error: errors.New("json syntax error: invalid character 'i' looking for beginning of object key string: 26139377")},
		}, nil},
		{"testdata/wrong-archive-file-1.zip", []string{}, map[string]*ArtifactEntryScanResult{},
			errors.New("requires gzip-compressed body: gzip: invalid header")},
		{"testdata/wrong-archive-file-2.zip.gz", []string{}, map[string]*ArtifactEntryScanResult{},
			errors.New("tar error: unexpected EOF")},
	}

	for _, tt := range data {
		t.Run(fmt.Sprintf("scan %s", tt.path), func(t *testing.T) {
			testingFile, err := os.Open(tt.path)
			require.NoError(t, err)
			defer testingFile.Close()

			scanConfig, err := NewScanConfig(tt.patterns)
			require.NoError(t, err)
			scanner := NewArtifactsScanner(&config.ArtifactsConfig{LoadSize: 4096})
			result, err := scanner.ScanArchive(testingFile, scanConfig)
			if tt.error == nil {
				require.NoError(t, err)
				require.Equal(t, len(tt.results), len(result.Entries))
				for name, entry := range tt.results {
					resultData, present := result.Entries[name]
					require.True(t, present)
					require.Equal(t, entry.Format, resultData.Format)
					require.Equal(t, entry.Unrecognized, resultData.Unrecognized)
					require.Equal(t, entry.Error, resultData.Error)
					if entry.Error != nil {
						require.True(t, result.Failed)
					}
					expectedStr, err := json.Marshal(entry.Result)
					require.NoError(t, err)
					outputStr, err := json.Marshal(resultData.Result)
					require.NoError(t, err)
					require.Equal(t, string(expectedStr), string(outputStr))
				}
			} else {
				require.Equal(t, tt.error, err)
			}
		})
	}
}
