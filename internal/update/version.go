// Package update implements the fixed DarkPhish release trust boundary.
package update

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
)

var stableVersion = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

// Compare compares full stable SemVer components, never display versions.
func Compare(a, b string) (int, error) {
	parse := func(s string) ([3]uint64, error) {
		var v [3]uint64
		if !stableVersion.MatchString(s) {
			return v, errors.New("invalid stable version")
		}
		for i, part := range strings.Split(s, ".") {
			n, err := strconv.ParseUint(part, 10, 64)
			if err != nil {
				return v, errors.New("version component overflow")
			}
			v[i] = n
		}
		return v, nil
	}
	x, err := parse(a)
	if err != nil {
		return 0, err
	}
	y, err := parse(b)
	if err != nil {
		return 0, err
	}
	for i := range x {
		if x[i] < y[i] {
			return -1, nil
		}
		if x[i] > y[i] {
			return 1, nil
		}
	}
	return 0, nil
}
