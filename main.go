package main

import (
	"context"
	"fileSync/config"
	"fileSync/p2p"
	"log"
	"os"
	"os/signal"
	"syscall"
)

var (
	globalConfig config.FileSyncConfig
	ctx          context.Context
)

func main() {
	// 创建将发送接收信号的通道。发送信号且通道未就绪时，通知不会阻止。因此最好创建缓冲通道。
	sig := make(chan os.Signal, 1)
	// Notify将捕获给定的信号并通过sig发送os.Signal值。如果参数中未指定信号，则匹配所有信号。
	signal.Notify(sig, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGHUP)
	// 两种最常见的信号是SIGINT和SIGTERM。这两个信号将导致程序终止。SIGHUP表示调用该进程的终端已经关闭，程序可以通过该信号决定是否移动到后台执行

	// 创建要等待信号处理的通道
	exitChan := make(chan int)
	go func() {
		s := <-sig
		if i, ok := s.(syscall.Signal); ok {
			exitChan <- int(i)
		} else {
			exitChan <- 0
		}
	}()

	var err error
	// 读取配置文件 这个操作也可以直接在StartNetwork里执行 但想了一下后续可能会拓展别的模块 还是在main执行比较好
	globalConfig, err = config.ReadConfig()
	if err != nil {
		log.Println("read config failed", err)
		return
	}

	// 通过p2p网络同步文件 可以在发布的时候其它节点都收到文件并保存 最好是能在定期发布的基础上如果用户手动ctrl+s保存时主动调PublishFile
	canceled := context.CancelFunc(func() {})
	ctx, canceled = context.WithCancel(context.Background())

	// 启动p2p同步
	go p2p.StartNetwork(ctx, globalConfig)

	code := <-exitChan
	canceled()
	os.Exit(code)
}
