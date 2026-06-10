package engine

import (
	"path/filepath"
	"strings"
)

func globMatch(pattern, value string) bool {
	pattern = filepath.ToSlash(pattern)
	value = filepath.ToSlash(value)
	if pattern == value {
		return true
	}
	if strings.HasPrefix(pattern, "**/") {
		suffix := strings.TrimPrefix(pattern, "**/")
		if ok, _ := filepath.Match(suffix, value); ok {
			return true
		}
		if strings.HasSuffix(value, "/"+strings.TrimSuffix(suffix, "/**")) {
			return true
		}
		return strings.Contains(value, "/"+strings.TrimSuffix(suffix, "/**")+"/")
	}
	if strings.HasSuffix(pattern, "/**") {
		prefix := strings.TrimSuffix(pattern, "/**")
		return value == prefix || strings.HasPrefix(value, prefix+"/")
	}
	ok, _ := filepath.Match(pattern, value)
	return ok
}
