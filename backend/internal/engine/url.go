package engine

import "strings"

func BuildURL(baseURL, path string) string {
	baseURL = strings.TrimRight(baseURL, "/")
	path = ensureLeadingSlash(path)
	if strings.HasSuffix(strings.ToLower(baseURL), "/v1") && strings.HasPrefix(strings.ToLower(path), "/v1/") {
		baseURL = strings.TrimSuffix(baseURL, baseURL[len(baseURL)-3:])
	}
	return baseURL + path
}

func ensureLeadingSlash(path string) string {
	if path == "" {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		return "/" + path
	}
	return path
}
