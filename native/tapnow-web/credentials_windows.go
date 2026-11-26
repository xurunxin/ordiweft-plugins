//go:build windows

package tapnowweb

import (
	"errors"
	"golang.org/x/sys/windows"
	"unsafe"
)

const credentialStorage = "Windows DPAPI（当前服务用户）"

func protectCredential(b []byte) ([]byte, error)   { return cryptCredential(b, true) }
func unprotectCredential(b []byte) ([]byte, error) { return cryptCredential(b, false) }
func cryptCredential(b []byte, encrypt bool) ([]byte, error) {
	if len(b) == 0 {
		return nil, errors.New("empty credential")
	}
	in := windows.DataBlob{Size: uint32(len(b)), Data: &b[0]}
	var out windows.DataBlob
	var e error
	if encrypt {
		e = windows.CryptProtectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	} else {
		e = windows.CryptUnprotectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	}
	if e != nil {
		return nil, e
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}
