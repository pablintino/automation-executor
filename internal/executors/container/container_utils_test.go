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

func createVolume(name string, labels map[string]string) (string, error) {
	opts := []string{"volume", "create", name}
	for label, value := range labels {
		if value != "" {
			opts = append(opts, "--label", fmt.Sprintf("%s=%s", label, value))
		} else {
			opts = append(opts, "--label", name)
		}
	}
	if err := exec.Command("podman", opts...).Run(); err != nil {
		zap.S().Errorw("failed to create podman volume", "name", name, "error", err)
		return "", err
	}
	return name, nil
}

func createContainer(name string, image string, cmd []string, labels map[string]string, volumes map[string]string) (string, error) {
	opts := []string{"container", "run", "-d", "--name", name}
	for vol, dest := range volumes {
		opts = append(opts, "--volume", fmt.Sprintf("%s:%s", vol, dest))
	}
	for label, value := range labels {
		if value != "" {
			opts = append(opts, "--label", fmt.Sprintf("%s=%s", label, value))
		} else {
			opts = append(opts, "--label", name)
		}
	}
	opts = append(opts, image)
	opts = append(opts, cmd...)
	out, err := exec.Command("podman", opts...).Output()
	if err != nil {
		zap.S().Errorw("failed to create podman container", "name", name, "error", err)
		return "", err
	}
	return strings.TrimRight(string(out), "\n"), nil
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

func inspectContainer(id string) (map[string]interface{}, error) {
	return inspectResource("container", id)
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

func assertContainerLabelExists(t *testing.T, containerData map[string]interface{}, name string, value interface{}) {
	if configRaw, ok := containerData["Config"].(map[string]interface{}); ok {
		if labelsRaw, ok := configRaw["Labels"].(map[string]interface{}); ok {
			if labelValue, ok := labelsRaw[name]; ok {
				assert.Equal(t, value, labelValue)
				return
			}
		}
	}
	assert.Failf(t, "label not found %s", name)
}
