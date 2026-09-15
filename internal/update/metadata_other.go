//go:build !linux

package update

func validateFileMetadata(string) error { return nil }
