package controller

import (
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"
)

// mustParseQuantity parses a resource quantity string and panics on error.
// Safe to use for hardcoded values in controller logic.
func mustParseQuantity(s string) resource.Quantity {
	q, err := resource.ParseQuantity(s)
	if err != nil {
		panic(err)
	}
	return q
}

// isValidBranchName validates Git branch names.
// Git branch names must not contain: spaces, ~, ^, :, ?, *, [, \, .., @{, //
// and must not start or end with a slash or dot.
func isValidBranchName(branch string) bool {
	if branch == "" {
		return false
	}

	if strings.HasPrefix(branch, "/") || strings.HasSuffix(branch, "/") {
		return false
	}
	if strings.HasPrefix(branch, ".") || strings.HasSuffix(branch, ".") {
		return false
	}

	invalidPatterns := []string{" ", "~", "^", ":", "?", "*", "[", "\\", "..", "@{", "//"}
	for _, pattern := range invalidPatterns {
		if strings.Contains(branch, pattern) {
			return false
		}
	}

	validBranchRegex := regexp.MustCompile(`^[a-zA-Z0-9._/-]+$`)
	return validBranchRegex.MatchString(branch)
}
