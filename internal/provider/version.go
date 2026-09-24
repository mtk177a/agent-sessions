package provider

import "strings"

type semver struct {
	core       [3]string
	prerelease []string
}

// SemverAtLeast reports whether value and floor are valid semantic versions
// and value is greater than or equal to floor. Build metadata is ignored.
func SemverAtLeast(value, floor string) bool {
	parsedValue, ok := parseSemver(value)
	if !ok {
		return false
	}
	parsedFloor, ok := parseSemver(floor)
	return ok && compareSemver(parsedValue, parsedFloor) >= 0
}

func parseSemver(value string) (semver, bool) {
	if value == "" || strings.HasPrefix(value, "v") {
		return semver{}, false
	}
	main, build, hasBuild := strings.Cut(value, "+")
	if hasBuild && !validIdentifiers(build, false) {
		return semver{}, false
	}
	coreText, prerelease, hasPrerelease := strings.Cut(main, "-")
	parts := strings.Split(coreText, ".")
	if len(parts) != 3 {
		return semver{}, false
	}
	var result semver
	for i, part := range parts {
		if !validNumericIdentifier(part) {
			return semver{}, false
		}
		result.core[i] = part
	}
	if hasPrerelease {
		if !validIdentifiers(prerelease, true) {
			return semver{}, false
		}
		result.prerelease = strings.Split(prerelease, ".")
	}
	return result, true
}

func validNumericIdentifier(value string) bool {
	if value == "" || len(value) > 1 && value[0] == '0' {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func validIdentifiers(value string, rejectNumericLeadingZero bool) bool {
	if value == "" {
		return false
	}
	for _, part := range strings.Split(value, ".") {
		if part == "" {
			return false
		}
		numeric := true
		for _, char := range part {
			if char < '0' || char > '9' {
				numeric = false
			}
			if char != '-' && (char < '0' || char > '9') && (char < 'A' || char > 'Z') && (char < 'a' || char > 'z') {
				return false
			}
		}
		if rejectNumericLeadingZero && numeric && len(part) > 1 && part[0] == '0' {
			return false
		}
	}
	return true
}

func compareSemver(left, right semver) int {
	for i := range left.core {
		comparison := compareNumericIdentifier(left.core[i], right.core[i])
		if comparison < 0 {
			return -1
		}
		if comparison > 0 {
			return 1
		}
	}
	if len(left.prerelease) == 0 || len(right.prerelease) == 0 {
		switch {
		case len(left.prerelease) == len(right.prerelease):
			return 0
		case len(left.prerelease) == 0:
			return 1
		default:
			return -1
		}
	}
	for i := 0; i < len(left.prerelease) && i < len(right.prerelease); i++ {
		leftNumeric := isNumericIdentifier(left.prerelease[i])
		rightNumeric := isNumericIdentifier(right.prerelease[i])
		switch {
		case leftNumeric && rightNumeric:
			if comparison := compareNumericIdentifier(left.prerelease[i], right.prerelease[i]); comparison != 0 {
				return comparison
			}
		case leftNumeric && !rightNumeric:
			return -1
		case !leftNumeric && rightNumeric:
			return 1
		case !leftNumeric && !rightNumeric && left.prerelease[i] < right.prerelease[i]:
			return -1
		case !leftNumeric && !rightNumeric && left.prerelease[i] > right.prerelease[i]:
			return 1
		}
	}
	switch {
	case len(left.prerelease) < len(right.prerelease):
		return -1
	case len(left.prerelease) > len(right.prerelease):
		return 1
	default:
		return 0
	}
}

func isNumericIdentifier(value string) bool {
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return value != ""
}

func compareNumericIdentifier(left, right string) int {
	if len(left) < len(right) {
		return -1
	}
	if len(left) > len(right) {
		return 1
	}
	return strings.Compare(left, right)
}
