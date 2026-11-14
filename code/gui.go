package main

// gui.go — окно патчера на чистом Win32. Открывается без аргументов;
// консольный режим — через флаги.

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

// ---------- user32 / kernel32 / comdlg32 ----------

var (
	u32gui = syscall.NewLazyDLL("user32.dll")
	k32gui = syscall.NewLazyDLL("kernel32.dll")
	cdlg   = syscall.NewLazyDLL("comdlg32.dll")
	cc32   = syscall.NewLazyDLL("comctl32.dll")
	sh32   = syscall.NewLazyDLL("shell32.dll")
	ole32  = syscall.NewLazyDLL("ole32.dll")
	gdi    = syscall.NewLazyDLL("gdi32.dll")

	pRegisterClassExW     = u32gui.NewProc("RegisterClassExW")
	pCreateWindowExW      = u32gui.NewProc("CreateWindowExW")
	pDefWindowProc        = u32gui.NewProc("DefWindowProcW")
	pShowWindow           = u32gui.NewProc("ShowWindow")
	pGetMessageW          = u32gui.NewProc("GetMessageW")
	pTranslateMessage     = u32gui.NewProc("TranslateMessage")
	pDispatchMessageW     = u32gui.NewProc("DispatchMessageW")
	pPostQuitMessage      = u32gui.NewProc("PostQuitMessage")
	pSendMessageW         = u32gui.NewProc("SendMessageW")
	pInvalidateRectGui    = u32gui.NewProc("InvalidateRect")
	pBeginPaint           = u32gui.NewProc("BeginPaint")
	pEndPaint             = u32gui.NewProc("EndPaint")
	pFillRect             = u32gui.NewProc("FillRect")
	pCreateSolidBrush     = gdi.NewProc("CreateSolidBrush")
	pCreateFontW          = gdi.NewProc("CreateFontW")
	pDeleteObjectGui      = gdi.NewProc("DeleteObject")
	pDestroyWindow        = u32gui.NewProc("DestroyWindow")
	pGetWindowTextW       = u32gui.NewProc("GetWindowTextW")
	pSetWindowTextW       = u32gui.NewProc("SetWindowTextW")
	pSHBrowseForFolderW   = sh32.NewProc("SHBrowseForFolderW")
	pSHGetPathFromIDListW = sh32.NewProc("SHGetPathFromIDListW")
	pCoInitializeEx       = ole32.NewProc("CoInitializeEx")
	pCoTaskMemFree        = ole32.NewProc("CoTaskMemFree")
	pFreeConsole          = k32gui.NewProc("FreeConsole")
	pInitCommonCtl        = cc32.NewProc("InitCommonControls")
	pEnumDisplayMonitors  = u32gui.NewProc("EnumDisplayMonitors")
	pGetMonitorInfoW      = u32gui.NewProc("GetMonitorInfoW")
	pLoadCursorW          = u32gui.NewProc("LoadCursorW")
	pAdjustWindowRect     = u32gui.NewProc("AdjustWindowRectEx")
	pEnableWindow         = u32gui.NewProc("EnableWindow")
	pSetWindowPos         = u32gui.NewProc("SetWindowPos")
	pSetFocus             = u32gui.NewProc("SetFocus")
	pSetWindowLongPtrW    = u32gui.NewProc("SetWindowLongPtrW")
	pGetWindowLongPtrW    = u32gui.NewProc("GetWindowLongPtrW")
	pCallWindowProcW      = u32gui.NewProc("CallWindowProcW")
	pGetWindowRect        = u32gui.NewProc("GetWindowRect")
	pMonitorFromWindow    = u32gui.NewProc("MonitorFromWindow")
	pGetClientRect        = u32gui.NewProc("GetClientRect")
	pDrawTextW            = u32gui.NewProc("DrawTextW")
	pSelectObject         = gdi.NewProc("SelectObject")
	pSetTextColor         = gdi.NewProc("SetTextColor")
	pSetBkMode            = gdi.NewProc("SetBkMode")
)

const (
	wsChildGui          = 0x40000000
	wsVisibleGui        = 0x10000000
	wsBorder            = 0x00800000
	wsVScrollGui        = 0x00200000
	wsTabstopGui        = 0x00010000
	wsGroup             = 0x00020000
	bsPushbutton        = 0x00000000
	bsDefpushbutton     = 0x00000001
	bsAutoCheckbox      = 0x00000003
	bsGroupbox          = 0x00000007
	cbsDropdownlist     = 0x0003
	esAutohscroll       = 0x0080
	esMultiline         = 0x0004
	esAutoVscroll       = 0x0040
	esReadonly          = 0x0800
	wmPaint             = 0x000F
	wmClose             = 0x0010
	wmDestroy           = 0x0002
	wmCommand           = 0x0111
	wmHscroll           = 0x0114
	wmSetFont           = 0x0030
	wmUser              = 0x0400
	tbmSetRange         = wmUser + 6 // TBM_SETRANGE
	tbmSetPos           = wmUser + 5 // TBM_SETPOS
	bmGetcheck          = 0x00F0
	bmSetcheck          = 0x00F1
	cbAddstring         = 0x0143
	cbSetcursel         = 0x014E
	cbGetcursel         = 0x0147
	csHredrawGui        = 0x0002
	csVredrawGui        = 0x0001
	swShow              = 5
	defaultGuiFont      = 17 // DEFAULT_GUI_FONT
	bifReturnOnlyFsDirs = 0x0001
	bifEditBox          = 0x0010
	bifNewDialogStyle   = 0x0040
)

// msg — раскладка MSG (x64).
type msg struct {
	hwnd     uintptr
	message  uint32
	_        uint32
	wParam   uintptr
	lParam   intptr_t
	time     uint32
	_        uint32
	ptX, ptY int32
}

type intptr_t = int

// wndClassEx — раскладка WNDCLASSEX (x64).
type wndClassExGui struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   uintptr
	ClassName  uintptr
	IconSm     uintptr
}

// paintStruct — раскладка PAINTSTRUCT (x64).
type paintStruct struct {
	Hdc          uintptr
	FErase       uint32
	_            uint32
	RcPaint      [4]int32
	Restore      uint32
	_            uint32
	FUpdateErase uint32
	_            uint32
	Reserved     [32]byte
}

// rect — раскладка RECT.
type rect struct {
	Left, Top, Right, Bottom int32
}

// browseInfoW — раскладка BROWSEINFOW (x64), диалог выбора папки.
type browseInfoW struct {
	HwndOwner   uintptr
	PidlRoot    uintptr
	DisplayName uintptr
	Title       uintptr
	Flags       uint32
	_           uint32
	Callback    uintptr
	LParam      uintptr
	Image       int32
	_           uint32
}

// utf16Keep — строки для WinAPI должны жить весь сеанс.
var utf16Keep []*uint16

func u16p(s string) *uint16 {
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		p, _ = syscall.UTF16PtrFromString("?")
	}
	utf16Keep = append(utf16Keep, p)
	return p
}

// ---------- состояние интерфейса ----------

// школа-строки в порядке рядов интерфейса; ключи — маски базовых школ.
var guiSchools = []struct {
	mask byte
	ru   string
	en   string
}{
	{0x01, "Физика", "Physical"},
	{0x02, "Святая", "Holy"},
	{0x04, "Огонь", "Fire"},
	{0x08, "Природа", "Nature"},
	{0x10, "Лёд", "Frost"},
	{0x20, "Тень", "Shadow"},
	{0x40, "Тайная", "Arcane"},
	{0x7F, "Все школы", "All schools (chaos)"},
}

// mixedRow — строка редактора сочетания школ.
type mixedRow struct {
	mask  byte
	label uintptr
	edits [3]uintptr
}

type guiState struct {
	hwnd        uintptr
	font        uintptr
	exeEdit     uintptr
	dmgCombo    uintptr
	dmgDec      uintptr
	healCombo   uintptr
	healDec     uintptr
	chkSchool   uintptr
	chkAddon    uintptr
	chkAddonSch uintptr
	chkTexts    uintptr
	btnReset    uintptr
	btnBrowse   uintptr
	btnScale    uintptr
	sliders     [8][3]uintptr // [школа][r,g,b] — дескрипторы трекбаров
	rgbEdits    [8][3]uintptr // поля ручного ввода R/G/B
	base        [8][3]uint8   // базовые цвета
	sync        bool          // true, пока текст полей меняем программно

	chkSchoolOn bool // галочки — состояние переживает пересоздание окна
	chkAddonOn  bool
	chkAddSchOn bool
	chkTextsOn  bool // «до 8 всплывающих чисел вместо 4»
	scaleIdx    int  // -1 = авто (по монитору), 0..3 — тест HD/FHD/2K/4K
	rescale     bool // окно закрыто ради пересоздания с новым масштабом

	classRegistered bool
	expanded        bool              // раскрыт список сочетаний школ
	overrides       map[byte][3]uint8 // ручные цвета смешанных масок
	mixedRows       []mixedRow

	// окно возвращается на своё место после пересоздания
	savedPos  *rect // последний GetWindowRect окна
	savedWork rect  // рабочая область монитора, где стояло окно

	// попап-окно уведомлений (стандартные контролы)
	logWin           uintptr
	logClosed        bool // окно уведомлений закрыто вручную
	logEdit          uintptr
	logWinRegistered bool
	logLines         []string // последние сообщения
	editProc         map[uintptr]uintptr

	// секция сочетаний показывается/скрывается стрелкой
	gbMix     uintptr
	btnToggle uintptr
	// состояние, переживаемое пересозданием окна (кнопка масштаба)
	savedExe string
	savedSel [4]int // dmg, dmgDec, heal, healDec — индексы комбобоксов

	hintFont uintptr // крупный жирный шрифт плашки
	// скрывается, пока клиент не выбран
	lockables []uintptr
	sticky    []uintptr // верхний ряд — виден всегда
	loaded    bool      // клиент выбран — настройки разблокированы
}

var gui = guiState{
	overrides:   map[byte][3]uint8{},
	chkSchoolOn: true,
	chkAddonOn:  true,
	chkAddSchOn: true,
	chkTextsOn:  true,
	scaleIdx:    -1, // авто
	savedSel:    [4]int{-1, -1, -1, -1},
}

// id элементов управления.
const (
	idBrowse     = 1001
	idPatch      = 1002
	idRestore    = 1003
	idExeEdit    = 1004
	idDmgCombo   = 1005
	idDmgDec     = 1006
	idHealCombo  = 1007
	idHealDec    = 1008
	idChkSchool  = 1009
	idChkAddon   = 1010
	idChkAddSch  = 1011
	idBtnReset   = 1012
	idTestScale  = 1013
	idToggleMix  = 1014
	idChkTexts   = 1015
	idMixedBase  = 5000 // + строка*3 + канал
	idSliderBase = 2000 // + школа*3 + канал
	idRGBBase    = 3000 // + школа*3 + канал
)

const enChange = 0x0300
const enKillfocus = 0x0200

// uiScale — множитель геометрии, подбирается под экран
var uiScale = 1.4

// S — перевод дизайн-координаты в экранные пиксели.
func S(v int32) int32 {
	return int32(float64(v) * uiScale)
}

func createCtlEx(parent uintptr, exStyle uint32, class, text string, style uint32, x, y, w, h int32, id int) uintptr {
	style &^= wsVisibleGui // появится только после сборки окна
	hwnd, _, _ := pCreateWindowExW.Call(uintptr(exStyle), uintptr(unsafe.Pointer(u16p(class))),
		uintptr(unsafe.Pointer(u16p(text))), uintptr(style),
		uintptr(S(x)), uintptr(S(y)), uintptr(S(w)), uintptr(S(h)),
		parent, uintptr(id), 0, 0)
	pSendMessageW.Call(hwnd, wmSetFont, gui.font, 0)
	return hwnd
}

func createCtl(parent uintptr, class, text string, style uint32, x, y, w, h int32, id int) uintptr {
	style &^= wsVisibleGui // появится только после сборки окна
	hwnd, _, _ := pCreateWindowExW.Call(0, uintptr(unsafe.Pointer(u16p(class))),
		uintptr(unsafe.Pointer(u16p(text))), uintptr(style),
		uintptr(S(x)), uintptr(S(y)), uintptr(S(w)), uintptr(S(h)),
		parent, uintptr(id), 0, 0)
	pSendMessageW.Call(hwnd, wmSetFont, gui.font, 0)
	return hwnd
}

func setSlider(h uintptr, pos int) {
	pSendMessageW.Call(h, tbmSetRange, 1, uintptr(255<<16)) // lo=0, hi=255
	pSendMessageW.Call(h, tbmSetPos, 1, uintptr(pos))
}

func getSlider(h uintptr) int {
	r, _, _ := pSendMessageW.Call(h, 0x0400) // TBM_GETPOS
	return int(r)
}

func getCheck(h uintptr) bool {
	r, _, _ := pSendMessageW.Call(h, bmGetcheck, 0, 0)
	return r != 0
}

func setCheck(h uintptr, v bool) {
	p := uintptr(0)
	if v {
		p = 1
	}
	pSendMessageW.Call(h, bmSetcheck, p, 0)
}

func comboAdd(h uintptr, items ...string) {
	for _, it := range items {
		pSendMessageW.Call(h, cbAddstring, 0, uintptr(unsafe.Pointer(u16p(it))))
	}
	pSendMessageW.Call(h, cbSetcursel, 0, 0)
}

func comboSel(h uintptr) int {
	r, _, _ := pSendMessageW.Call(h, cbGetcursel, 0, 0)
	return int(r)
}

func comboSet(h uintptr, i int) {
	pSendMessageW.Call(h, cbSetcursel, uintptr(i), 0)
}

// updateDecEnabled — список «зн:» активен только для режима K/M
// (в остальных знаков после точки нет).
func updateDecEnabled() {
	on := func(ok bool) uintptr {
		if ok {
			return 1
		}
		return 0
	}
	pEnableWindow.Call(gui.dmgDec, on(comboSel(gui.dmgCombo) == 2))
	pEnableWindow.Call(gui.healDec, on(comboSel(gui.healCombo) == 2))
}

func getText(h uintptr) string {
	n, _, _ := pSendMessageW.Call(h, 0x000E, 0, 0) // WM_GETTEXTLENGTH
	if n == 0 {
		return ""
	}
	buf := make([]uint16, n+1)
	pGetWindowTextW.Call(h, uintptr(unsafe.Pointer(&buf[0])), uintptr(n+1))
	return syscall.UTF16ToString(buf)
}

// ---------- окно уведомлений ----------

// appendLog — показываем сообщение в окне уведомлений.
func appendLog(s string) {
	for _, ln := range strings.Split(s, "\n") {
		ln = strings.TrimSpace(strings.ReplaceAll(ln, "\r", ""))
		gui.logLines = append(gui.logLines, ln)
	}
	if n := len(gui.logLines); n > 8 {
		gui.logLines = gui.logLines[n-8:]
	}
	if strings.TrimSpace(genLogText()) == "" || gui.hwnd == 0 {
		return
	}
	showLogWindow()
}

// genLogText — текст для EDIT; разрыв строки в нём — только CRLF.
func genLogText() string {
	t := strings.TrimSpace(strings.Join(gui.logLines, "\n"))
	return strings.ReplaceAll(t, "\n", "\r\n")
}

// notifyClear — новое уведомление пишется с чистого листа.
func notifyClear() {
	gui.logLines = gui.logLines[:0]
}

// showLogWindow — создать/показать окно с текстом уведомления.
func showLogWindow() {
	gui.logClosed = false // новое сообщение снова открывает окно
	if gui.logWin == 0 {
		createLogWindow()
		if gui.logWin == 0 {
			return
		}
	}
	// заменить текст и прокрутить в конец
	text := genLogText()
	pSetWindowTextW.Call(gui.logEdit, uintptr(unsafe.Pointer(u16p(text))))
	pSendMessageW.Call(gui.logEdit, 0x00B1, ^uintptr(0), ^uintptr(0)) // EM_SETSEL
	pSendMessageW.Call(gui.logEdit, 0x00B7, 0, 0)                     // EM_SCROLLCARET

	// по центру главного окна
	var wr rect
	pGetWindowRect.Call(gui.hwnd, uintptr(unsafe.Pointer(&wr)))
	hr := logWindowRect()
	x := (wr.Left+wr.Right)/2 - (hr.Right-hr.Left)/2
	y := (wr.Top+wr.Bottom)/2 - (hr.Bottom-hr.Top)/2
	pSetWindowPos.Call(gui.logWin, 0, uintptr(uint32(x)), uintptr(uint32(y)), 0, 0, 0x15) // NOZORDER|NOACTIVATE
	pShowWindow.Call(gui.logWin, 8)                                                       // SW_SHOWNOACTIVATE
}

// logWindowRect — наружные размеры окна уведомлений (для центрирования).
func logWindowRect() rect {
	const winStyle = 0x00C80000
	rc := rect{Left: 0, Top: 0, Right: S(480), Bottom: S(210)}
	pAdjustWindowRect.Call(uintptr(unsafe.Pointer(&rc)), winStyle, 0, 0)
	return rect{0, 0, rc.Right - rc.Left, rc.Bottom - rc.Top}
}

// createLogWindow — маленькое окно со стандартными контролами.
func createLogWindow() {
	if !gui.logWinRegistered {
		hInst, _, _ := pGetModuleHandle()
		name := u16p("dmgPatcherLog")
		wc := wndClassExGui{
			Size:       uint32(unsafe.Sizeof(wndClassExGui{})),
			WndProc:    logWndProcPtr,
			Instance:   hInst,
			Cursor:     pLoadCursorW_Cached(),
			Background: 5, // COLOR_WINDOW (белый фон)
			ClassName:  uintptr(unsafe.Pointer(name)),
		}
		pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
		gui.logWinRegistered = true
	}
	// размеры в дизайн-пикселях, масштаб везде один
	const winStyle = 0x00C80000 // WS_POPUP|WS_CAPTION|WS_SYSMENU (закр. крестик)
	const clientW, clientH = 480, 210
	rc := rect{Left: 0, Top: 0, Right: S(clientW), Bottom: S(clientH)}
	pAdjustWindowRect.Call(uintptr(unsafe.Pointer(&rc)), winStyle, 0, 0)
	winW, winH := rc.Right-rc.Left, rc.Bottom-rc.Top
	h, _, _ := pCreateWindowExW.Call(0x80, // WS_EX_TOOLWINDOW — без панели задач
		uintptr(unsafe.Pointer(u16p("dmgPatcherLog"))),
		uintptr(unsafe.Pointer(u16p(tr("Уведомления", "Notifications")))),
		winStyle, 0, 0, uintptr(uint32(winW)), uintptr(uint32(winH)),
		gui.hwnd, 0, hInstOf(), 0)
	if h == 0 {
		return
	}
	gui.logWin = h
	// реальная структура строк: без мягкого переноса, длинная строка
	// уезжает под горизонтальный скролл
	gui.logEdit = createCtlEx(h, 0x200, "EDIT", "", wsChildGui|esMultiline|esReadonly|
		esAutoVscroll|esAutohscroll|wsVScrollGui|0x100000, 12, 12, clientW-24, clientH-58, 0)
	pSendMessageW.Call(gui.logEdit, wmSetFont, gui.font, 0)
	okBtn := createCtl(h, "BUTTON", tr("ОК", "OK"), wsChildGui|bsPushbutton|wsTabstopGui,
		(clientW-90)/2, clientH-30, 90, 26, 9001)
	pSendMessageW.Call(okBtn, wmSetFont, gui.font, 0)
	pShowWindow.Call(gui.logEdit, 5)
	pShowWindow.Call(okBtn, 5)
}

// logWndProc — «ОК» и крестик скрывают окно уведомлений.
func logWndProc(hwnd, msg, wparam, lparam uintptr) uintptr {
	switch msg {
	case wmCommand:
		if int(wparam&0xFFFF) == 9001 {
			gui.logClosed = true
			pShowWindow.Call(hwnd, 0)
			return 0
		}
	case wmClose:
		gui.logClosed = true
		pShowWindow.Call(hwnd, 0)
		return 0
	}
	return defWindowProc(hwnd, msg, wparam, lparam)
}

var logWndProcPtr = syscall.NewCallback(logWndProc)

func hInstOf() uintptr {
	h, _, _ := pGetModuleHandle()
	return h
}

func defWindowProc(hwnd, msg, wparam, lparam uintptr) uintptr {
	r, _, _ := pDefWindowProc.Call(hwnd, msg, wparam, lparam)
	return r
}

// ---------- действия ----------

// runCaptured выполняет f, перехватывая stdout в строку (для лога GUI).
func runCaptured(f func()) string {
	r, w, err := os.Pipe()
	if err != nil {
		f()
		return ""
	}
	old := os.Stdout
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	f()
	w.Close()
	out := <-done
	os.Stdout = old
	r.Close()
	return out
}

// paletteFromGUI собирает палитру из ползунков.
func paletteFromGUI() []schoolType {
	base := map[byte][3]uint8{}
	for i, gs := range guiSchools {
		base[gs.mask] = gui.base[i]
	}
	p := blendPalette(base)
	// ручные цвета смешанных масок перекрывают расчётные
	for i := range p {
		if c, ok := gui.overrides[p[i].mask]; ok {
			p[i].color = argb(uint32(c[0]), uint32(c[1]), uint32(c[2]))
		}
	}
	return p
}

func (g *guiState) onPatch() {
	notifyClear()
	exe := strings.TrimSpace(getText(g.exeEdit))
	if exe == "" {
		appendLog(tr("Укажите путь к Wow.exe", "Specify the path to Wow.exe"))
		return
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		appendLog(tr("ОШИБКА: ", "ERROR: ") + err.Error())
		return
	}
	pe, err := parsePE(data)
	if err != nil {
		appendLog(tr("ОШИБКА: ", "ERROR: ") + err.Error())
		return
	}
	dmg := formatCfg{mode: byte(comboSel(g.dmgCombo)), dec: byte(comboSel(g.dmgDec) + 1)}
	heal := formatCfg{mode: byte(comboSel(g.healCombo)), dec: byte(comboSel(g.healDec) + 1)}
	school := getCheck(g.chkSchool)
	wantAddon := getCheck(g.chkAddon)
	addonSchool := wantAddon && getCheck(g.chkAddonSch)
	moreTexts := getCheck(g.chkTexts)
	// успех — короткая сводка; ошибка — весь вывод из консоли
	code := 1
	var out string
	out = runCaptured(func() {
		code = doPatch(exe, pe, dmg, heal, school, wantAddon, addonSchool, moreTexts, paletteFromGUI())
	})
	if code == 0 {
		appendLog(tr("Патч выполнен: цифры боя обновлены.", "Patch done: combat digits updated."))
		if school {
			appendLog(tr("Цвета урона по школам применены.", "School damage colors applied."))
		}
		if moreTexts {
			appendLog(tr("Всплывающие числа: до 8 одновременно (было 4).",
				"Floating numbers: up to 8 at once (was 4)."))
		}
		if wantAddon {
			appendLog(tr("Аддон записан в Interface\\AddOns\\dmg_format_patcher.",
				"Addon written to Interface\\AddOns\\dmg_format_patcher."))
		}
	} else {
		if detail := strings.TrimSpace(out); detail != "" {
			appendLog(detail) // полный вывод ошибки
		} else {
			appendLog(tr("Патч не выполнен. Подробности — в консоли (запуск с флагами).",
				"Patch failed. Details are in the console (run with flags)."))
		}
	}
}

func (g *guiState) onRestore() {
	notifyClear()
	exe := strings.TrimSpace(getText(g.exeEdit))
	if exe == "" {
		appendLog(tr("Укажите путь к Wow.exe", "Specify the path to Wow.exe"))
		return
	}
	code := 0
	var out string
	out = runCaptured(func() {
		code = doRestore(exe)
	})
	if code == 0 {
		appendLog(tr("Оригинал восстановлен: цифры и цвета как в игре, аддон удалён.",
			"Original restored: vanilla digits and colors, addon removed."))
	} else if detail := strings.TrimSpace(out); detail != "" {
		appendLog(detail) // полный вывод ошибки
	}
}

// applyLockState — настройки скрыты, пока не выбран клиент.
func (g *guiState) applyLockState() {
	sw := uintptr(0) // SW_HIDE
	if gui.loaded {
		sw = 5 // SW_SHOW
	}
	for _, h := range gui.sticky {
		pShowWindow.Call(h, 5)
	}
	for _, h := range gui.lockables {
		pShowWindow.Call(h, sw)
	}
	if gui.loaded {
		// секция сочетаний — по стрелке
		swMix := uintptr(0)
		if gui.expanded {
			swMix = 5
		}
		pShowWindow.Call(gui.gbMix, swMix)
		for _, row := range g.mixedRows {
			pShowWindow.Call(row.label, swMix)
			for ch := 0; ch < 3; ch++ {
				pShowWindow.Call(row.edits[ch], swMix)
			}
		}
	}
	pInvalidateRectGui.Call(g.hwnd, 0, 0)
}

// exeDirOf — папка, где лежит патчер (сам exe или его реальный путь).
func exeDirOf() string {
	p, err := os.Executable()
	if err != nil {
		return ""
	}
	if ev, err := filepath.EvalSymlinks(p); err == nil {
		p = ev
	}
	return filepath.Dir(p)
}

// browseCallbackProc — начальная папка диалога выбора.
var browseCallbackProcPtr = syscall.NewCallback(func(hwnd, msg, lp, dat uintptr) uintptr {
	if msg == 1 && dat != 0 { // BFFM_INITIALIZED
		// BFFM_SETSELECTIONW = WM_USER+103
		pSendMessageW.Call(hwnd, 0x400+103, 1, dat)
	}
	return 0
})

func (g *guiState) browse() {
	notifyClear()
	var name [260]uint16
	title := u16p(tr("Выберите папку с Wow.exe", "Select the folder with Wow.exe"))
	// стартовая папка — папка клиента или патчера
	initDir := exeDirOf()
	if cur := strings.TrimSpace(getText(g.exeEdit)); cur != "" {
		if st, err := os.Stat(filepath.Dir(cur)); err == nil && st.IsDir() {
			initDir = filepath.Dir(cur)
		}
	}
	init := u16p(initDir)
	bi := browseInfoW{
		HwndOwner:   g.hwnd,
		DisplayName: uintptr(unsafe.Pointer(&name[0])),
		Title:       uintptr(unsafe.Pointer(title)),
		Flags:       bifReturnOnlyFsDirs | bifEditBox | bifNewDialogStyle,
		Callback:    browseCallbackProcPtr,
		LParam:      uintptr(unsafe.Pointer(init)),
	}
	pidl, _, _ := pSHBrowseForFolderW.Call(uintptr(unsafe.Pointer(&bi)))
	if pidl == 0 {
		return // отмена
	}
	var path [520]uint16
	pSHGetPathFromIDListW.Call(pidl, uintptr(unsafe.Pointer(&path[0])))
	pCoTaskMemFree.Call(pidl)
	folder := syscall.UTF16ToString(path[:])
	if folder == "" {
		return
	}
	// если в папке лежит Wow.exe — подставляем путь сразу до него
	if _, err := os.Stat(filepath.Join(folder, "Wow.exe")); err == nil {
		folder = filepath.Join(folder, "Wow.exe")
	}
	pSetWindowTextW.Call(g.exeEdit, uintptr(unsafe.Pointer(u16p(folder))))
	g.loadExeConfig()
}

func (g *guiState) loadExeConfig() {
	exe := strings.TrimSpace(getText(g.exeEdit))
	data, err := os.ReadFile(exe)
	if err != nil {
		return
	}
	pe, err := parsePE(data)
	if err != nil {
		return
	}
	// клиент прочитан — настройки разблокированы
	g.loaded = true
	g.applyLockState()
	dmg, heal, school := readCurrentConfig(data, pe)
	comboSet(g.dmgCombo, int(dmg.mode))
	comboSet(g.dmgDec, int(clampDec(dmg.dec))-1)
	comboSet(g.healCombo, int(heal.mode))
	comboSet(g.healDec, int(clampDec(heal.dec))-1)
	updateDecEnabled()
	setCheck(g.chkSchool, school)

	// цвета школ из вшитой секции (если патч со школами стоит)
	colors, ok := readSchoolColors(data, pe)
	if !ok {
		g.setDefaultPalette()
		return
	}
	for i, gs := range guiSchools {
		if c, ok := colors[gs.mask]; ok {
			g.base[i] = c
			gui.sync = true
			setSlider(g.sliders[i][0], int(c[0]))
			setSlider(g.sliders[i][1], int(c[1]))
			setSlider(g.sliders[i][2], int(c[2]))
			pSetWindowTextW.Call(g.rgbEdits[i][0], uintptr(unsafe.Pointer(u16p(fmt.Sprint(c[0])))))
			pSetWindowTextW.Call(g.rgbEdits[i][1], uintptr(unsafe.Pointer(u16p(fmt.Sprint(c[1])))))
			pSetWindowTextW.Call(g.rgbEdits[i][2], uintptr(unsafe.Pointer(u16p(fmt.Sprint(c[2])))))
			gui.sync = false
		}
	}
	// цвета смешанных масок — для показа, не как overrides
	for rowIdx, m := range mixedMasks() {
		if c, ok := colors[m]; ok {
			if rowIdx < len(g.mixedRows) && g.mixedRows[rowIdx].mask == m {
				gui.sync = true
				for ch := 0; ch < 3; ch++ {
					pSetWindowTextW.Call(g.mixedRows[rowIdx].edits[ch],
						uintptr(unsafe.Pointer(u16p(fmt.Sprint(c[ch])))))
				}
				gui.sync = false
			}
		}
	}
	pInvalidateRectGui.Call(g.hwnd, 0, 0)
}

// hintRect — плашка «выберите клиент» в дизайн-координатах.
var hintRect = rect{Left: 60, Top: 270, Right: 596, Bottom: 270 + 52}

func paintHint(hdc uintptr) {
	br, _, _ := pCreateSolidBrush.Call(0x00E1FFFF) // RGB(255,255,225)
	rc := rect{S(hintRect.Left), S(hintRect.Top), S(hintRect.Right), S(hintRect.Bottom)}
	pFillRect.Call(hdc, uintptr(unsafe.Pointer(&rc)), br)
	pDeleteObjectGui.Call(br)
	pSetTextColor.Call(hdc, 0x00201E90) // RGB(144,30,32)
	pSetBkMode.Call(hdc, 1)             // TRANSPARENT
	pSelectObject.Call(hdc, gui.hintFont)
	msg := tr("Сначала выберите папку с клиентом — настройка недоступна",
		"Select the client folder first — settings are locked")
	tr2 := rect{rc.Left + S(8), rc.Top, rc.Right - S(8), rc.Bottom}
	pDrawTextW.Call(hdc, uintptr(unsafe.Pointer(u16p(msg))),
		uintptr(len(mustUtf16(msg))-1), uintptr(unsafe.Pointer(&tr2)),
		0x0001|0x0004|0x0020) // DT_CENTER|DT_VCENTER|DT_SINGLELINE
}

// paintFlagPane — кнопка-флаг, рисованная родителем; полосы плюс
// тёмная рамка у выбранного языка.
func paintFlagPane(hdc uintptr, x, y, w, h int32, code string) {
	full := rect{Left: x, Top: y, Right: x + w, Bottom: y + h}
	fillPane(hdc, &full, 0x00FFFFFF)
	if code == "ru" {
		fillPane(hdc, &rect{Left: full.Left, Top: full.Top + h/3,
			Right: full.Right, Bottom: full.Top + 2*h/3}, 0x00A63900)
		fillPane(hdc, &rect{Left: full.Left, Top: full.Top + 2*h/3,
			Right: full.Right, Bottom: full.Bottom}, 0x001E2BD5)
	} else {
		sx := h / 7
		for i := 0; i < 7; i++ {
			c := uint32(0x00FFFFFF)
			if i%2 == 0 {
				c = 0x003422B2
			}
			fillPane(hdc, &rect{Left: full.Left, Top: full.Top + int32(i)*sx,
				Right: full.Right, Bottom: full.Top + int32(i+1)*sx}, c)
		}
		fillPane(hdc, &rect{Left: full.Left, Top: full.Top,
			Right: full.Left + int32(float64(w)*0.45), Bottom: full.Top + sx*4}, 0x006E3B3C)
	}
	if (code == "ru") == (lang == "ru") {
		e := int32(2)
		frame := full
		fillPane(hdc, &rect{Left: frame.Left, Top: frame.Top,
			Right: frame.Right, Bottom: frame.Top + e}, 0x00303030)
		fillPane(hdc, &rect{Left: frame.Left, Top: frame.Bottom - e,
			Right: frame.Right, Bottom: frame.Bottom}, 0x00303030)
		fillPane(hdc, &rect{Left: frame.Left, Top: frame.Top,
			Right: frame.Left + e, Bottom: frame.Bottom}, 0x00303030)
		fillPane(hdc, &rect{Left: frame.Right - e, Top: frame.Top,
			Right: frame.Right, Bottom: frame.Bottom}, 0x00303030)
	}
}

func fillPane(hdc uintptr, rc *rect, color uint32) {
	br, _, _ := pCreateSolidBrush.Call(uintptr(color))
	pFillRect.Call(hdc, uintptr(unsafe.Pointer(rc)), br)
	pDeleteObjectGui.Call(br)
}

// ---------- поля ввода: Enter/Escape уводят фокус ----------

// editHookProc — подмена процедуры поля: Enter/Escape «финализируют»
// значение (EN_KILLFOCUS сработает при потере фокуса).
func editHookProc(hwnd, msg, wparam, lparam uintptr) uintptr {
	if msg == 0x0100 { // WM_KEYDOWN
		if wparam == 0x0D || wparam == 0x1B { // Enter / Escape
			pSetFocus.Call(gui.hwnd)
			return 0
		}
	}
	if old, ok := gui.editProc[hwnd]; ok {
		r, _, _ := pCallWindowProcW.Call(old, hwnd, msg, wparam, lparam)
		return r
	}
	r, _, _ := pDefWindowProc.Call(hwnd, msg, wparam, lparam)
	return r
}

var editHookPtr = syscall.NewCallback(editHookProc)

// hookEdit — подключить поле к перехвату клавиш.
func hookEdit(h uintptr) {
	if h == 0 {
		return
	}
	if gui.editProc == nil {
		gui.editProc = map[uintptr]uintptr{}
	}
	if _, ok := gui.editProc[h]; ok {
		return
	}
	old, _, _ := pGetWindowLongPtrW.Call(h, ^uintptr(3)) // GWLP_WNDPROC = -4
	gui.editProc[h] = old
	pSetWindowLongPtrW.Call(h, ^uintptr(3), editHookPtr)
}

// ---------- отрисовка ----------

func drawSwatches(hdc uintptr) {
	// кнопки-флаги RU/EN под надписью «Клиент»
	paintFlagPane(hdc, S(12), S(40), S(30), S(24), "ru")
	paintFlagPane(hdc, S(44), S(40), S(30), S(24), "en")
	// до выбора клиента вместо настроек рисуется подсказка
	if !gui.loaded {
		paintHint(hdc)
		return
	}
	// образцы в тех же строках, что и ползунки, после RGB-полей
	y := int32(232)
	for i := range guiSchools {
		r := gui.base[i][0]
		gr := gui.base[i][1]
		b := gui.base[i][2]
		color := uint32(0) | uint32(r) | uint32(gr)<<8 | uint32(b)<<16
		br, _, _ := pCreateSolidBrush.Call(uintptr(color))
		rc := rect{S(144), S(y), S(176), S(y + 18)}
		pFillRect.Call(hdc, uintptr(unsafe.Pointer(&rc)), br)
		pDeleteObjectGui.Call(br)
		y += 34
	}

	if gui.expanded {
		for rowIdx, m := range mixedMasks() {
			col := 0
			line := rowIdx
			if rowIdx >= 11 {
				col = 1
				line = rowIdx - 11
			}
			c := gui.mixedColor(m)
			color := uint32(c[0]) | uint32(c[1])<<8 | uint32(c[2])<<16
			br, _, _ := pCreateSolidBrush.Call(uintptr(color))
			// центрируем квадрат по полю ввода строки
			x := S(int32(22 + col*316))
			rowTop := S(int32(568 + line*24))
			margin := (S(20) - S(16)) / 2
			rc := rect{x + S(124), rowTop + margin, x + S(152), rowTop + margin + S(16)}
			pFillRect.Call(hdc, uintptr(unsafe.Pointer(&rc)), br)
			pDeleteObjectGui.Call(br)
		}
	}
}

// ---------- оконная процедура ----------

func wndProc(hwnd, msg uintptr, wparam, lparam uintptr) uintptr {
	switch msg {
	case wmCommand:
		switch int(wparam & 0xFFFF) {
		case idBrowse:
			gui.browse()
		case idPatch:
			gui.onPatch()
		case idRestore:
			gui.onRestore()
		case idTestScale:
			gui.onTestScale()
		case idToggleMix:
			gui.onToggleMix()
		case idDmgCombo, idHealCombo:
			// «сокращение» — единственный режим, где есть знаки после точки
			updateDecEnabled()
		case idBtnReset:
			gui.onResetColors()
		default:
			// ручной ввод RGB: EN_CHANGE от наших полей
			id := int(wparam & 0xFFFF)
			hi := int(wparam>>16) & 0xFFFF
			// путь вбитый руками — читаем при уходе с поля
			if id == idExeEdit && hi == enKillfocus {
				gui.loadExeConfig()
			}
			if id >= idRGBBase && id < idRGBBase+24 {
				if hi == enChange && !gui.sync {
					gui.onRGBEdit(id-idRGBBase, false)
				}
				if hi == enKillfocus {
					gui.onRGBEdit(id-idRGBBase, true)
				}
			}
			if id >= idMixedBase && id < idMixedBase+int(len(gui.mixedRows))*3 {
				if hi == enChange && !gui.sync {
					gui.onMixedEdit((id-idMixedBase)/3, (id-idMixedBase)%3, false)
				}
				if hi == enKillfocus {
					gui.onMixedEdit((id-idMixedBase)/3, (id-idMixedBase)%3, true)
				}
			}
		}
	case 0x0201: // клик: флаги — сменить язык, фон — снять фокус
		x := int16(lparam & 0xFFFF)
		y := int16((lparam >> 16) & 0xFFFF)
		if x >= int16(S(12)) && x < int16(S(44)) && y >= int16(S(40)) && y < int16(S(64)) {
			gui.setLang("ru")
			return 0
		}
		if x >= int16(S(44)) && x < int16(S(74)) && y >= int16(S(40)) && y < int16(S(64)) {
			gui.setLang("en")
			return 0
		}
		pSetFocus.Call(gui.hwnd)
		return 0
	case wmHscroll:
		// перерисовка только задетой колонки образцов
		for i := range guiSchools {
			for ch := 0; ch < 3; ch++ {
				if gui.sliders[i][ch] != lparam {
					continue
				}
				v := uint8(getSlider(gui.sliders[i][ch]))
				if gui.base[i][ch] != v {
					gui.base[i][ch] = v
					gui.sync = true
					pSetWindowTextW.Call(gui.rgbEdits[i][ch],
						uintptr(unsafe.Pointer(u16p(fmt.Sprint(v)))))
					gui.sync = false
				}
				break
			}
		}
		// зависимые сочетания школ меняются тоже
		if len(gui.mixedRows) > 0 {
			bm := map[byte][3]uint8{}
			for i, gs := range guiSchools {
				bm[gs.mask] = gui.base[i]
			}
			pal := blendPalette(bm)
			gui.sync = true
			for rowIdx := range gui.mixedRows {
				mask := gui.mixedRows[rowIdx].mask
				if _, ok := gui.overrides[mask]; ok {
					continue
				}
				for _, st := range pal {
					if st.mask != mask {
						continue
					}
					for ch := 0; ch < 3; ch++ {
						v := uint8(st.color >> (16 - 8*uint(ch)))
						pSetWindowTextW.Call(gui.mixedRows[rowIdx].edits[ch],
							uintptr(unsafe.Pointer(u16p(fmt.Sprint(v)))))
					}
					break
				}
			}
			gui.sync = false
		}
		rc := rect{Left: S(140), Top: S(226), Right: S(180), Bottom: S(512)}
		pInvalidateRectGui.Call(hwnd, uintptr(unsafe.Pointer(&rc)), 0)
		if gui.expanded {
			rc2 := rect{Left: S(130), Top: S(548), Right: S(650), Bottom: S(844)}
			pInvalidateRectGui.Call(hwnd, uintptr(unsafe.Pointer(&rc2)), 0)
		}
	case wmPaint:
		var ps paintStruct
		hdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		drawSwatches(hdc)
		pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	case wmClose:
		pDestroyWindow.Call(hwnd)
	case wmDestroy:
		pPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := pDefWindowProc.Call(hwnd, msg, wparam, lparam)
	return r
}

var wndProcPtr = syscall.NewCallback(wndProc)

// тестовые пресеты «разрешений» для кнопки проверки масштаба.
var testScales = []float64{0.84, 1.27, 1.4, 1.8}
var testNames = []string{"HD", "FHD", "2K", "4K"}

// windowSizeFor — наружный размер окна для текущего состояния
// (свёрнутые/развёрнутые сочетания школ).
func windowSizeFor() (int32, int32) {
	const winStyle = 0x00C80000 | wsVisibleGui // WS_CAPTION|WS_SYSMENU|WS_MINIMIZEBOX
	clientH := int32(536)
	if gui.expanded {
		clientH = 844
	}
	rc := rect{Left: 0, Top: 0, Right: S(656), Bottom: S(clientH)}
	pAdjustWindowRect.Call(uintptr(unsafe.Pointer(&rc)), winStyle, 0, 0)
	return rc.Right - rc.Left, rc.Bottom - rc.Top
}

// winStyleGui — стиль основного окна (фиксированное, без resize).
const winStyleGui = 0x00C80000 | wsVisibleGui

// runGUI — инициализация и цикл пересоздания окна (окно пересоздаётся
// при смене тестового масштаба кнопкой).
func runGUI() {
	// консоль при двойном клике не нужна — окно патчера самодостаточно
	pFreeConsole.Call()
	pInitCommonCtl.Call()        // без него трекбары не создаются нормально
	pCoInitializeEx.Call(0, 0x2) // STA для диалога выбора папки

	for {
		gui.rescale = false
		createUI()
		var m msg
		for {
			r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
			if r == 0 {
				break
			}
			pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
			pDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
		}
		if !gui.rescale {
			break
		}
	}
}

// createUI — регистрация класса (один раз), окно и все контролы
// с текущим uiScale.
func createUI() {
	// окна прошлого цикла мертвы — дескрипторные списки начисто
	gui.lockables = gui.lockables[:0]
	gui.mixedRows = gui.mixedRows[:0]
	gui.logEdit = 0
	gui.editProc = nil
	if gui.font != 0 {
		pDeleteObjectGui.Call(gui.font)
		gui.font = 0
	}
	if gui.hintFont != 0 {
		pDeleteObjectGui.Call(gui.hintFont)
		gui.hintFont = 0
	}

	// целевой монитор — основной
	mons := guiMonitors()
	target := rect{0, 0, 1920, 1080}
	if len(mons) > 0 {
		target = mons[0].r
	}
	// масштаб: тестовый пресет перекрывает авто-подбор под монитор
	if gui.scaleIdx >= 0 {
		uiScale = testScales[gui.scaleIdx]
	} else {
		tmw := float64(target.Right - target.Left)
		tmh := float64(target.Bottom - target.Top)
		uiScale = tmh * 0.75 / 640
		if f := tmw * 0.55 / 656; f < uiScale {
			uiScale = f
		}
		if uiScale > 1.4 {
			uiScale = 1.4
		}
		if uiScale < 0.9 {
			uiScale = 0.9
		}
	}
	hInst, _, _ := pGetModuleHandle()
	cls := u16p("dmgFormatPatcherGui")
	if !gui.classRegistered {
		wc := wndClassExGui{
			Size:       uint32(unsafe.Sizeof(wndClassExGui{})),
			Style:      csHredrawGui | csVredrawGui,
			WndProc:    wndProcPtr,
			Instance:   hInst,
			Cursor:     pLoadCursorW_Cached(),
			Background: 16, // COLOR_BTNFACE+1
			ClassName:  uintptr(unsafe.Pointer(cls)),
		}
		pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
		gui.classRegistered = true
	}

	winW, winH := windowSizeFor()
	// восстановить позицию; не влезает — подтянуть внутрь
	wx := target.Left + (target.Right-target.Left-winW)/2
	wy := target.Top + (target.Bottom-target.Top-winH)/2
	if gui.savedPos != nil && (gui.savedWork.Right > gui.savedWork.Left) {
		wx, wy = gui.savedPos.Left, gui.savedPos.Top
		wk := gui.savedWork
		if wx > wk.Right-winW {
			wx = wk.Right - winW
		}
		if wx < wk.Left {
			wx = wk.Left
		}
		if wy > wk.Bottom-winH {
			wy = wk.Bottom - winH
		}
		if wy < wk.Top {
			wy = wk.Top
		}
	}
	w, _, _ := pCreateWindowExW.Call(0,
		uintptr(unsafe.Pointer(cls)),
		uintptr(unsafe.Pointer(u16p("dmg_format_patcher — WoW 3.3.5a"))),
		winStyleGui,
		uintptr(uint32(wx)), uintptr(uint32(wy)),
		uintptr(winW), uintptr(winH), 0, 0, hInst, 0)
	gui.hwnd = w
	gui.logWin = 0 // окно уведомлений принадлежит старому окну

	fontH := int32(-13*uiScale - 0.5)
	gui.font, _, _ = pCreateFontW.Call(
		uintptr(uint32(fontH)), 0, 0, 0, // высота от масштаба, ширина авто
		0x190, 0, 0, 0, // weight FW_NORMAL
		1, 0, 0, 5, // DEFAULT_CHARSET, CLEARTYPE_QUALITY
		0, uintptr(unsafe.Pointer(u16p("Segoe UI"))))
	// шрифт подсказки — крупнее и жирнее основного
	gui.hintFont, _, _ = pCreateFontW.Call(
		uintptr(uint32(int32(-15*uiScale-0.5))), 0, 0, 0,
		0x2BC, // FW_BOLD
		0, 0, 0,
		1, 0, 0, 5,
		0, uintptr(unsafe.Pointer(u16p("Segoe UI"))))

	// --- путь к клиенту + кнопки ---
	createCtl(w, "STATIC", tr("Клиент:", "Client:"), wsChildGui|wsVisibleGui, 12, 14, 70, 20, 0)
	gui.exeEdit = createCtl(w, "EDIT", "", wsChildGui|wsVisibleGui|wsBorder|esAutohscroll|wsTabstopGui, 82, 12, 376, 22, idExeEdit)
	hookEdit(gui.exeEdit)
	if gui.savedExe != "" {
		pSetWindowTextW.Call(gui.exeEdit, uintptr(unsafe.Pointer(u16p(gui.savedExe))))
	}
	gui.btnBrowse = createCtl(w, "BUTTON", tr("Обзор", "Browse"), wsChildGui|wsVisibleGui|bsPushbutton|wsTabstopGui, 464, 11, 74, 24, idBrowse)
	btnPatch := createCtl(w, "BUTTON", tr("Патчить", "Patch"), wsChildGui|wsVisibleGui|bsDefpushbutton|wsTabstopGui, 554, 11, 92, 24, idPatch)
	btnRestore := createCtl(w, "BUTTON", tr("Восстановить exe", "Restore exe"), wsChildGui|wsVisibleGui|bsPushbutton|wsTabstopGui, 82, 40, 200, 24, idRestore)
	gui.btnScale = createCtl(w, "BUTTON", scaleLabel(), wsChildGui|wsVisibleGui|bsPushbutton|wsTabstopGui, 290, 40, 268, 24, idTestScale)
	gui.sticky = []uintptr{
		createCtl(w, "STATIC", tr("Клиент:", "Client:"), wsChildGui|wsVisibleGui, 12, 14, 70, 20, 0),
		gui.exeEdit, gui.btnBrowse, btnPatch, btnRestore, gui.btnScale,
	}

	// --- формат цифр ---
	gbFmt := createCtl(w, "BUTTON", tr("Формат цифр", "Number format"), wsChildGui|wsVisibleGui|bsGroupbox, 12, 72, 316, 110, 0)
	gui.dmgCombo = createCtl(w, "COMBOBOX", "", wsChildGui|wsVisibleGui|cbsDropdownlist|wsTabstopGui, 84, 92, 164, 200, idDmgCombo)
	gui.dmgDec = createCtl(w, "COMBOBOX", "", wsChildGui|wsVisibleGui|cbsDropdownlist|wsTabstopGui, 278, 92, 40, 200, idDmgDec)
	gui.healCombo = createCtl(w, "COMBOBOX", "", wsChildGui|wsVisibleGui|cbsDropdownlist|wsTabstopGui, 84, 126, 164, 200, idHealCombo)
	gui.healDec = createCtl(w, "COMBOBOX", "", wsChildGui|wsVisibleGui|cbsDropdownlist|wsTabstopGui, 278, 126, 40, 200, idHealDec)
	gui.lockables = append(gui.lockables,
		gbFmt,
		createCtl(w, "STATIC", tr("Урон:", "Damage:"), wsChildGui|wsVisibleGui, 22, 96, 58, 18, 0),
		createCtl(w, "STATIC", tr("зн:", "dec:"), wsChildGui|wsVisibleGui, 254, 96, 24, 18, 0),
		createCtl(w, "STATIC", tr("Лечение:", "Healing:"), wsChildGui|wsVisibleGui, 22, 130, 58, 18, 0),
		createCtl(w, "STATIC", tr("зн:", "dec:"), wsChildGui|wsVisibleGui, 254, 130, 24, 18, 0),
		gui.dmgCombo, gui.dmgDec, gui.healCombo, gui.healDec)
	comboAdd(gui.dmgCombo,
		tr("как в игре (1234567)", "game default (1234567)"),
		tr("точки (1.234.567)", "dots (1.234.567)"),
		tr("сокращение (111.11K)", "short (111.11K)"))
	comboAdd(gui.dmgDec, "1", "2", "3")
	comboSet(gui.dmgDec, 1)
	comboAdd(gui.healCombo,
		tr("как в игре (1234567)", "game default (1234567)"),
		tr("точки (1.234.567)", "dots (1.234.567)"),
		tr("сокращение (111.11K)", "short (111.11K)"))
	comboAdd(gui.healDec, "1", "2", "3")
	comboSet(gui.healDec, 1)
	if gui.savedSel[0] >= 0 {
		comboSet(gui.dmgCombo, gui.savedSel[0])
		comboSet(gui.dmgDec, gui.savedSel[1])
		comboSet(gui.healCombo, gui.savedSel[2])
		comboSet(gui.healDec, gui.savedSel[3])
	}
	updateDecEnabled()

	// «Больше всплывающих чисел»: расширяет массив текстов в exe
	// (PLAYERNAMEDESC::m_worldText[4] → [8]), отдельно от формата цифр.
	gui.chkTexts = createCtl(w, "BUTTON", tr("До 8 всплывающих чисел (вместо 4)",
		"Up to 8 floating numbers (instead of 4)"),
		wsChildGui|wsVisibleGui|bsAutoCheckbox|wsTabstopGui, 22, 154, 296, 18, idChkTexts)
	setCheck(gui.chkTexts, gui.chkTextsOn)
	gui.lockables = append(gui.lockables, gui.chkTexts)

	// --- аддон ---
	gbAddon := createCtl(w, "BUTTON", tr("Цифры на персонаже (получаемый урон/лечение)",
		"Digits on the character (incoming damage/healing)"), wsChildGui|wsVisibleGui|bsGroupbox, 334, 72, 312, 110, 0)
	gui.chkAddon = createCtl(w, "BUTTON", tr("Аддон: переформатировать цифры",
		"Addon: reformat the digits"),
		wsChildGui|wsVisibleGui|bsAutoCheckbox|wsTabstopGui, 346, 98, 290, 18, idChkAddon)
	setCheck(gui.chkAddon, gui.chkAddonOn)
	gui.chkAddonSch = createCtl(w, "BUTTON", tr("Аддон: красить по школе",
		"Addon: color by school"),
		wsChildGui|wsVisibleGui|bsAutoCheckbox|wsTabstopGui, 346, 128, 290, 18, idChkAddSch)
	setCheck(gui.chkAddonSch, gui.chkAddSchOn)
	gui.lockables = append(gui.lockables, gbAddon, gui.chkAddon, gui.chkAddonSch)

	// --- цвета школ ---
	gbSchool := createCtl(w, "BUTTON", tr("Цвет цифр по школам", "School digit colors"), wsChildGui|wsVisibleGui|bsGroupbox, 12, 188, 634, 328, 0)
	gui.chkSchool = createCtl(w, "BUTTON", tr("Красить цифры урона по школам", "Color damage digits by school"),
		wsChildGui|wsVisibleGui|bsAutoCheckbox|wsTabstopGui, 24, 204, 400, 18, idChkSchool)
	setCheck(gui.chkSchool, gui.chkSchoolOn)
	gui.lockables = append(gui.lockables, gbSchool, gui.chkSchool)
	gui.btnReset = createCtl(w, "BUTTON", tr("Сбросить цвета", "Reset colors"),
		wsChildGui|wsVisibleGui|bsPushbutton|wsTabstopGui, 470, 202, 164, 22, idBtnReset)
	gui.lockables = append(gui.lockables, gui.btnReset)

	// --- стрелка: раскрыть/скрыть список сочетаний школ ---
	gui.btnToggle = createCtl(w, "BUTTON", toggleMixLabel(), wsChildGui|wsVisibleGui|bsPushbutton|wsTabstopGui, 12, 512, 634, 22, idToggleMix)
	gui.lockables = append(gui.lockables, gui.btnToggle)
	// секция сочетаний существует всегда: скрыта, если список свёрнут
	gui.gbMix = createCtl(w, "BUTTON", tr("Сочетания школ — свой цвет для каждой маски",
		"School combinations — own color per mask"), wsChildGui|bsGroupbox, 12, 548, 634, 286, 0)
	gui.lockables = append(gui.lockables, gui.gbMix)
	{
		y0 := int32(568)
		rowH := int32(24)
		for rowIdx, m := range mixedMasks() {
			col := int32(0)
			line := rowIdx
			if rowIdx >= 11 {
				col = 1
				line = rowIdx - 11
			}
			yy := y0 + int32(line)*rowH
			x := int32(22) + col*316
			lbl := createCtl(w, "STATIC", mixedMaskName(m), wsChildGui|wsVisibleGui, x, yy, 120, 20, 0)
			var e [3]uintptr
			c := gui.mixedColor(m)
			gui.sync = true
			for ch := 0; ch < 3; ch++ {
				e[ch] = createCtlEx(w, 0x200, "EDIT", "", wsChildGui|wsVisibleGui|esAutohscroll|
					0x2000, // ES_NUMBER, рамка — WS_EX_CLIENTEDGE
					x+166+int32(ch)*40, yy, 36, 20, idMixedBase+rowIdx*3+ch)
				pSetWindowTextW.Call(e[ch], uintptr(unsafe.Pointer(u16p(fmt.Sprint(c[ch])))))
			}
			gui.sync = false
			gui.lockables = append(gui.lockables, lbl, e[0], e[1], e[2])
			for _, hh := range e {
				hookEdit(hh)
				pSendMessageW.Call(hh, 0x00C5, 3, 0) // до трёх цифр
			}
			gui.mixedRows = append(gui.mixedRows, mixedRow{mask: m, label: lbl, edits: e})
		}
	}
	if !gui.expanded {
		pShowWindow.Call(gui.gbMix, 0) // SW_HIDE
		for _, row := range gui.mixedRows {
			pShowWindow.Call(row.label, 0)
			pShowWindow.Call(row.edits[0], 0)
			pShowWindow.Call(row.edits[1], 0)
			pShowWindow.Call(row.edits[2], 0)
		}
	}

	// базовые цвета по умолчанию — из дефолтной палитры
	for i, gs := range guiSchools {
		for _, st := range schoolTypes {
			if st.mask == gs.mask {
				gui.base[i][0] = uint8(st.color >> 16)
				gui.base[i][1] = uint8(st.color >> 8)
				gui.base[i][2] = uint8(st.color)
				break
			}
		}
	}

	y := int32(230)
	nameW := int32(116)
	for i, gs := range guiSchools {
		gui.lockables = append(gui.lockables, createCtl(w, "STATIC", tr(gs.ru, gs.en), wsChildGui|wsVisibleGui, 24, y, nameW, 22, 0))
		for ch := 0; ch < 3; ch++ {
			h := createCtl(w, "msctls_trackbar32", "", wsChildGui|wsVisibleGui|0x0100, // TBS_HORZ
				184+int32(ch)*90, y, 84, 22, idSliderBase+i*3+ch)
			gui.sliders[i][ch] = h
			gui.lockables = append(gui.lockables, h)
			setSlider(h, int(gui.base[i][ch]))

			e := createCtlEx(w, 0x200, "EDIT", "", wsChildGui|wsVisibleGui|esAutohscroll|
				0x2000, // WS_EX_CLIENTEDGE + ES_NUMBER
				456+int32(ch)*48, y+1, 44, 24, idRGBBase+i*3+ch)
			gui.rgbEdits[i][ch] = e
			gui.lockables = append(gui.lockables, e)
			hookEdit(e)
			pSendMessageW.Call(e, 0x00C5, 3, 0) // EM_LIMITTEXT: максимум 3 цифры
			gui.sync = true
			pSetWindowTextW.Call(e, uintptr(unsafe.Pointer(u16p(fmt.Sprint(gui.base[i][ch])))))
			gui.sync = false
		}
		y += 34
	}

	// группбоксы — под своими контролами
	for _, gb := range []uintptr{gbFmt, gbAddon, gbSchool} {
		pSetWindowPos.Call(gb, 1, 0, 0, 0, 0, 0x13) // HWND_BOTTOM
	}

	gui.applyLockState() // скрыть лишнее до показа
	pShowWindow.Call(w, swShow)
	if !gui.logClosed && strings.TrimSpace(genLogText()) != "" {
		showLogWindow() // уведомление, пришедшее во время пересоздания
	}

	// трекбар сбрасывает позицию при первой отрисовке — повторяем
	for i := range guiSchools {
		for ch := 0; ch < 3; ch++ {
			setSlider(gui.sliders[i][ch], int(gui.base[i][ch]))
		}
	}
}

func pGetModuleHandle() (uintptr, uintptr, error) {
	return k32gui.NewProc("GetModuleHandleW").Call(0)
}

// mustUtf16 — UTF-16 кодирование строки без ошибки (пустая — пусто).
func mustUtf16(s string) []uint16 {
	v, err := syscall.UTF16FromString(s)
	if err != nil {
		return []uint16{}
	}
	return v
}

// pLoadCursorW_Cached — стандартная стрелка для класса окна.
func pLoadCursorW_Cached() uintptr {
	h, _, _ := pLoadCursorW.Call(0, 32512) // IDC_ARROW
	return h
}

// onRGBEdit — ручной ввод значения канала: ограничить 0..255, сдвинуть
// ползунок и перерисовать образец; commit — переписать поле
// ограниченным значением (только при уходе с поля).
func (g *guiState) onRGBEdit(idx int, commit bool) {
	i, ch := idx/3, idx%3
	h := g.rgbEdits[i][ch]
	v := parseByte(getText(h))
	if commit && fmt.Sprint(v) != strings.TrimSpace(getText(h)) {
		g.sync = true
		pSetWindowTextW.Call(h, uintptr(unsafe.Pointer(u16p(fmt.Sprint(v)))))
		g.sync = false
		pSendMessageW.Call(h, 0x00B1, ^uintptr(0), ^uintptr(0))
	}
	g.base[i][ch] = v
	g.sync = true
	setSlider(g.sliders[i][ch], int(v))
	g.sync = false
	pInvalidateRectGui.Call(g.hwnd, 0, 0)
}

// onResetColors — вернуть стандартную палитру.
func (g *guiState) onResetColors() {
	g.setDefaultPalette()
}

// setDefaultPalette — вернуть ползунки и поля к стандарту, стереть
// ручные цвета сочетаний.
func (g *guiState) setDefaultPalette() {
	for k := range g.overrides {
		delete(g.overrides, k)
	}
	for i, gs := range guiSchools {
		for _, st := range schoolTypes {
			if st.mask == gs.mask {
				g.base[i][0] = uint8(st.color >> 16)
				g.base[i][1] = uint8(st.color >> 8)
				g.base[i][2] = uint8(st.color)
				break
			}
		}
	}
	bm := map[byte][3]uint8{}
	for i, gs := range guiSchools {
		bm[gs.mask] = g.base[i]
	}
	def := blendPalette(bm)
	g.sync = true
	for rowIdx := range g.mixedRows {
		mask := g.mixedRows[rowIdx].mask
		for _, st := range def {
			if st.mask == mask {
				for ch := 0; ch < 3; ch++ {
					v := uint8(st.color >> (16 - 8*uint(ch)))
					pSetWindowTextW.Call(g.mixedRows[rowIdx].edits[ch],
						uintptr(unsafe.Pointer(u16p(fmt.Sprint(v)))))
				}
				break
			}
		}
	}
	for i := range guiSchools {
		for ch := 0; ch < 3; ch++ {
			setSlider(g.sliders[i][ch], int(g.base[i][ch]))
			pSetWindowTextW.Call(g.rgbEdits[i][ch],
				uintptr(unsafe.Pointer(u16p(fmt.Sprint(g.base[i][ch])))))
		}
	}
	g.sync = false
	pInvalidateRectGui.Call(g.hwnd, 0, 0)
}

// parseByte — число 0..255 из текста поля; мусор → 0.
func parseByte(s string) uint8 {
	v := 0
	for _, c := range strings.TrimSpace(s) {
		if c < '0' || c > '9' {
			return 0
		}
		v = v*10 + int(c-'0')
		if v > 255 {
			return 255
		}
	}
	return uint8(v)
}

// monitorInfoW — раскладка MONITORINFO (x64).
type monitorInfoW struct {
	Size    uint32
	Monitor [4]int32
	Work    [4]int32
	Flags   uint32
}

type guiMonitor struct {
	r       rect
	primary bool
}

// guiMonitors — прямоугольники всех мониторов; primary — первым.
func guiMonitors() []guiMonitor {
	var out []guiMonitor
	cb := syscall.NewCallback(func(hm, hdc, rcp, lp uintptr) uintptr {
		var mi monitorInfoW
		mi.Size = uint32(unsafe.Sizeof(mi))
		pGetMonitorInfoW.Call(hm, uintptr(unsafe.Pointer(&mi)))
		m := guiMonitor{
			r:       rect{mi.Monitor[0], mi.Monitor[1], mi.Monitor[2], mi.Monitor[3]},
			primary: mi.Flags&1 != 0,
		}
		if m.primary {
			out = append([]guiMonitor{m}, out...)
		} else {
			out = append(out, m)
		}
		return 1
	})
	pEnumDisplayMonitors.Call(0, 0, cb, 0)
	return out
}

// scaleLabel — подпись кнопки теста масштаба.
func scaleLabel() string {
	if gui.scaleIdx < 0 {
		return tr("Масштаб интерфейса: авто", "Interface scale: auto")
	}
	return tr("Масштаб: "+testNames[gui.scaleIdx], "Scale: "+testNames[gui.scaleIdx])
}

// onTestScale — кнопка «Масштаб»: HD → FHD → 2K → 4K → авто,
// окно пересоздаётся с новым масштабом.
func (g *guiState) onTestScale() {
	g.chkSchoolOn = getCheck(g.chkSchool)
	g.chkAddonOn = getCheck(g.chkAddon)
	g.chkAddSchOn = getCheck(g.chkAddonSch)
	g.chkTextsOn = getCheck(g.chkTexts)
	g.savedExe = getText(g.exeEdit)
	g.savedSel[0] = comboSel(g.dmgCombo)
	g.savedSel[1] = comboSel(g.dmgDec)
	g.savedSel[2] = comboSel(g.healCombo)
	g.savedSel[3] = comboSel(g.healDec)
	g.saveWindowPos()
	g.scaleIdx = (g.scaleIdx + 1) % len(testScales)
	g.rescale = true
	pDestroyWindow.Call(g.hwnd)
}

// saveWindowPos — запомнить, где стояло окно и на каком мониторе,
// чтобы после пересоздания вернуть его на то же место.
func (g *guiState) saveWindowPos() {
	var r rect
	pGetWindowRect.Call(g.hwnd, uintptr(unsafe.Pointer(&r)))
	g.savedPos = &r
	hm, _, _ := pMonitorFromWindow.Call(g.hwnd, 2) // MONITOR_DEFAULTTONEAREST
	var mi monitorInfoW
	mi.Size = uint32(unsafe.Sizeof(mi))
	pGetMonitorInfoW.Call(hm, uintptr(unsafe.Pointer(&mi)))
	g.savedWork = rect{mi.Work[0], mi.Work[1], mi.Work[2], mi.Work[3]}
}

// mixedMasks — маски-сочетания (все, кроме 8 базовых строк интерфейса).
func mixedMasks() []byte {
	base := map[byte]bool{0x01: true, 0x02: true, 0x04: true, 0x08: true,
		0x10: true, 0x20: true, 0x40: true, 0x7F: true}
	var out []byte
	for _, m := range knownMasks {
		if !base[m] {
			out = append(out, m)
		}
	}
	return out
}

// mixedColor — текущий цвет смешанной маски: override или из палитры.
func (g *guiState) mixedColor(mask byte) [3]uint8 {
	if c, ok := g.overrides[mask]; ok {
		return c
	}
	bm := map[byte][3]uint8{}
	for i, gs := range guiSchools {
		bm[gs.mask] = g.base[i]
	}
	for _, st := range blendPalette(bm) {
		if st.mask == mask {
			return [3]uint8{uint8(st.color >> 16), uint8(st.color >> 8), uint8(st.color)}
		}
	}
	return [3]uint8{200, 200, 200}
}

// mixedMaskName — читаемое имя сочетания школ.
func mixedMaskName(mask byte) string {
	var names map[byte]string
	if lang == "en" {
		names = map[byte]string{0x02: "Holy", 0x04: "Fire", 0x08: "Nature",
			0x10: "Frost", 0x20: "Shadow", 0x40: "Arcane"}
	} else {
		names = map[byte]string{0x02: "Святая", 0x04: "Огонь", 0x08: "Природа",
			0x10: "Лёд", 0x20: "Тень", 0x40: "Тайная"}
	}
	if mask == 0x7E {
		return tr("Все магии", "All magic")
	}
	var parts []string
	for _, b := range []byte{0x40, 0x20, 0x10, 0x08, 0x04, 0x02} {
		if mask&b != 0 {
			parts = append(parts, names[b])
		}
	}
	s := strings.Join(parts, "+")
	if mask&0x01 != 0 {
		s += tr("+физ", "+phys") // HOLYSTRIKE, FLAMESTRIKE и т.п.
	}
	return s
}

// setLang — сменить язык и пересобрать окно (настройки сохраняются).
func (g *guiState) setLang(l string) {
	if lang == l {
		return
	}
	g.chkSchoolOn = getCheck(g.chkSchool)
	g.chkAddonOn = getCheck(g.chkAddon)
	g.chkAddSchOn = getCheck(g.chkAddonSch)
	g.chkTextsOn = getCheck(g.chkTexts)
	g.savedExe = getText(g.exeEdit)
	g.savedSel[0] = comboSel(g.dmgCombo)
	g.savedSel[1] = comboSel(g.dmgDec)
	g.savedSel[2] = comboSel(g.healCombo)
	g.savedSel[3] = comboSel(g.healDec)
	g.saveWindowPos()
	lang = l
	g.rescale = true
	pDestroyWindow.Call(g.hwnd)
}

// ---------- кнопки-флаги ----------

// toggleMixLabel — подпись кнопки раскрытия списка сочетаний.
func toggleMixLabel() string {
	if gui.expanded {
		return tr("▲ Скрыть сочетания школ", "▲ Hide school combinations")
	}
	return tr("▼ Сочетания школ: показать цвета", "▼ School combinations: show colors")
}

// onToggleMix — развернуть/свернуть секцию сочетаний.
func (g *guiState) onToggleMix() {
	if !g.loaded {
		return // при блокировке секция скрыта вместе с настройками
	}
	g.expanded = !g.expanded
	sw := func(h uintptr) {
		if h != 0 {
			pShowWindow.Call(h, map[bool]uintptr{false: 0, true: 5}[g.expanded]) // SW_HIDE/SW_SHOW
		}
	}
	sw(g.gbMix)
	for _, row := range g.mixedRows {
		sw(row.label)
		sw(row.edits[0])
		sw(row.edits[1])
		sw(row.edits[2])
	}
	pSetWindowTextW.Call(g.btnToggle, uintptr(unsafe.Pointer(u16p(toggleMixLabel()))))
	w, h := windowSizeFor()
	// SWP_NOMOVE|SWP_NOZORDER|SWP_NOACTIVATE
	pSetWindowPos.Call(g.hwnd, 0, 0, 0, uintptr(uint32(w)), uintptr(uint32(h)), 0x16)
	g.saveWindowPos() // пересоздания (масштаб) сохранят новое место
	pInvalidateRectGui.Call(g.hwnd, 0, 0)
}

// onMixedEdit — ручной ввод цвета смешанной маски (override); при
// commit поле переписывается ограниченным значением (только уход с поля).
func (g *guiState) onMixedEdit(idx, ch int, commit bool) {
	if idx >= len(g.mixedRows) {
		return
	}
	mask := g.mixedRows[idx].mask
	h := g.mixedRows[idx].edits[ch]
	v := parseByte(getText(h))
	if commit && fmt.Sprint(v) != strings.TrimSpace(getText(h)) {
		gui.sync = true
		pSetWindowTextW.Call(h, uintptr(unsafe.Pointer(u16p(fmt.Sprint(v)))))
		gui.sync = false
		pSendMessageW.Call(h, 0x00B1, ^uintptr(0), ^uintptr(0))
	}
	c := g.mixedColor(mask)
	c[ch] = v
	g.overrides[mask] = c
	pInvalidateRectGui.Call(g.hwnd, 0, 0)
}
