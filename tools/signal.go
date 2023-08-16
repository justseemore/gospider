package tools

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

func Signal(preCtx context.Context, fun func()) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch,
		syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGILL, syscall.SIGTRAP,
		syscall.SIGABRT, syscall.SIGBUS, syscall.SIGFPE, syscall.SIGSEGV, syscall.SIGPIPE,
		syscall.SIGALRM, syscall.SIGTERM)
	select {
	case <-preCtx.Done():
		if fun != nil {
			fun()
		}
		signal.Stop(ch)
	case s := <-ch:
		if fun != nil {
			fun()
		}
		signal.Stop(ch)
		signal.Reset(s)
		if p, err := os.FindProcess(os.Getpid()); err == nil && p != nil {
			sg, ok := s.(syscall.Signal)
			if ok && sg == syscall.SIGINT {
				p.Signal(syscall.SIGKILL)
			} else {
				p.Signal(s)
			}
		}
	}
}
