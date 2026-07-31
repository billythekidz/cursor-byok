//go:build !darwin

package cursor

func RemoveCACertFromDarwinKeychain(_ []byte) error { return nil }
