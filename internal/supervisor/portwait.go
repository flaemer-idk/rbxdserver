package supervisor

import (
	"fmt"
	"net"
)

// GetFreePort находит свободный TCP-порт. Классический TOCTOU: между
// закрытием слушателя и биндом ребёнка порт может занять кто-то другой.
// Это осознанный остаточный риск: готовность веба проверяется HTTP-запросом
// (см. rfdproc.WaitForHTTP), так что «чужой» порт даст понятную ошибку старта,
// а не тихую ложную готовность. RCC-порт — UDP, проверить нельзя в принципе
// (документировано в rbxd INTEGRATION.md §3).
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

func itoa(n int) string { return fmt.Sprintf("%d", n) }
