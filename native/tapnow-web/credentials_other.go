//go:build !windows

package tapnowweb

const credentialStorage = "服务数据目录（0600 权限文件）"

func protectCredential(b []byte) ([]byte, error)   { return b, nil }
func unprotectCredential(b []byte) ([]byte, error) { return b, nil }
