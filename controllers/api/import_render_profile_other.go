//go:build !linux

package api

func renderedProfileRootIsBounded(string) bool {
	return false
}
