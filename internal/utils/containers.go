package utils

import "strings"

const (
	dockerDefaultDomain       = "docker.io"
	dockerLegacyDefaultDomain = "index.docker.io"
)

func ExtractRegistryNameFromTag(tag string) string {
	var domain string
	i := strings.IndexRune(tag, '/')
	if i == -1 || (!strings.ContainsAny(tag[:i], ".:") && tag[:i] != "localhost") {
		domain = dockerDefaultDomain
	} else {
		domain = tag[:i]
	}
	if domain == dockerLegacyDefaultDomain {
		domain = dockerDefaultDomain
	}
	return domain
}
