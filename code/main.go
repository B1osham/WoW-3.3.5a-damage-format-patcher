// dmg_format_patcher — патчер летающих цифр урона/лечения WoW 3.3.5a (12340).
//
// Что делает: в клиенте числа боя форматируются через
// SStrPrintf(buf, 0x20, "%d"|"+%d", value) (VA 0x0076F070) в четырёх местах:
//
//	0x0070F8B4, 0x0070F914 — безымянная функция летающего текста урона
//	                          (гейтится CVar s_cvCombatDamage)
//	0x007223CF              — CGUnit_C::AddWorldDamageText
//	0x00722494              — CGUnit_C::AddWorldHealingText (там "+%d")
//
// Патч перенаправляет call на вшитую в exe функцию-форматтер (см. cave.go),
// которая умеет три режима отображения: обычные цифры, 1.234.567 (точки) и
// 111.11K / 111.11M с выбираемым числом знаков после точки (1..3).
// Урон и лечение настраиваются независимо: в пещеру вшиваются две копии кода
// со своими байтами конфигурации.
//
// Куда вшивается код: если секция .zdata (VA 0x00DD1000, 4КБ) пуста — в неё
// (как в исходном dots-патче); если занята (другой патч) — в конец файла
// добавляется новая секция .w3fm, и exe становится больше. Поэтому патч
// ставится и на уже модифицированные клиенты.
//
// Запуск:
//
//	dmg_format_patcher.exe                    — интерактивный выбор формата
//	dmg_format_patcher.exe "D:\WoW\Wow.exe"   — указать exe явно
//	флаги: --dmg=MODE[:DEC] --heal=MODE[:DEC] — без вопросов
//	  MODE: 0|plain — как в игре, 1|dots — 1.234.567, 2|short — 111.11K/111.11M
//	  DEC:  знаков после точки для short (1..3, по умолчанию 2)
//	--lang=ru|en — язык сообщений (по умолчанию русский)
//	--restore — вернуть оригинальные вызовы SStrPrintf
package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const (
	imageBase = 0x00400000

	// Сайт лечения: call rel32 → SStrPrintf. Сайты урона перечислены в doPatch.
	healCallVA = 0x00722494

	// .zdata: VA 0x00DD1000, файловый указатель 0x72CE00, 0x1000 байт.
	zdataName  = ".zdata"
	zdataVA    = 0x00DD1000
	addSecName = ".w3fm"
	fileAlign  = 0x200
	sectAlign  = 0x1000
	secCharRWX = 0xE0000060 // CODE|INITIALIZED_DATA|EXECUTE|READ|WRITE
)

// ---------- язык сообщений ----------

// lang — текущий язык интерфейса: "ru" (по умолчанию) или "en".
var lang = "ru"

// tr возвращает сообщение на выбранном языке. Все тексты программы проходят
// через неё, поэтому перевод держится в одном месте — рядом с местом вывода.
func tr(ru, en string) string {
	if lang == "en" {
		return en
	}
	return ru
}

func main() {
	setupConsole()
	setupColors()

	args, restore := parseArgs(os.Args[1:])
	exe := resolveTarget(args)
	fmt.Println(tr("клиент / client: ", "клиент / client: ") + exe)

	if restore {
		exit(doRestore(exe))
	}

	// Читаем текущую конфигурацию из exe, если наш патч уже стоит.
	data, err := os.ReadFile(exe)
	if err != nil {
		errln("ОШИБКА / ERROR: " + tr(
		"не удалось прочитать файл: ", "failed to read the file: ") + err.Error())
		exit(3)
	}
	pe, err := parsePE(data)
	if err != nil {
		errln("ОШИБКА / ERROR: " + err.Error() +
			tr(" — это не PE-файл клиента 3.3.5a.", " — this is not a 3.3.5a client PE file."))
		exit(2)
	}
	curDmg, curHeal := readCurrentConfig(data, pe)

	var dmg, heal formatCfg
	if cliDmg != "" || cliHeal != "" {
		dmg, err = parseFmt(cliDmg, curDmg)
		if err != nil {
			errln(tr("ОШИБКА: --dmg: ", "ERROR: --dmg: ") + err.Error())
			exit(1)
		}
		heal, err = parseFmt(cliHeal, curHeal)
		if err != nil {
			errln(tr("ОШИБКА: --heal: ", "ERROR: --heal: ") + err.Error())
			exit(1)
		}
	} else {
		dmg, heal = askFormats(curDmg, curHeal)
	}

	// Флаг --addon= без интерактива: да/нет в любом написании.
	if cliAddon != "" {
		switch strings.ToLower(cliAddon) {
		case "1", "да", "yes", "y", "on", "true":
			wantAddon = true
		case "0", "нет", "no", "n", "off", "false":
			wantAddon = false
		default:
			errln(tr("ОШИБКА: --addon= ждёт 1 или 0", "ERROR: --addon= expects 1 or 0"))
			exit(1)
		}
	}

	printC(colorRed, tr("урон:    %s\n", "damage: %s\n"), dmg.describe())
	printC(colorGreen, tr("лечение: %s\n", "heal:   %s\n"), heal.describe())
	code := doPatch(exe, data, pe, dmg, heal)
	if code != 0 {
		exit(code)
	}
	if wantAddon {
		doAddon(exe, dmg, heal)
	}
	exit(0)
}

// wantAddon — решение по аддону «своего» всплывающего текста.
var wantAddon bool

// doAddon пишет аддон в текущую папку и напоминает перенести его в корень
// игры, если патчер запущен не оттуда.
func doAddon(exe string, dmg, heal formatCfg) {
	cwd, err := os.Getwd()
	if err != nil {
		errln(tr("ОШИБКА: не определить текущую папку: ", "ERROR: cannot get current folder: ") + err.Error())
		return
	}
	path, err := writeAddon(cwd, dmg, heal)
	if err != nil {
		errln(err.Error())
		return
	}
	printC(colorGreen, tr("аддон:       %s\n", "addon:       %s\n"), path)

	// Патчер могли запустить не из корня игры — тогда аддон лежит не там,
	// игра аддон не увидит: напоминаем перенести вручную.
	gameDir, _ := filepath.Abs(filepath.Dir(exe))
	cwdAbs, _ := filepath.Abs(cwd)
	if !strings.EqualFold(cwdAbs, gameDir) {
		printC(colorYellow, tr(
			"ВНИМАНИЕ: патчер запущен не из корневой папки игры —\n"+
				"перенесите папку аддона вручную:\n  %s\nв\n  %s\n",
			"NOTE: the patcher was started outside the game root folder —\n"+
				"move the addon folder manually:\n  %s\ninto\n  %s\n"),
			path, filepath.Join(gameDir, "Interface", "AddOns"))
	}
}

// formatCfg — выбранный формат для одной группы чисел.
type formatCfg struct {
	mode byte // 0 plain, 1 dots, 2 short
	dec  byte // знаков после точки для short (1..3)
}

func (f formatCfg) describe() string {
	switch f.mode {
	case 0:
		return tr("обычные цифры (1234567)", "plain digits (1234567)")
	case 1:
		return tr("точки-разделители (1.234.567)", "dot separators (1.234.567)")
	default:
		ex := map[byte]string{1: "111.1", 2: "111.11", 3: "111.111"}[clampDec(f.dec)]
		if lang == "en" {
			return fmt.Sprintf("short (%sK / %sM)", ex, ex)
		}
		return fmt.Sprintf("сокращённо (%sK / %sM)", ex, ex)
	}
}

func clampDec(d byte) byte {
	if d < 1 {
		return 1
	}
	if d > 3 {
		return 3
	}
	return d
}

// ---------- разбор аргументов ----------

var (
	cliDmg   string
	cliHeal  string
	cliAddon string // "" — не задан (интерактив), "1"/"0" — из флага
)

func parseArgs(argv []string) (targets []string, restore bool) {
	for _, a := range argv {
		switch {
		case a == "--restore" || a == "-restore" || a == "/restore":
			restore = true
		case strings.HasPrefix(a, "--dmg="):
			cliDmg = strings.TrimPrefix(a, "--dmg=")
		case strings.HasPrefix(a, "--heal="):
			cliHeal = strings.TrimPrefix(a, "--heal=")
		case strings.HasPrefix(a, "--addon="):
			cliAddon = strings.TrimPrefix(a, "--addon=")
		case strings.HasPrefix(a, "--lang="):
			// Язык должен быть известен до первых сообщений.
			switch strings.ToLower(strings.TrimPrefix(a, "--lang=")) {
			case "ru", "rus", "рус":
				lang = "ru"
			case "en", "eng":
				lang = "en"
			}
		case a == "--help" || a == "-h" || a == "/?":
			printUsage()
			exit(0)
		case strings.HasPrefix(a, "-") || strings.HasPrefix(a, "/"):
			// неизвестный флаг игнорируем
		default:
			if len(targets) == 0 {
				targets = append(targets, a)
			}
		}
	}
	return targets, restore
}

func printUsage() {
	fmt.Println(tr(
		"Патчер летающих цифр урона/лечения WoW 3.3.5a (build 12340).",
		"Damage/heal floating text patcher for WoW 3.3.5a (build 12340)."))
	fmt.Println()
	fmt.Println(`  dmg_format_patcher.exe`+
		tr("                     ищет Wow.exe рядом с собой",
			"                     find Wow.exe next to itself"))
	fmt.Println(`  dmg_format_patcher.exe "D:\WoW\Wow.exe"`+
		tr("    указать exe явно", "    specify exe explicitly"))
	fmt.Println(`  ... --dmg=short:2 --heal=dots`+
		tr("              без вопросов", "              no questions asked"))
	fmt.Println("  ... --lang=ru|en" +
		tr("                  язык сообщений", "                  message language"))
	fmt.Println("  ... --restore" +
		tr("                           вернуть оригинал", "                           restore original calls"))
	fmt.Println()
	fmt.Println(tr(
		"  формат: plain | dots | short[:DEC], DEC=1..3 (по умолчанию 2)",
		"  format: plain | dots | short[:DEC], DEC=1..3 (default 2)"))
}

// parseFmt разбирает "plain" / "dots" / "short" / "short:3" / "2:1" / "0".
func parseFmt(s string, cur formatCfg) (formatCfg, error) {
	if s == "" {
		return cur, nil
	}
	var modeStr, decStr string
	if i := strings.IndexByte(s, ':'); i >= 0 {
		modeStr, decStr = s[:i], s[i+1:]
	} else {
		modeStr = s
	}
	var f formatCfg
	switch strings.ToLower(modeStr) {
	case "0", "plain":
		f.mode = 0
	case "1", "dots":
		f.mode = 1
	case "2", "short":
		f.mode = 2
	default:
		return f, fmtErr(tr(
			"неизвестный режим %q (plain|dots|short)",
			"unknown mode %q (plain|dots|short)"), modeStr)
	}
	f.dec = 2
	if f.mode == 2 && decStr != "" {
		d, err := strconv.Atoi(decStr)
		if err != nil || d < 1 || d > 3 {
			return f, fmtErr(tr(
				"знаков после точки должно быть 1..3, а не %q",
				"digits after dot must be 1..3, not %q"), decStr)
		}
		f.dec = byte(d)
	}
	return f, nil
}

// fmtErr — форматирование ошибки с неконстантной строкой формата:
// vet запрещает fmtErr(tr(...)) напрямую, поэтому форматируем через
// Sprintf и оборачиваем результат.
func fmtErr(format string, args ...any) error {
	return fmt.Errorf("%s", fmt.Sprintf(format, args...))
}

// ---------- интерактив ----------

func askFormats(curDmg, curHeal formatCfg) (dmg, heal formatCfg) {
	r := bufio.NewReader(os.Stdin)
	askLanguage(r)
	printC(colorRed, tr("Цифры урона:", "Damage numbers:")+"\n")
	dmg = askOne(r, curDmg, colorRed)
	printC(colorGreen, tr("Цифры лечения:", "Heal numbers:")+"\n")
	heal = askOne(r, curHeal, colorGreen)
	wantAddon = askAddon(r)
	return dmg, heal
}

// askAddon спрашивает, модифицировать ли аддон Blizzard_CombatText: урон/
// лечение самого себя рисует этот аддон, а не exe, поэтому свой текст
// нельзя исправить патчем exe. Enter = да.
func askAddon(r *bufio.Reader) bool {
	printC(colorYellow, tr(
		"Свой всплывающий текст (Blizzard_CombatText):\n",
		"Own floating text (Blizzard_CombatText):\n"))
	fmt.Println(tr(
		"  1 — модифицировать аддоном (по умолчанию)",
		"  1 — modify via addon (default)"))
	fmt.Println(tr(
		"  0 — не трогать",
		"  0 — leave as is"))
	for {
		fmt.Print(tr("выбор: ", "choose: "))
		line, err := r.ReadString('\n')
		if err != nil {
			exit(1)
		}
		switch strings.TrimSpace(strings.ToLower(line)) {
		case "", "1", "да", "yes", "y", "д", "on", "true":
			return true
		case "0", "нет", "no", "n", "н", "off", "false":
			return false
		default:
			fmt.Println("  " + tr("введите 0 или 1", "enter 0 or 1"))
		}
	}
}

// askLanguage — первый вопрос интерактива: язык сообщений. Enter = русский.
func askLanguage(r *bufio.Reader) {
	fmt.Println("Язык / Choose language:")
	fmt.Println("  1 — Русский (по умолчанию / default)")
	fmt.Println("  2 — English")
	for {
		fmt.Print("выбор / choose: ")
		line, err := r.ReadString('\n')
		if err != nil {
			exit(1)
		}
		switch strings.TrimSpace(line) {
		case "", "1", "ru", "rus", "рус":
			lang = "ru"
			return
		case "2", "en", "eng":
			lang = "en"
			return
		default:
			fmt.Println("  " + tr("введите 1 или 2 / enter 1 or 2", "введите 1 или 2 / enter 1 or 2"))
		}
	}
}

func askOne(r *bufio.Reader, cur formatCfg, menuColor uint16) formatCfg {
	printC(menuColor, tr(
		"  0 — как в игре:      1234567\n  1 — с точками:       1.234.567\n  2 — сокращённо:      111.11K / 111.11M\n",
		"  0 — original:        1234567\n  1 — dots:            1.234.567\n  2 — short:           111.11K / 111.11M\n"))
	fmt.Printf(tr(
		"(Enter — оставить как есть: %s)\n",
		"(Enter — keep current: %s)\n"), cur.describe())
	for {
		fmt.Print(tr("выбор: ", "choose: "))
		line, err := r.ReadString('\n')
		if err != nil {
			exit(1)
		}
		line = strings.TrimSpace(line)
		if line == "" {
			return cur
		}
		f, err := parseFmt(line, cur)
		if err != nil {
			fmt.Println("  "+tr("ошибка: ", "error: ") + err.Error())
			continue
		}
		if f.mode == 2 {
			fmt.Print(tr(
				"знаков после точки (1..3, Enter = 2): ",
				"digits after dot (1..3, Enter = 2): "))
			dl, _ := r.ReadString('\n')
			dl = strings.TrimSpace(dl)
			if dl != "" {
				d, err := strconv.Atoi(dl)
				if err != nil || d < 1 || d > 3 {
					fmt.Println("  " + tr("нужно число 1..3, попробуйте ещё раз", "must be 1..3, try again"))
					continue
				}
				f.dec = byte(d)
			}
		}
		return f
	}
}

// ---------- применение патча ----------

// doPatch вшивает форматтер и переключает вызовы.
func doPatch(exe string, data []byte, pe *peInfo, dmg, heal formatCfg) int {
	// 1. Определяем пещеру: пустая .zdata или новая секция в конце.
	caveOff, caveVA, data, err := ensureCave(data, pe)
	if err != nil {
		errln(tr("ОШИБКА: ", "ERROR: ") + err.Error())
		return 2
	}

	// 2. Образ пещеры: конфиг + две копии кода.
	img, err := buildCaveImage(dmg.mode, clampDec(dmg.dec), heal.mode, clampDec(heal.dec), caveVA)
	if err != nil {
		errln(tr("ОШИБКА сборки кода: ", "ERROR building code: ") + err.Error())
		return 2
	}
	copy(data[caveOff:caveOff+caveSize], img)

	// 3. Перенаправляем вызовы: три сайта урона → код урона, лечение → код
	// лечения. Перезапись E8 rel32 работает и поверх старого dots-патча.
	dmgCallVAs := []uint32{0x0070F8B4, 0x0070F914, 0x007223CF}
	sites := make([][2]uint32, 0, 4)
	for _, va := range dmgCallVAs {
		sites = append(sites, [2]uint32{va, caveVA + offCodeDmg})
	}
	sites = append(sites, [2]uint32{healCallVA, caveVA + offCodeHeal})
	for _, s := range sites {
		off, err := pe.vaToOffset(s[0])
		if err != nil {
			fmt.Printf(tr("ОШИБКА: сайт %08X: %v\n", "ERROR: site %08X: %v\n"), s[0], err)
			return 2
		}
		if data[off] != 0xE8 {
			fmt.Printf(tr(
				"ОШИБКА: по адресу %08X нет команды call (байт %02X) —\n        это не билд 12340 или сайт уже занят чужим патчем.\n",
				"ERROR: address %08X has no call (byte %02X) —\n        not a 12340 build or the site is already patched.\n"),
				s[0], data[off])
			return 2
		}
		rel := int32(s[1]) - int32(s[0]+5)
		data[off] = 0xE8
		binary.LittleEndian.PutUint32(data[off+1:], uint32(rel))
	}

	if err := writeWithBackup(exe, data); err != nil {
		fmt.Println(tr("ОШИБКА: ", "ERROR: ") + err.Error())
		return 3
	}
	fmt.Printf(tr("код:         VA %08X, файл %08X%s\n", "code:        VA %08X, file %08X%s\n"),
		caveVA, caveOff, caveNote)
	fmt.Printf(tr(
		"вызовы:      урон %s → %08X, лечение %08X → %08X\n",
		"calls:       damage %s → %08X, heal %08X → %08X\n"),
		vaList(dmgCallVAs), caveVA+offCodeDmg, healCallVA, caveVA+offCodeHeal)
	fmt.Println(tr("готово. Запустите клиент и проверьте цифры боя.",
		"Done. Run the client and check the combat numbers."))
	return 0
}

var caveNote string

func vaList(vas []uint32) string {
	parts := make([]string, len(vas))
	for i, v := range vas {
		parts[i] = fmt.Sprintf("%08X", v)
	}
	return strings.Join(parts, ",")
}

// ensureCave находит 4КБ под код: пустая .zdata или новая секция в конце
// файла. Возвращает файловое смещение, VA и, возможно, расширенный файл.
func ensureCave(data []byte, pe *peInfo) (int, uint32, []byte, error) {
	// Своя пещера с меткой W3FM — переиспользуем её при повторном запуске,
	// чтобы не плодить секции (иначе каждый прогон добавлял бы новую .w3fm).
	if off, va, ok := findOurCave(data, pe); ok {
		caveNote = tr(" (код уже вшит — переиспользован)", " (code already injected, reused)")
		return off, va, data, nil
	}
	// Пустая .zdata?
	for i := range pe.sections {
		s := &pe.sections[i]
		if s.name == zdataName && s.rawsize >= caveSize {
			off := int(s.rawptr)
			empty := true
			for _, b := range data[off : off+caveSize] {
				if b != 0 {
					empty = false
					break
				}
			}
			if empty {
				caveNote = tr(" (секция .zdata)", " (section .zdata)")
				return off, imageBase + s.vaddr, data, nil
			}
		}
	}
	// Наш прошлый патч уже добавлял секцию?
	for i := range pe.sections {
		s := &pe.sections[i]
		if s.name == addSecName && s.vsize >= caveSize {
			caveNote = tr(" (секция .w3fm уже была)", " (section .w3fm already present)")
			return int(s.rawptr), imageBase + s.vaddr, data, nil
		}
	}
	return addSection(data, pe)
}

// addSection дописывает в конец файла новую секцию RWX и правит заголовки.
func addSection(data []byte, pe *peInfo) (int, uint32, []byte, error) {
	if len(pe.sections) >= 96 {
		return 0, 0, nil, fmtErr(tr(
			"таблица секций переполнена",
			"section table is full"))
	}
	// Хвост последней секции в файле.
	end := uint32(0)
	for i := range pe.sections {
		if e := pe.sections[i].rawptr + pe.sections[i].rawsize; e > end {
			end = e
		}
	}
	rawPtr := alignUp(end, fileAlign)
	rawSize := alignUp(caveSize, fileAlign)

	// VA — за последней секцией, с выравниванием; следим, чтобы не
	// пересечься с .tls/.rsrc, лежащими после: берём максимум конца
	// виртуального пространства.
	virtEnd := uint32(0)
	for i := range pe.sections {
		if e := pe.sections[i].vaddr + pe.sections[i].vsize; e > virtEnd {
			virtEnd = e
		}
	}
	vaddr := alignUp(virtEnd, sectAlign)

	// Заголовки должны влезть: новая запись — после существующих.
	newSecEnd := pe.secTableOff + (len(pe.sections)+1)*40
	if newSecEnd > len(data) || uint32(newSecEnd) > pe.headersSize {
		// Слегка параноидально: за заголовками следуют данные; расширять
		// заголовки не будем — на практике у 12340 запас есть.
		return 0, 0, nil, fmtErr(tr(
			"нет места в таблице секций",
			"no space in the section table"))
	}

	// Дописываем данные в конец файла.
	newLen := int(rawPtr + rawSize)
	out := make([]byte, newLen)
	copy(out, data)

	// Запись секции.
	o := pe.secTableOff + len(pe.sections)*40
	hdr := make([]byte, 40)
	copy(hdr, addSecName)
	binary.LittleEndian.PutUint32(hdr[8:], uint32(caveSize)) // VirtualSize
	binary.LittleEndian.PutUint32(hdr[12:], vaddr)           // VirtualAddress
	binary.LittleEndian.PutUint32(hdr[16:], rawSize)         // SizeOfRawData
	binary.LittleEndian.PutUint32(hdr[20:], rawPtr)          // PointerToRawData
	binary.LittleEndian.PutUint32(hdr[36:], secCharRWX)      // Characteristics
	copy(out[o:], hdr)

	// Число секций и SizeOfImage.
	binary.LittleEndian.PutUint16(out[pe.peOff+6:], uint16(len(pe.sections)+1))
	binary.LittleEndian.PutUint32(out[pe.optOff+56:], alignUp(vaddr+uint32(caveSize), sectAlign))

	// Контрольную сумму обновляем: некоторые лаунчеры её проверяют.
	putChecksum(out, pe)

	caveNote = tr(
		" (добавлена секция .w3fm, файл вырос)",
		" (added section .w3fm, file size increased)")
	return int(rawPtr), imageBase + vaddr, out, nil
}

// doRestore возвращает оригинальные вызовы SStrPrintf и чистит пещеру.
func doRestore(exe string) int {
	data, err := os.ReadFile(exe)
	if err != nil {
		errln("ОШИБКА / ERROR: " + tr(
		"не удалось прочитать файл: ", "failed to read the file: ") + err.Error())
		return 3
	}
	pe, err := parsePE(data)
	if err != nil {
		errln(tr("ОШИБКА: ", "ERROR: ") + err.Error())
		return 2
	}
	dmgCallVAs := []uint32{0x0070F8B4, 0x0070F914, 0x007223CF}
	sites := append([]uint32{}, dmgCallVAs...)
	sites = append(sites, healCallVA)
	for _, va := range sites {
		off, err := pe.vaToOffset(va)
		if err != nil {
			fmt.Printf(tr("ОШИБКА: сайт %08X: %v\n", "ERROR: site %08X: %v\n"), va, err)
			return 2
		}
		if data[off] != 0xE8 {
			fmt.Printf(tr(
				"ОШИБКА: по адресу %08X нет call — файл не похож на билд 12340\n",
				"ERROR: no call at address %08X — the file does not match build 12340\n"), va)
			return 2
		}
		binary.LittleEndian.PutUint32(data[off+1:], uint32(int32(sstrPrintfVA)-int32(va+5)))
	}
	// Чистим пещеру, если она наша.
	if off, _, ok := findOurCave(data, pe); ok {
		for i := off; i < off+caveSize; i++ {
			data[i] = 0
		}
	}
	if err := os.WriteFile(exe, data, 0o644); err != nil {
		errln(tr("ОШИБКА: не удалось записать файл: ", "ERROR: failed to save the file: ") + err.Error())
		return 3
	}
	if cwd, err := os.Getwd(); err == nil && removeAddon(cwd) {
		fmt.Println(tr("аддон dmg_format_patcher удалён.", "dmg_format_patcher addon removed."))
	}
	fmt.Println(tr(
		"восстановлено: вызовы SStrPrintf, вшитый код очищен.",
		"restored: SStrPrintf calls, injected code cleared."))
	return 0
}

// findOurCave ищет пещеру с нашей меткой W3FM (конфиг-область или код).
func findOurCave(data []byte, pe *peInfo) (int, uint32, bool) {
	for i := range pe.sections {
		s := &pe.sections[i]
		if s.rawsize < caveSize {
			continue
		}
		off := int(s.rawptr)
		if off+caveSize > len(data) {
			continue
		}
		if string(data[off+offMagic:off+offMagic+4]) == string(caveMagic[:]) {
			return off, imageBase + s.vaddr, true
		}
	}
	return 0, 0, false
}

// readCurrentConfig читает сохранённые настройки из пещеры (для подсказки
// «оставить как есть» при повторном запуске).
func readCurrentConfig(data []byte, pe *peInfo) (dmg, heal formatCfg) {
	off, _, ok := findOurCave(data, pe)
	if !ok {
		return formatCfg{mode: 0, dec: 2}, formatCfg{mode: 0, dec: 2}
	}
	dmg = formatCfg{mode: data[off+offModeDmg], dec: data[off+offDecDmg]}
	heal = formatCfg{mode: data[off+offModeHeal], dec: data[off+offDecHeal]}
	return dmg, heal
}

func writeWithBackup(exe string, data []byte) error {
	backup := exe + ".bak"
	if _, err := os.Stat(backup); err != nil {
		orig, rerr := os.ReadFile(exe)
		if rerr != nil {
			return fmt.Errorf("%s: %w", tr(
				"не удалось прочитать клиент для бэкапа",
				"failed to read the client for backup"), rerr)
		}
		if werr := os.WriteFile(backup, orig, 0o644); werr != nil {
			return fmt.Errorf("%s: %w", fmt.Sprintf(tr(
				"не удалось создать бэкап %s",
				"failed to create backup %s"), backup), werr)
		}
		fmt.Println(tr("бэкап: ", "backup: ") + backup)
	} else {
		fmt.Println(tr("бэкап уже есть: ", "backup already exists: ") + backup)
	}
	if err := os.WriteFile(exe, data, 0o644); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "denied") {
			return fmt.Errorf("%s: %w", tr(
				"не удалось записать клиент (он запущен?)",
				"failed to save client (is it running?)"), err)
		}
		return fmt.Errorf("%s: %w", tr(
			"не удалось записать клиент",
			"failed to save client"), err)
	}
	return nil
}

// ---------- PE ----------

type peInfo struct {
	imageBase   uint32
	peOff       int
	optOff      int
	secTableOff int
	headersSize uint32
	sections    []peSection
}

type peSection struct {
	name    string
	vaddr   uint32
	vsize   uint32
	rawptr  uint32
	rawsize uint32
}

func parsePE(data []byte) (*peInfo, error) {
	if len(data) < 0x40 || data[0] != 'M' || data[1] != 'Z' {
		return nil, fmtErr(tr(
			"файл не является PE-образом",
			"the file is not a PE image"))
	}
	peOff := int(binary.LittleEndian.Uint32(data[0x3C:]))
	if peOff+24 > len(data) || string(data[peOff:peOff+4]) != "PE\x00\x00" {
		return nil, fmtErr(tr(
			"неверный заголовок PE",
			"invalid PE header"))
	}
	numSec := int(binary.LittleEndian.Uint16(data[peOff+6:]))
	optSize := int(binary.LittleEndian.Uint16(data[peOff+20:]))
	opt := peOff + 24
	if opt+optSize > len(data) || optSize < 224 {
		return nil, fmtErr(tr(
			"неожиданный optional header (не PE32?)",
			"unexpected optional header (not PE32?)"))
	}
	info := &peInfo{
		imageBase:   binary.LittleEndian.Uint32(data[opt+28:]),
		peOff:       peOff,
		optOff:      opt,
		secTableOff: opt + optSize,
		headersSize: binary.LittleEndian.Uint32(data[opt+60:]),
	}
	if info.imageBase != imageBase {
		return nil, fmtErr(tr(
			"база образа %08X, а не %08X",
			"image base is %08X, expected %08X"), info.imageBase, imageBase)
	}
	for i := 0; i < numSec; i++ {
		o := info.secTableOff + i*40
		if o+40 > len(data) {
			return nil, fmtErr(tr(
				"таблица секций обрезана",
				"section table truncated"))
		}
		info.sections = append(info.sections, peSection{
			name:    strings.TrimRight(string(data[o:o+8]), "\x00"),
			vsize:   binary.LittleEndian.Uint32(data[o+8:]),
			vaddr:   binary.LittleEndian.Uint32(data[o+12:]),
			rawsize: binary.LittleEndian.Uint32(data[o+16:]),
			rawptr:  binary.LittleEndian.Uint32(data[o+20:]),
		})
	}
	return info, nil
}

func (pe *peInfo) vaToOffset(va uint32) (int, error) {
	rva := va - pe.imageBase
	for i := range pe.sections {
		s := &pe.sections[i]
		size := s.vsize
		if s.rawsize > size {
			size = s.rawsize
		}
		if rva >= s.vaddr && rva < s.vaddr+size {
			return int(s.rawptr + (rva - s.vaddr)), nil
		}
	}
	return 0, fmtErr(tr(
		"адрес %08X не найден в секциях",
		"address %08X not found in sections"), va)
}

func alignUp(v, a uint32) uint32 {
	if v%a == 0 {
		return v
	}
	return v + a - v%a
}

// putChecksum пересчитывает контрольную сумму PE.
func putChecksum(data []byte, pe *peInfo) {
	// Сначала обнулить старую.
	binary.LittleEndian.PutUint32(data[pe.optOff+64:], 0)
	var sum uint64
	ln := len(data)
	for i := 0; i < ln; i += 2 {
		var w uint64
		if i+1 < ln {
			w = uint64(data[i]) | uint64(data[i+1])<<8
		} else {
			w = uint64(data[i])
		}
		sum += w
		sum = (sum & 0xFFFF) + (sum >> 16)
	}
	sum = (sum & 0xFFFF) + (sum >> 16)
	sum += uint64(ln)
	binary.LittleEndian.PutUint32(data[pe.optOff+64:], uint32(sum))
}

// ---------- утилиты консоли ----------

func setupConsole() {
	k32 := syscall.NewLazyDLL("kernel32.dll")
	k32.NewProc("SetConsoleOutputCP").Call(65001)
}

func resolveTarget(args []string) string {
	if len(args) > 0 && args[0] != "" {
		return args[0]
	}

	// Без аргументов ищем клиент сначала в текущей папке запуска,
	// затем рядом с самим патчером. Имя сравниваем без учёта регистра.
	var dirs []string
	if cwd, err := os.Getwd(); err == nil {
		dirs = append(dirs, cwd)
	}
	if exe, err := os.Executable(); err == nil {
		dirs = append(dirs, filepath.Dir(exe))
	}

	seen := make(map[string]bool)
	for _, dir := range dirs {
		key := strings.ToLower(filepath.Clean(dir))
		if seen[key] {
			continue
		}
		seen[key] = true
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() && strings.EqualFold(entry.Name(), "Wow.exe") {
				cand := filepath.Join(dir, entry.Name())
				if info, err := os.Stat(cand); err == nil && !info.IsDir() {
					return cand
				}
			}
		}
	}

	errln("ОШИБКА / ERROR: Wow.exe не найден в текущей папке или рядом с патчером. / Wow.exe not found in the current folder or next to the patcher.\nУкажите путь к Wow.exe / Specify the path to Wow.exe, например / for example:")
	fmt.Println(`  dmg_format_patcher.exe "D:\Games\World of Warcraft\Wow.exe"`)
	exit(1)
	return ""
}

// exit завершает работу; при интерактивном запуске ждёт Enter.
func exit(code int) {
	if len(os.Args) == 1 {
		fmt.Println()
		fmt.Print("Нажмите Enter, чтобы закрыть... / Press Enter to close...")
		var s string
		_, _ = fmt.Scanln(&s)
	}
	os.Exit(code)
}
