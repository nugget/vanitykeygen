//go:build !unix

package client

// setProcessNiceness is a no-op on non-Unix platforms.
func setProcessNiceness(nice int) error {
	return nil
}
