// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"os"
	"time"
)

// The kernel releases this lock on process exit, including a host timeout.
func lockUploads(ctx context.Context, path string) (*os.File, error) {
	lock, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()
	for ctx.Err() == nil {
		held, err := tryUploadLock(lock)
		if err != nil {
			lock.Close()
			return nil, err
		}
		if held {
			return lock, nil
		}
		select {
		case <-ctx.Done():
		case <-ticker.C:
		}
	}
	lock.Close()
	return nil, ctx.Err()
}
