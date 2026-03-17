//go:build windows

package main

func stdinPrefetch() ([]byte, bool, error) {
	// Windows 下缺少简单的非阻塞 stdin 检测，保守读取。
	return nil, true, nil
}
