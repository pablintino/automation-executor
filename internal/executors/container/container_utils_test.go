package container

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"os/exec"
	"strings"
	"testing"
)

// DO NOT USE the API. These utils needs to be completely agnostic and API decoupled. Using
// the cli removes the need to care about API versioning for the tests
// Do not use the logging lib. These test utils should be as agnostic of the code as possible
const testResourcesLabel = "automation-executor-test-resource"

func cleanUpAllPodmanTestResources() error {
	containerIds, err := getResourcesByLabels("container", map[string]string{testResourcesLabel: ""})
	if err != nil {
		return err
	}
	if len(containerIds) > 0 {
		opts := []string{"container", "rm", "--force"}
		opts = append(opts, containerIds...)
		if err := exec.Command("podman", opts...).Run(); err != nil {
			zap.S().Errorw("failed to remove podman containers", "error", err)
			return err
		}
	}
	return nil
}

func deleteSingleContainer(id string) (bool, error) {
	return deleteSingleResource("container", id)
}

func deleteSingleVolume(id string) (bool, error) {
	return deleteSingleResource("volume", id)
}

func deleteSingleResource(resource string, id string) (bool, error) {
	exists, err := checkResourceExists(resource, id)
	if err != nil || !exists {
		return false, err
	}
	if err := exec.Command("podman", resource, "rm", "--force").Run(); err != nil {
		zap.S().Errorw("failed to remove podman resource", "resource", resource, "id", id, "error", err)
		return false, err
	}
	return true, nil
}

func getPodmanVolumesByLabels(labels map[string]string) ([]string, error) {
	return getResourcesByLabels("volume", labels)
}

func getSinglePodmanContainerByLabels(labels map[string]string) (map[string]interface{}, error) {
	ids, err := getResourcesByLabels("container", labels)
	if err != nil {
		return nil, err
	}
	if len(ids) != 1 {
		return nil, nil
	}
	return inspectResource("container", ids[0])
}

func inspectResource(resource string, id string) (map[string]interface{}, error) {
	output, err := exec.Command("podman", resource, "inspect", id).Output()
	if err != nil {
		var exitErr *exec.ExitError
		ok := errors.As(err, &exitErr)
		if ok && exitErr != nil && strings.Contains(strings.ToLower(string(exitErr.Stderr)), "no such") {
			return nil, nil
		}
		zap.S().Errorw("failed to inspect podman resource", "resource", resource, "error", err, "id", id)
		return nil, err
	}
	jsonList := make([]map[string]interface{}, 1)
	if err := json.Unmarshal(output, &jsonList); err != nil {
		zap.S().Errorw("failed to decode podman resource inspect", "resource", resource, "error", err, "id", id)
		return nil, err
	}
	return jsonList[0], nil
}

func checkResourceExists(resource string, id string) (bool, error) {
	data, err := inspectResource(resource, id)
	if err != nil {
		return false, err
	}
	return data != nil, nil
}

func getResourcesByLabels(resource string, labels map[string]string) ([]string, error) {
	opts := []string{resource, "ls", "--quiet"}
	opts = append(opts, createLabelFilter(labels)...)
	output, err := exec.Command("podman", opts...).Output()
	if err != nil {
		zap.S().Errorw("failed to list resources", "resource", resource, "error", err)
		return nil, err
	}
	trimmedOut := strings.TrimRight(string(output), "\n")
	if len(trimmedOut) > 0 {
		return strings.Split(trimmedOut, "\n"), nil
	}
	return nil, nil
}

func createLabelFilter(labels map[string]string) []string {
	filters := make([]string, 0)
	for label, value := range labels {
		if value == "" {
			filters = append(filters, "--filter", fmt.Sprintf("label=%s", label))
		} else {
			filters = append(filters, "--filter", fmt.Sprintf("label=%s=%s", label, value))
		}
	}
	return filters
}

func assertVolumeIsMounted(t *testing.T, inspectData map[string]interface{}, name string, destination string) {
	if mounts, ok := inspectData["Mounts"].([]interface{}); ok {
		for _, mountRaw := range mounts {
			mount, ok := mountRaw.(map[string]interface{})
			require.True(t, ok)
			if mountType, ok := mount["Type"].(string); ok && mountType == "volume" {
				require.Contains(t, mount, "Name")
				mountName, ok := mount["Name"].(string)
				require.True(t, ok)
				assert.Equal(t, name, mountName)
				require.Contains(t, mount, "Name")
				mountDestination, ok := mount["Destination"].(string)
				require.True(t, ok)
				assert.Equal(t, destination, mountDestination)
				return
			}
		}
	}
	assert.Failf(t, "volume mount not found %s", name)
}
