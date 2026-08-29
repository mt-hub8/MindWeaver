//go:build !windows

package app

func sameBackupDestinationLeaf(left, right string) (bool, error) { return left == right, nil }
