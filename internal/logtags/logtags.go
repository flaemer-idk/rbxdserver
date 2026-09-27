// Package logtags — цветные теги компонентов rbxdserver в журнале.
// ANSI-коды: в живом терминале это цвета, в файлах/journal — видимые коды
// (несущественно, зато grep-ается по слову в теге).
package logtags

const (
	// Supervisor — фиолетовый (жирный пурпурный): переходы состояний сессии.
	Supervisor = "\x1b[1;35m[Supervisor]\x1b[0m"
	// Session — жёлтый: игроки, presence, таймеры пустоты.
	Session = "\x1b[1;33m[Session]\x1b[0m"
	// CDN — голубой: постоянный CDN-веб.
	CDN = "\x1b[1;36m[CDN]\x1b[0m"
	// Proc — оранжевый: жизненный цикл дочерних процессов (rfdproc).
	Proc = "\x1b[1;38;5;208m[rfdproc]\x1b[0m"
	// Web — синий: релей stdout веб-процесса сессии.
	Web = "\x1b[1;34m[web]\x1b[0m"
	// RCC — зеленый: релей stdout RCC-процесса.
	RCC = "\x1b[1;32m[rcc]\x1b[0m"
)

// ByPrefix — тег по logPrefix дочернего процесса (web / rcc / CDN).
func ByPrefix(prefix string) string {
	switch prefix {
	case "web":
		return Web
	case "rcc":
		return RCC
	case "CDN":
		return CDN
	default:
		return "[" + prefix + "]"
	}
}
