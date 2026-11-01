package main

// color.go — цвета в консоли через Win32 SetConsoleTextAttribute.
//
// Почему не ANSI-коды (\x1b[31m): в классической консоли Windows (conhost)
// они не работают без включённого Virtual Terminal Processing, а в комбинации
// с UTF-8-кодировкой (CP 65001, наша установка для русского текста) часто
// печатаются как мусор «←[31m». Прямая установка атрибутов буфера от
// кодировки не зависит и работает везде, начиная с Windows XP.
//
// Если stdout перенаправлен в файл/пайп (GetConsoleScreenBufferInfo не
// срабатывает), цвета отключаются — в файл не попадает управляющего мусора.

import (
	"fmt"
	"syscall"
	"unsafe"
)

const (
	colorRed    uint16 = 0x000C // FOREGROUND_RED | INTENSITY — урон, ошибки
	colorGreen  uint16 = 0x000A // FOREGROUND_GREEN | INTENSITY — лечение, успех
	colorYellow uint16 = 0x000E // FOREGROUND_RED|GREEN|INTENSITY — предупреждения
)

var (
	kernel32     = syscall.NewLazyDLL("kernel32.dll")
	consoleHwnd  uintptr
	defaultAttr  uint16
	colorEnabled bool
)

// consoleScreenBufferInfo — раскладка CONSOLE_SCREEN_BUFFER_INFO.
type consoleScreenBufferInfo struct {
	dwSize              coord
	dwCursorPosition    coord
	wAttributes         uint16
	srWindow            smallRect
	dwMaximumWindowSize coord
}

type coord struct{ x, y int16 }

type smallRect struct{ left, top, right, bottom int16 }

// setupColors определяет, можно ли красить: есть ли настоящая консоль.
func setupColors() {
	getStd := kernel32.NewProc("GetStdHandle")
	// STD_OUTPUT_HANDLE = (DWORD)-11
	h, _, _ := getStd.Call(^uintptr(10))
	if h == 0 || h == ^uintptr(0) {
		return
	}
	var info consoleScreenBufferInfo
	r1, _, _ := kernel32.NewProc("GetConsoleScreenBufferInfo").Call(
		h, uintptr(unsafe.Pointer(&info)))
	if r1 == 0 {
		return // перенаправлен в файл/пайп — без цветов
	}
	consoleHwnd = h
	defaultAttr = info.wAttributes
	colorEnabled = true
}

// setColor ставит атрибут; возвращает прежний.
func setColor(attr uint16) uint16 {
	if !colorEnabled {
		return 0
	}
	old := defaultAttr
	var cur consoleScreenBufferInfo
	if r1, _, _ := kernel32.NewProc("GetConsoleScreenBufferInfo").Call(
		consoleHwnd, uintptr(unsafe.Pointer(&cur))); r1 != 0 {
		old = cur.wAttributes
	}
	kernel32.NewProc("SetConsoleTextAttribute").Call(consoleHwnd, uintptr(attr))
	return old
}

func restoreColor(attr uint16) {
	if colorEnabled {
		kernel32.NewProc("SetConsoleTextAttribute").Call(consoleHwnd, uintptr(attr))
	}
}

// printC печатает в заданном цвете и возвращает исходный.
func printC(attr uint16, format string, args ...any) {
	old := setColor(attr)
	fmt.Printf(format, args...)
	restoreColor(old)
}

// errln — сообщение об ошибке красным.
func errln(msg string) {
	printC(colorRed, "%s\n", msg)
}
