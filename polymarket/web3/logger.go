package web3

import (
	"fmt"
	"io"
	"log"
	"os"
	"sync"
)

// Logger 是 SDK 内部的最小日志接口。
//
// 库代码不该直接 fmt.Println 到 stdout(会污染调用方的日志管道 / JSON 输出 /
// 多 goroutine 并发写 stdout)。所有进度信息走这里,默认到 stderr。
//
// 调用方可以 SetLogger(myLogger) 替换成自己的实现(zap / slog / 静默 / 等等)。
type Logger interface {
	Logf(format string, args ...interface{})
}

// stdLogger 默认走 log.Logger,写 stderr,带前缀和时间戳。
type stdLogger struct {
	l *log.Logger
}

func (s *stdLogger) Logf(format string, args ...interface{}) {
	s.l.Output(2, fmt.Sprintf(format, args...))
}

// DiscardLogger 完全静默,适合生产环境。
type DiscardLogger struct{}

func (DiscardLogger) Logf(format string, args ...interface{}) {}

// PrefixLogger 在 stderr 上加自定义前缀,适合调试。
func PrefixLogger(prefix string) Logger {
	return &stdLogger{l: log.New(os.Stderr, prefix, log.LstdFlags|log.Lmicroseconds)}
}

// WriterLogger 把日志写到任意 io.Writer。
func WriterLogger(w io.Writer, prefix string) Logger {
	return &stdLogger{l: log.New(w, prefix, log.LstdFlags|log.Lmicroseconds)}
}

var (
	loggerMu sync.RWMutex
	logger   Logger = &stdLogger{l: log.New(os.Stderr, "[polymarket/web3] ", log.LstdFlags)}
)

// SetLogger 替换全局 logger。线程安全。
// 传 nil 等同于 DiscardLogger(关闭所有日志)。
func SetLogger(l Logger) {
	loggerMu.Lock()
	defer loggerMu.Unlock()
	if l == nil {
		logger = DiscardLogger{}
	} else {
		logger = l
	}
}

// logf 是内部用的快捷调用,带读锁保护。
func logf(format string, args ...interface{}) {
	loggerMu.RLock()
	l := logger
	loggerMu.RUnlock()
	l.Logf(format, args...)
}
