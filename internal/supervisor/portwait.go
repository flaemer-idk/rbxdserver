package supervisor

import (
	"fmt"
	"net"
	"time"
)

// GetFreePort находит первый случайный свободный TCP-порт в системе
func GetFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func WaitForPort(port int, timeout time.Duration) error {
	address := fmt.Sprintf("127.0.0.1:%d", port)
	deadline := time.Now().Add(timeout)
	
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", address, 1*time.Second)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	
	return fmt.Errorf("timeout waiting for port %d", port)
}