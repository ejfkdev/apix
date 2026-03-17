package main

import (
	"os"
)

func main() {
	// 使用 cobra 执行命令
	Execute()
	
	// 向后兼容：如果没有子命令但有 stdin 输入，执行旧的 stdin 模式
	// 这个逻辑现在在 rootCmd.Run 中处理
	_ = os.Stdin
}
