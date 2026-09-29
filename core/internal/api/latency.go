package api

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"time"
)

// TCPHandshakeLatency TCP 握手延迟（毫秒），供节点测速排序。
func TCPHandshakeLatency(host string, port int) (int64, error) {
	addr := fmt.Sprintf("%s:%d", host, port)
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return -1, err
	}
	_ = conn.Close()
	return time.Since(start).Milliseconds(), nil
}

func hashShort(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])[:8]
}
