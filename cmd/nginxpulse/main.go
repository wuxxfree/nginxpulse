package main

import (
	"os"
	"strings"
	"syscall"

	"github.com/likaia/nginxpulse/internal/app"
	"github.com/sirupsen/logrus"
)

func main() {
	if err := app.Run(); err != nil {
		logrus.WithError(err).Error("服务启动失败")
		// 在容器的 "direct" 启动模式中，nginx 往往是 PID 1，而 Go 后端是后台启动的子进程。
		// 此时即使后端启动失败退出，容器仍可能因为 nginx 仍在前台而保持 Running。
		// 为了让“启动失败=容器退出”成立，这里检测到 PID 1 为 nginx 时会向其发送 SIGTERM。
		// 安全性：仅在确认 PID 1 是 nginx 时才执行，避免误伤其它运行环境。
		tryTerminatePID1IfNginx()
		os.Exit(1)
	}
}

func tryTerminatePID1IfNginx() {
	commBytes, err := os.ReadFile("/proc/1/comm")
	if err != nil {
		return
	}
	comm := strings.TrimSpace(string(commBytes))
	if comm != "nginx" {
		return
	}
	if err := syscall.Kill(1, syscall.SIGTERM); err != nil {
		logrus.WithError(err).Warn("服务启动失败：尝试终止 PID 1 (nginx) 失败")
		return
	}
	logrus.Warn("服务启动失败：检测到 PID 1 为 nginx，已发送 SIGTERM 以终止容器")
}
