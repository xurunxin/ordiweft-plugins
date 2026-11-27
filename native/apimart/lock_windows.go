// SPDX-License-Identifier: Apache-2.0
package main

import (
	"os"
	"syscall"
	"unsafe"
)

var lockFileEx = syscall.NewLazyDLL("kernel32.dll").NewProc("LockFileEx")

func tryUploadLock(file *os.File) (bool, error) {
	const exclusive, failImmediately = 2, 1
	const lockViolation = syscall.Errno(33)
	var offset syscall.Overlapped
	ret, _, err := lockFileEx.Call(file.Fd(), exclusive|failImmediately, 0, 1, 0, uintptr(unsafe.Pointer(&offset)))
	if ret != 0 {
		return true, nil
	}
	if err == lockViolation {
		return false, nil
	}
	return false, err
}
