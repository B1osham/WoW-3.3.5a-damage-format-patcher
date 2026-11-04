// dmg_format_patcher — формат цифр боя и цвет урона по школе магии
// для WoW 3.3.5a (12340).
//
// Формат чисел: четыре вызова SStrPrintf перенаправляются на вшитый
// форматтер (plain / 1.234.567 / 111.11K).
// Цвет школы: mov eax/edi, 0xADA6C (жёлтый глобал) заменяется на переход
// в стаб, который по SpellRec::m_schoolMask выбирает цвет из таблицы.
//
// Сайты:
//
//	0x0070F8B4, 0x0070F914, 0x007223CF — формат урона (call)
//	0x00722494                          — формат лечения (call)
//	0x0072240A                          — цвет урона (mov eax)
//	0x0071FA36                          — цвет DoT/сводок (mov edi)
//
// Код вшивается в пустую .zdata либо в новую секцию .w3cs в конце файла.
// Сайты перед патчем сверяются: не исходный байт и не наш переход — отказ.
//
// Запуск:
//
//	dmg_format_patcher.exe                    — найдёт Wow.exe рядом
//	dmg_format_patcher.exe "D:\WoW\Wow.exe"   — указать exe явно
//	dmg_format_patcher.exe --restore          — вернуть оригинал
//	dmg_format_patcher.exe --lang=en          — язык сообщений
package main

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

const (
	imageBase     = 0x00400000
	damageCallVA1 = 0x0070F8B4 // вызов формата урона
	damageCallVA2 = 0x0070F914
	damageCallVA3 = 0x007223CF
	healCallVA    = 0x00722494 // вызов формата лечения
	backDamageVA  = 0x0072240F // возврат стаба цвета: AddWorldDamageText
	backWorldVA   = 0x0071FA3B // возврат стаба цвета: AddWorldText

	zdataName  = ".zdata"
	zdataVA    = 0x00DD1000
	addSecName = ".w3cs"
	fileAlign  = 0x200
	sectAlign  = 0x1000
	secCharRWX = 0xE0000060 // CODE|INITIALIZED_DATA|EXECUTE|READ|WRITE
)

var dmgCallVAs = []uint32{damageCallVA1, damageCallVA2, damageCallVA3}

// colorSite — сайт цвета: адрес mov, исходные байты, смещение стаба.
// 0x71FA36 (AddWorldText) не патчим: чтение m_schoolMask от его
// указателя на запись spell-кэша может выйти за границу секции и
// уронить клиент. Сайт всё же восстанавливается — старые билды его
// патчили.
type colorSite struct {
	va   uint32
	orig [5]byte
	stub uint32
	hook bool
}

var colorSites = []colorSite{
	{0x0072240A, [5]byte{0xB8, 0x6C, 0xAA, 0xAD, 0x00}, offStubA, true},  // AddWorldDamageText
	{0x0071FA36, [5]byte{0xBF, 0x6C, 0xAA, 0xAD, 0x00}, offStubB, false}, // AddWorldText
}

// ---------- язык сообщений ----------

var lang = "ru"

func tr(ru, en string) string {
	if lang == "en" {
		return en
	}
	return ru
}

func fmtErr(format string, args ...any) error {
	return errors.New(fmt.Sprintf(format, args...))
}

func main() {
	// Win32 требует: окно создаётся и обслуживается одним потоком ОС.
	runtime.LockOSThread()
	setupConsole()
	setupColors()

	// Без аргументов — графический интерфейс; флаги — консольный режим.
	if len(os.Args) == 1 {
		runGUI()
		return
	}

	args, restore := parseArgs(os.Args[1:])
	exe := resolveTarget(args)
	fmt.Println(tr("клиент / client: ", "client: ") + exe)

	if restore {
		exit(doRestore(exe))
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		errln(tr("ОШИБКА: ", "ERROR: ") + tr(
			"не удалось прочитать файл: ", "failed to read the file: ") + err.Error())
		exit(3)
	}
	pe, err := parsePE(data)
	if err != nil {
		errln(tr("ОШИБКА: ", "ERROR: ") + err.Error() +
			tr(" — это не PE-файл клиента 3.3.5a.", " — this is not a 3.3.5a client PE file."))
		exit(2)
	}

	curDmg, curHeal, curSchool := readCurrentConfig(data, pe)
	dmg, heal, school, wantAddon, addonSchool, ok := resolveArgs(curDmg, curHeal, curSchool)
	if !ok {
		return
	}
	exit(doPatch(exe, pe, dmg, heal, school, wantAddon, addonSchool, schoolTypes))
}

// ---------- разбор аргументов ----------

var (
	cliDmg      string
	cliHeal     string
	cliAddon    string // "" — не задан, "1"/"0" — из флага
	cliSch      string // "" — не задан, "0"/"1" — цвет школы в exe
	cliAddonSch string // "" — не задан, "0"/"1" — цвет школы в аддоне
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
		case strings.HasPrefix(a, "--school="):
			cliSch = strings.TrimPrefix(a, "--school=")
		case strings.HasPrefix(a, "--addon-school="):
			cliAddonSch = strings.TrimPrefix(a, "--addon-school=")
		case strings.HasPrefix(a, "--lang="):
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
			// неизвестные флаги игнорируем
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
		"Патчер боевых цифр WoW 3.3.5a (12340): форматы чисел и цвет по школе магии.",
		"Combat numbers patcher for WoW 3.3.5a (12340): number formats and spell-school colors."))
	fmt.Println()
	fmt.Println(`  dmg_format_patcher.exe` + tr(
		"                    ищет Wow.exe рядом с собой",
		"                    find Wow.exe next to itself"))
	fmt.Println(`  dmg_format_patcher.exe "D:\WoW\Wow.exe"` + tr(
		"  указать exe явно", "  specify exe explicitly"))
	fmt.Println(`  ... --dmg=short:2 --heal=dots --school=1 --addon=0` + tr(
		" — без вопросов", " — no questions asked"))
	fmt.Println("  ... --lang=ru|en" + tr(
		"                  язык сообщений", "                  message language"))
	fmt.Println("  ... --restore" + tr(
		"                           вернуть оригинал",
		"                           restore original"))
	fmt.Println()
	fmt.Println(tr(
		"  формат: plain | dots | short[:DEC], DEC=1..3 (по умолчанию 2)",
		"  format: plain | dots | short[:DEC], DEC=1..3 (default 2)"))
}

// ---------- формат чисел ----------

// formatCfg — формат одной группы чисел.
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

// ---------- интерактив ----------

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
			fmt.Println("  введите 1 или 2 / enter 1 or 2")
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
			fmt.Println("  " + tr("ошибка: ", "error: ") + err.Error())
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

func askAddon(r *bufio.Reader) bool {
	printC(colorYellow, tr(
		"Свой всплывающий текст (Blizzard_CombatText):\n",
		"Own floating text (Blizzard_CombatText):\n"))
	fmt.Println(tr("  0 — не трогать", "  0 — leave as is"))
	fmt.Println(tr(
		"  1 — модифицировать аддоном (Enter = да)",
		"  1 — modify via addon (Enter = yes)"))
	return yesNo(r)
}

// askAddonSchool — цветной текст в аддоне. Enter = как выбрано в exe.
func askAddonSchool(r *bufio.Reader, exeSchool bool) bool {
	printC(colorYellow, tr(
		"Цветной текст в аддоне (свой всплывающий текст):\n",
		"Colored text in the addon (self floating text):\n"))
	fmt.Println(tr("  0 — без цвета (только формат цифр)", "  0 — no color (numbers only)"))
	fmt.Println(tr("  1 — цвет школы, как в exe", "  1 — school colors, same as in the exe"))
	if exeSchool {
		fmt.Println(tr("  (Enter = 1)", "  (Enter = 1)"))
	} else {
		fmt.Println(tr("  (Enter = 0)", "  (Enter = 0)"))
	}
	return yesNo(r)
}

// askSchool — цвет цифр урона в мире. Enter = включить.
func askSchool(r *bufio.Reader, cur bool) bool {
	printC(colorYellow, tr(
		"Цвет цифр заклинаний:\n",
		"Spell damage color:\n"))
	fmt.Print(tr(
		"  0 — как в игре: жёлтый (255,222,0)\n",
		"  0 — as in the game: yellow (255,222,0)\n"))
	fmt.Print(tr(
		"  1 — по школе заклинания: каждой маске школ из таблицы клиента —\n"+
			"      свой цвет (физика белая, святая золотистая, огонь красный,\n"+
			"      природа зелёная, лёд голубой, тень фиолетовая, тайная бирюзовая,\n"+
			"      все школы сразу — розово-фиолетовая)\n",
		"  1 — by spell school: every client school mask gets its own color\n"+
			"      (physical white, holy golden, fire red, nature green, frost\n"+
			"      light blue, shadow purple, arcane turquoise, all schools —\n"+
			"      chaos pink)\n"))
	fmt.Println(tr(
		"Автоатаки остаются белыми; смешанные школы — свой цвет каждой маске.",
		"Auto-attacks stay white; every school blend has its own color."))
	def := "1"
	if !cur {
		def = "0"
	}
	fmt.Println(tr("  (Enter = "+def+")", "  (Enter = "+def+")"))
	return yesNo(r)
}

func yesNo(r *bufio.Reader) bool {
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

// resolveArgs применяет CLI-флаги, чего не хватило — спрашивает
// интерактивом. ok=false — пользователь отменил.
func resolveArgs(curDmg, curHeal formatCfg, curSchool bool) (dmg formatCfg, heal formatCfg, school bool, wantAddon bool, addonSchool bool, ok bool) {
	r := bufio.NewReader(os.Stdin)

	needInteractive := cliDmg == "" || cliHeal == "" || cliAddon == "" || cliSch == "" ||
		(cliAddonSch == "" && strings.ToLower(cliAddon) != "0")
	if needInteractive {
		askLanguage(r)
	}
	if cliDmg != "" || cliHeal != "" {
		var err error
		dmg, err = parseFmt(cliDmg, curDmg)
		if err != nil {
			errln(tr("ОШИБКА: --dmg: ", "ERROR: --dmg: ") + err.Error())
			return dmg, heal, school, wantAddon, addonSchool, false
		}
		heal, err = parseFmt(cliHeal, curHeal)
		if err != nil {
			errln(tr("ОШИБКА: --heal: ", "ERROR: --heal: ") + err.Error())
			return dmg, heal, school, wantAddon, addonSchool, false
		}
	} else {
		printC(colorRed, tr("Цифры урона:", "Damage numbers:")+"\n")
		dmg = askOne(r, curDmg, colorRed)
		printC(colorGreen, tr("Цифры лечения:", "Heal numbers:")+"\n")
		heal = askOne(r, curHeal, colorGreen)
	}
	switch strings.ToLower(cliSch) {
	case "1", "да", "yes", "y", "on", "true":
		school = true
	case "0", "нет", "no", "n", "off", "false":
		school = false
	default:
		school = askSchool(r, curSchool)
	}
	switch strings.ToLower(cliAddon) {
	case "1", "да", "yes", "y", "on", "true":
		wantAddon = true
	case "0", "нет", "no", "n", "off", "false":
		wantAddon = false
	default:
		wantAddon = askAddon(r)
	}
	switch strings.ToLower(cliAddonSch) {
	case "1", "да", "yes", "y", "д", "on", "true":
		addonSchool = true
	case "0", "нет", "no", "n", "н", "off", "false":
		addonSchool = false
	default:
		if wantAddon {
			addonSchool = askAddonSchool(r, school)
		} else {
			addonSchool = false
		}
	}
	return dmg, heal, school, wantAddon, addonSchool, true
}

// ---------- применение патча ----------

// readCurrentConfig — настройки, сохранённые во вшитой секции.
func readCurrentConfig(data []byte, pe *peInfo) (dmg, heal formatCfg, school bool) {
	dmg = formatCfg{mode: 0, dec: 2}
	heal = formatCfg{mode: 0, dec: 2}
	off, _, ok := findOurCave(data, pe)
	if !ok {
		return dmg, heal, false
	}
	dmg = formatCfg{mode: data[off+offModeDmg], dec: data[off+offDecDmg]}
	heal = formatCfg{mode: data[off+offModeHeal], dec: data[off+offDecHeal]}
	for i := range pe.sections {
		s := &pe.sections[i]
		base := int(s.rawptr)
		if s.rawsize < caveSize || base+offSchMagic+4 > len(data) {
			continue
		}
		if string(data[base+offSchMagic:base+offSchMagic+4]) == string(schMagic[:]) {
			school = true
			break
		}
	}
	return dmg, heal, school
}

// siteByte — ожидаемый первый байт сайта.
func siteExpected(va uint32) byte {
	for _, d := range dmgCallVAs {
		if d == va {
			return 0xE8 // call
		}
	}
	if va == healCallVA {
		return 0xE8 // call
	}
	for _, sp := range colorSites {
		if sp.va == va {
			return sp.orig[0]
		}
	}
	return 0xB8 // mov
}

func doPatch(exe string, pe *peInfo, dmg, heal formatCfg, school, wantAddon, addonSchool bool, palette []schoolType) (code int) {
	caveOff, caveVA, data, err := ensureCave(exe, pe)
	if err != nil {
		errln(tr("ОШИБКА: ", "ERROR: ") + err.Error())
		return 2
	}

	img, imgErr := buildCaveImage(dmg.mode, clampDec(dmg.dec), heal.mode, clampDec(heal.dec), school, caveVA, palette)
	if imgErr != nil {
		errln(tr("ОШИБКА сборки кода: ", "ERROR building code: ") + imgErr.Error())
		return 2
	}
	copy(data[caveOff:caveOff+caveSize], img)

	type edit struct{ va, target uint32 }
	var edits []edit
	for _, va := range dmgCallVAs {
		edits = append(edits, edit{va, caveVA + offCodeDmg})
	}
	edits = append(edits, edit{healCallVA, caveVA + offCodeHeal})

	// сайты, снятые с перехвата (0x71FA36 всегда, 0x72240A при
	// school=false): вернуть исходные байты, иначе старый стаб останется
	for _, sp := range colorSites {
		if sp.hook && school {
			continue
		}
		off, oerr := pe.vaToOffset(sp.va)
		if oerr != nil {
			continue
		}
		if data[off] == 0xE9 {
			copy(data[off:off+5], sp.orig[:])
		}
	}
	if school {
		for _, sp := range colorSites {
			if !sp.hook {
				continue
			}
			edits = append(edits, edit{sp.va, caveVA + sp.stub})
		}
	}

	for _, ed := range edits {
		off, oerr := pe.vaToOffset(ed.va)
		if oerr != nil {
			errln(tr("ОШИБКА: ", "ERROR: ") + fmtErr(tr(
				"сайт %08X не найден в секциях", "site %08X not found in sections"), ed.va).Error())
			return 2
		}
		first := data[off]
		// исходный байт или наш старый переход (E9) — оба варианта правим заново
		if first != siteExpected(ed.va) && first != 0xE9 {
			errln(tr("ОШИБКА: ", "ERROR: ") + fmtErr(tr(
				"по адресу %08X нет call/mov (байт %02X) — это не билд 12340 или сайт занят чужим патчем; файл не изменён.",
				"address %08X holds neither call nor mov (byte %02X) — not a 12340 build, or a foreign patch is there; file not modified."), ed.va, first).Error())
			return 2
		}
		data[off] = 0xE8
		if ed.target != caveVA+offCodeDmg && ed.target != caveVA+offCodeHeal {
			data[off] = 0xE9 // цветовый сайт — jmp на стаб
		}
		rel := int32(ed.target) - int32(ed.va+5)
		binary.LittleEndian.PutUint32(data[off+1:], uint32(rel))
	}

	putChecksum(data, pe)
	if err := writeWithBackup(exe, data); err != nil {
		errln(tr("ОШИБКА: ", "ERROR: ") + err.Error())
		return 3
	}

	fmt.Printf(tr("код:         VA %08X, файл %08X%s\n", "code:        VA %08X, file %08X%s\n"),
		caveVA, caveOff, secNote)
	fmt.Printf(tr(
		"форматы:     урон %s → %08X, лечение %08X → %08X\n",
		"formats:     damage %s → %08X, heal %08X → %08X\n"),
		dmg.describe(), caveVA+offCodeDmg, healCallVA, caveVA+offCodeHeal)
	if school {
		fmt.Printf(tr(
			"цвет школы:  урон %08X → %08X\n",
			"school colors: damage %08X → %08X\n"),
			colorSites[0].va, caveVA+offStubA)
	}
	fmt.Println(tr("готово. Запустите клиент и проверьте цифры боя.",
		"Done. Run the client and check the combat numbers."))
	if wantAddon {
		doAddon(exe, dmg, heal, addonSchool, palette)
	}
	return 0
}

var secNote string

// ensureCave находит место под вшитый код: наша секция → пустая .zdata →
// новая секция .w3cs в конце файла.
func ensureCave(exe string, pe *peInfo) (int, uint32, []byte, error) {
	data, err := os.ReadFile(exe)
	if err != nil {
		return 0, 0, nil, fmtErr(tr(
			"не удалось прочитать файл: ", "failed to read the file: ") + err.Error())
	}
	if off, va, ok := findOurCave(data, pe); ok {
		secNote = tr(" (вшитый код переиспользован)", " (injected code reused)")
		return off, va, data, nil
	}
	// Пустая .zdata?
	for i := range pe.sections {
		s := &pe.sections[i]
		if s.name == zdataName && s.rawsize >= caveSize &&
			s.vsize >= caveSize && s.char&0xE0000000 == 0xE0000000 {
			off := int(s.rawptr)
			if off+caveSize > len(data) {
				continue
			}
			empty := true
			for _, b := range data[off : off+caveSize] {
				if b != 0 {
					empty = false
					break
				}
			}
			if empty {
				secNote = tr(" (секция .zdata)", " (section .zdata)")
				return off, imageBase + s.vaddr, data, nil
			}
		}
	}
	// Чужая секция .w3cs подходящего размера — переписываем целиком.
	for i := range pe.sections {
		s := &pe.sections[i]
		if s.name == addSecName && s.rawsize >= caveSize &&
			s.vsize >= caveSize && s.char&0xE0000000 == 0xE0000000 {
			off := int(s.rawptr)
			if off+caveSize > len(data) {
				continue
			}
			secNote = tr(" (секция .w3cs переписана)", " (section .w3cs rewritten)")
			return off, imageBase + s.vaddr, data, nil
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
	end := uint32(0)
	for i := range pe.sections {
		if e := pe.sections[i].rawptr + pe.sections[i].rawsize; e > end {
			end = e
		}
	}
	rawPtr := alignUp(end, fileAlign)
	rawSize := alignUp(caveSize, fileAlign)

	virtEnd := uint32(0)
	for i := range pe.sections {
		if e := pe.sections[i].vaddr + pe.sections[i].vsize; e > virtEnd {
			virtEnd = e
		}
	}
	vaddr := alignUp(virtEnd, sectAlign)

	newSecEnd := pe.secTableOff + (len(pe.sections)+1)*40
	if newSecEnd > len(data) || uint32(newSecEnd) > pe.headersSize {
		return 0, 0, nil, fmtErr(tr(
			"нет места в таблице секций",
			"no space in the section table"))
	}

	newLen := int(rawPtr + rawSize)
	out := make([]byte, newLen)
	copy(out, data)

	o := pe.secTableOff + len(pe.sections)*40
	hdr := make([]byte, 40)
	copy(hdr, addSecName)
	binary.LittleEndian.PutUint32(hdr[8:], caveSize) // VirtualSize
	binary.LittleEndian.PutUint32(hdr[12:], vaddr)   // VirtualAddress
	binary.LittleEndian.PutUint32(hdr[16:], rawSize) // SizeOfRawData
	binary.LittleEndian.PutUint32(hdr[20:], rawPtr)  // PointerToRawData
	binary.LittleEndian.PutUint32(hdr[36:], secCharRWX)
	copy(out[o:], hdr)

	binary.LittleEndian.PutUint16(out[pe.peOff+6:], uint16(len(pe.sections)+1))
	binary.LittleEndian.PutUint32(out[pe.optOff+56:], alignUp(vaddr+caveSize, sectAlign))

	putChecksum(out, pe)
	secNote = tr(
		" (добавлена секция .w3cs, файл вырос)",
		" (added section .w3cs, file size increased)")
	return int(rawPtr), imageBase + vaddr, out, nil
}

// doRestore возвращает оригинальные байты на всех сайтах и чистит код.
func doRestore(exe string) int {
	data, err := os.ReadFile(exe)
	if err != nil {
		errln(tr("ОШИБКА: ", "ERROR: ") + tr(
			"не удалось прочитать файл: ", "failed to read the file: ") + err.Error())
		return 3
	}
	pe, err := parsePE(data)
	if err != nil {
		errln(tr("ОШИБКА: ", "ERROR: ") + err.Error())
		return 2
	}
	// Форматтер: E8 rel32 → SStrPrintf на каждом сайте.
	changed := false
	for _, va := range append(append([]uint32{}, dmgCallVAs...), healCallVA) {
		off, oerr := pe.vaToOffset(va)
		if oerr != nil {
			errln(tr("ОШИБКА: ", "ERROR: ") + fmtErr(tr(
				"сайт %08X: ", "site %08X: "), va).Error() + " — " + oerr.Error())
			return 2
		}
		if data[off] != 0xE8 && data[off] != 0xE9 {
			errln(tr("ОШИБКА: ", "ERROR: ") + fmtErr(tr(
				"по адресу %08X нет call — файл не похож на билд 12340",
				"no call at address %08X — the file does not match build 12340"), va).Error())
			return 2
		}
		if data[off] != 0xE8 {
			changed = true
		}
		data[off] = 0xE8
		binary.LittleEndian.PutUint32(data[off+1:],
			uint32(int32(sstrPrintfVA)-int32(va+5)))
	}
	// Цвет: mov eax/edi, 0xADA6C обратно.
	for _, sp := range colorSites {
		off, oerr := pe.vaToOffset(sp.va)
		if oerr != nil {
			errln(tr("ОШИБКА: ", "ERROR: ") + fmtErr(tr(
				"сайт %08X: ", "site %08X: "), sp.va).Error() + " — " + oerr.Error())
			return 2
		}
		if data[off] != sp.orig[0] && data[off] != 0xE9 {
			errln(tr("ОШИБКА: ", "ERROR: ") + fmtErr(tr(
				"по адресу %08X ни исходный mov, ни наш переход — файл не тронут нами.",
				"address %08X holds neither the original mov nor our jump — not our patch."), sp.va).Error())
			return 2
		}
		if !bytesEqual(data[off:off+5], sp.orig[:]) {
			changed = true
		}
		copy(data[off:off+5], sp.orig[:])
	}
	// Вшитый код — нулями.
	clean := true
	if off, _, ok := findOurCave(data, pe); ok {
		for i := off; i < off+caveSize; i++ {
			if data[i] != 0 {
				clean = false
			}
			data[i] = 0
		}
	}
	if !changed && clean {
		fmt.Println(tr("восстанавливать нечего — exe уже ванильный.",
			"nothing to restore — already a vanilla exe."))
		return 0
	}
	putChecksum(data, pe)
	if err := os.WriteFile(exe, data, 0o644); err != nil {
		errln(tr("ОШИБКА: не удалось записать файл: ", "ERROR: failed to save the file: ") + err.Error())
		return 3
	}
	if cwd, err := os.Getwd(); err == nil && removeAddon(cwd) {
		fmt.Println(tr("аддон dmg_format_patcher удалён.", "dmg_format_patcher addon removed."))
	}
	fmt.Println(tr(
		"восстановлено: вызовы SStrPrintf, жёлтый цвет спелл-урона, вшитый код очищен.",
		"restored: SStrPrintf calls, yellow spell damage color, injected code cleared."))
	return 0
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// writeWithBackup сохраняет бэкап один раз и записывает новый exe.
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

// ---------- аддон ----------

// doAddon пишет аддон в текущую папку и напоминает перенести в корень игры.
func doAddon(exe string, dmg, heal formatCfg, school bool, palette []schoolType) {
	cwd, err := os.Getwd()
	if err != nil {
		errln(tr("ОШИБКА: не определить текущую папку: ", "ERROR: cannot get current folder: ") + err.Error())
		return
	}
	path, err := writeAddon(cwd, dmg, heal, school, palette)
	if err != nil {
		errln(err.Error())
		return
	}
	printC(colorGreen, tr("аддон:       %s\n", "addon:       %s\n"), path)

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
	char    uint32 // ImageCharacteristics секции
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
			char:    binary.LittleEndian.Uint32(data[o+36:]),
		})
	}
	return info, nil
}

func (pe *peInfo) vaToOffset(va uint32) (int, error) {
	rva := va - pe.imageBase
	for i := range pe.sections {
		s := &pe.sections[i]
		if s.rawsize == 0 {
			continue // BSS
		}
		// только диапазон, реально лежащий в файле
		if rva >= s.vaddr && rva < s.vaddr+s.vsize && rva-s.vaddr < s.rawsize {
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

// resolveTarget — путь к Wow.exe: аргумент, текущая папка или рядом с exe.
func resolveTarget(args []string) string {
	if len(args) > 0 && args[0] != "" {
		return args[0]
	}
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
	errln(tr("ОШИБКА: Wow.exe не найден в текущей папке или рядом с патчером.\nУкажите путь к Wow.exe:",
		"ERROR: Wow.exe not found in the current folder or next to the patcher.\nSpecify the path to Wow.exe:"))
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

// readSchoolColors — читает цвета школ из вшитой секции пропатченного exe
// (блок W3CS): порядок цветов = knownMasks. ok=false, если блока нет.
func readSchoolColors(data []byte, pe *peInfo) (map[byte][3]uint8, bool) {
	out := map[byte][3]uint8{}
	off, _, ok := findOurCave(data, pe)
	if !ok {
		return out, false
	}
	base := int(off)
	if base+offSchMagic+4 > len(data) ||
		string(data[base+offSchMagic:base+offSchMagic+4]) != string(schMagic[:]) {
		return out, false
	}
	for i, m := range knownMasks {
		o := base + colorsOff + 4*i
		if o+4 > len(data) {
			break
		}
		v := binary.LittleEndian.Uint32(data[o:])
		out[m] = [3]uint8{uint8(v >> 16), uint8(v >> 8), uint8(v)}
	}
	return out, true
}
