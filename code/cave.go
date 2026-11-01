package main

// cave.go — машинный код функции-форматтера, вшиваемой в свободную секцию
// клиента, и мини-ассемблер для его генерации с вычислением переходов.
//
// Форматтер подменяет вызовы SStrPrintf(buf, 0x20, "%d"|"+%d", value) в
// функциях летающего боевого текста (урон и лечение). Сигнатура cdecl:
//
//	void fmtFunc(char* buf, size_t size, const char* fmt, uint32 value)
//
// buf=[ebp+8], size=[ebp+0xC], fmt=[ebp+0x10], value=[ebp+0x14].
// Префикс '+' ставится, когда fmt == 0x00A33A4C ("+%d": крит/поглощение),
// как в исходном патче с точками-разделителями.
//
// Режим (байт cfgModeVA): 0 — обычные цифры, 1 — 1.234.567, 2 — 111.11K/111.11M.
// В режиме 2 используется cfgDecVA — знаков после точки (1..3), нули
// в конце отбрасываются (111.10K → 111.1K, 111.00K → 111K).
// Пороги: ≥10 000 — K, ≥1 000 000 — M, меньше — обычные цифры.

const (
	fmtPlainVA = 0x009E289C // "%d"
	fmtPlusVA  = 0x00A33A4C // "+%d"
	sstrPrintfVA = 0x0076F070
)

// asmBuf — лента кода с метками и rel32-фикапами.
type asmBuf struct {
	b      []byte
	labels map[string]int
	fixups []asmFixup
}

type asmFixup struct {
	at    int
	label string
	rel8  bool
}

func newAsm() *asmBuf { return &asmBuf{labels: map[string]int{}} }

func (a *asmBuf) here() int { return len(a.b) }

func (a *asmBuf) emit(bs ...byte) { a.b = append(a.b, bs...) }

func (a *asmBuf) imm32(v uint32) {
	a.b = append(a.b, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
}

func (a *asmBuf) label(name string) {
	if _, dup := a.labels[name]; dup {
		panic("duplicate label " + name)
	}
	a.labels[name] = len(a.b)
}

// jmp / jcc — переходы rel32 (op — код после 0x0F для условных).
func (a *asmBuf) jmp(label string) {
	a.emit(0xE9)
	a.fixups = append(a.fixups, asmFixup{at: len(a.b), label: label})
	a.emit(0, 0, 0, 0)
}

func (a *asmBuf) jcc(cc byte, label string) {
	a.emit(0x0F, cc)
	a.fixups = append(a.fixups, asmFixup{at: len(a.b), label: label})
	a.emit(0, 0, 0, 0)
}

// jccRel8 — короткий условный переход (для плотного хвоста copy).
func (a *asmBuf) jccRel8(cc byte, label string) {
	a.emit(cc)
	a.fixups = append(a.fixups, asmFixup{at: len(a.b), label: label, rel8: true})
	a.emit(0)
}

func (a *asmBuf) resolve() ([]byte, error) {
	for _, f := range a.fixups {
		t, ok := a.labels[f.label]
		if !ok {
			return nil, &asmError{"метка не найдена: " + f.label}
		}
		if f.rel8 {
			rel := t - (f.at + 1)
			if rel < -128 || rel > 127 {
				return nil, &asmError{"переход дальше -128..127 до " + f.label}
			}
			a.b[f.at] = byte(rel)
			continue
		}
		rel := int64(t) - int64(f.at+4)
		if rel < -(1<<31) || rel > (1<<31)-1 {
			return nil, &asmError{"переход не влезает в rel32 до " + f.label}
		}
		v := uint32(rel)
		a.b[f.at] = byte(v)
		a.b[f.at+1] = byte(v >> 8)
		a.b[f.at+2] = byte(v >> 16)
		a.b[f.at+3] = byte(v >> 24)
	}
	return a.b, nil
}

type asmError struct{ msg string }

func (e *asmError) Error() string { return e.msg }

// genFormatter собирает функцию-форматтер. cfgModeVA/cfgDecVA — адреса
// байтов конфигурации (режим и знаков после точки) для этой копии функции.
func genFormatter(cfgModeVA, cfgDecVA uint32) ([]byte, error) {
	a := newAsm()

	// Пролог и сохранение аргументов.
	a.emit(0x55)                   // push ebp
	a.emit(0x8B, 0xEC)             // mov ebp, esp
	a.emit(0x53, 0x56, 0x57)       // push ebx; push esi; push edi
	a.emit(0x83, 0xEC, 0x20)       // sub esp, 0x20
	a.emit(0x8B, 0x45, 0x10)       // mov eax, [ebp+0x10]  ; fmt
	a.emit(0x8B, 0x75, 0x14)       // mov esi, [ebp+0x14]  ; value

	// Флаг '+' (крит/поглощение) — в байте [esp], буфер строки растёт
	// вниз от [esp+0x1F] и до него не доходит (максимум 15 символов).
	a.emit(0x3D) // cmp eax, fmtPlusVA
	a.imm32(fmtPlusVA)
	a.jcc(0x85, "notcrit")
	a.emit(0xC6, 0x04, 0x24, 0x01) // mov byte [esp], 1
	a.jmp("flagdone")
	a.label("notcrit")
	a.emit(0xC6, 0x04, 0x24, 0x00) // mov byte [esp], 0
	a.label("flagdone")

	a.emit(0x8D, 0x7C, 0x24, 0x1F) // lea edi, [esp+0x1F]
	a.emit(0xC6, 0x07, 0x00)       // mov byte [edi], 0    ; хвост строки

	// Диспетчер режимов.
	a.emit(0x0F, 0xB6, 0x05) // movzx eax, byte [cfgModeVA]
	a.imm32(cfgModeVA)
	a.emit(0x85, 0xC0) // test eax, eax
	a.jcc(0x84, "plain")
	a.emit(0x83, 0xF8, 0x01) // cmp eax, 1
	a.jcc(0x84, "dots")
	a.jmp("letters")

	// --- режим 0: обычные цифры ---
	a.label("plain")
	a.emit(0x85, 0xF6) // test esi, esi
	a.jcc(0x85, "plainLoop")
	a.emit(0x4F)             // dec edi
	a.emit(0xC6, 0x07, 0x30) // mov byte [edi], '0'
	a.jmp("postnum")
	a.label("plainLoop")
	a.emit(0x8B, 0xC6)       // mov eax, esi
	a.emit(0x33, 0xD2)       // xor edx, edx
	a.emit(0xB9, 0x0A, 0x00, 0x00, 0x00) // mov ecx, 10
	a.emit(0xF7, 0xF1)       // div ecx
	a.emit(0x8B, 0xF0)       // mov esi, eax
	a.emit(0x80, 0xC2, 0x30) // add dl, '0'
	a.emit(0x4F)             // dec edi
	a.emit(0x88, 0x17)       // mov [edi], dl
	a.emit(0x85, 0xF6)       // test esi, esi
	a.jcc(0x85, "plainLoop")
	a.jmp("postnum")

	// --- режим 1: 1.234.567 (как исходный dots-патч) ---
	a.label("dots")
	a.emit(0x33, 0xDB) // xor ebx, ebx — счётчик цифр с чистого листа
	a.emit(0x85, 0xF6) // test esi, esi
	a.jcc(0x85, "dotsLoop")
	a.emit(0x4F)             // dec edi
	a.emit(0xC6, 0x07, 0x30) // mov byte [edi], '0'
	a.jmp("postnum")
	a.label("dotsLoop")
	a.emit(0x8B, 0xC6)       // mov eax, esi
	a.emit(0x33, 0xD2)       // xor edx, edx
	a.emit(0xB9, 0x0A, 0x00, 0x00, 0x00) // mov ecx, 10
	a.emit(0xF7, 0xF1)       // div ecx
	a.emit(0x8B, 0xF0)       // mov esi, eax
	a.emit(0x80, 0xC2, 0x30) // add dl, '0'
	a.emit(0x4F)             // dec edi
	a.emit(0x88, 0x17)       // mov [edi], dl
	a.emit(0x43)             // inc ebx
	a.emit(0x85, 0xF6)       // test esi, esi
	a.jcc(0x84, "postnum")
	a.emit(0x83, 0xFB, 0x03) // cmp ebx, 3
	a.jcc(0x85, "dotsLoop")
	a.emit(0x4F)             // dec edi
	a.emit(0xC6, 0x07, 0x2E) // mov byte [edi], '.'
	a.emit(0x33, 0xDB)       // xor ebx, ebx
	a.jmp("dotsLoop")

	// --- режим 2: 111.11K / 111.11M ---
	// Слоты в стеке: [esp] флаг '+', [esp+4] целая часть, [esp+8] остаток,
	// [esp+0xB] счётчик дробных цифр, [esp+0xC].., [esp+0xD] суффикс,
	// [esp+0xE..0x10] дробные цифры (1..3).
	a.label("letters")
	a.emit(0x81, 0xFE) // cmp esi, 1000000
	a.imm32(1000000)
	a.jcc(0x83, "doM")
	a.emit(0x81, 0xFE) // cmp esi, 10000
	a.imm32(10000)
	a.jcc(0x83, "doK")
	a.jmp("plain") // < 10000 — обычные цифры
	a.label("doM")
	a.emit(0xC6, 0x44, 0x24, 0x0D, 'M') // mov byte [esp+0xD], 'M'
	a.emit(0xB9, 0x40, 0x42, 0x0F, 0x00) // mov ecx, 1000000
	a.jmp("doLetters")
	a.label("doK")
	a.emit(0xC6, 0x44, 0x24, 0x0D, 'K') // mov byte [esp+0xD], 'K'
	a.emit(0xB9, 0xE8, 0x03, 0x00, 0x00) // mov ecx, 1000
	a.label("doLetters")
	a.emit(0x8B, 0xC6)       // mov eax, esi
	a.emit(0x33, 0xD2)       // xor edx, edx
	a.emit(0xF7, 0xF1)       // div ecx  ; eax = целая часть, edx = остаток
	a.emit(0x89, 0x44, 0x24, 0x04) // mov [esp+4], eax
	a.emit(0x8B, 0xC2)       // mov eax, edx  ; остаток
	a.emit(0x33, 0xDB)       // xor ebx, ebx  ; j = 0

	// Дробные цифры: digit_j = (rem*10^j / D) % 10, считаются подряд.
	// Слоты: cur — [esp+0x18] (dword), он живёт только внутри цикла; строка
	// при выводе дорастает вниз максимум до [esp+0x15], уже после цикла.
	a.label("frac")
	a.emit(0x89, 0x44, 0x24, 0x18) // mov [esp+0x18], eax  ; cur
	a.label("fracLoop")
	a.emit(0xB8, 0x0A, 0x00, 0x00, 0x00) // mov eax, 10
	a.emit(0xF7, 0x64, 0x24, 0x18) // mul dword [esp+0x18] ; edx:eax = cur*10
	a.emit(0xF7, 0xF1)             // div ecx            ; eax = цифра, edx = остаток
	a.emit(0x88, 0x44, 0x1C, 0x0E) // mov [esp+ebx+0xE], al
	a.emit(0x89, 0x54, 0x24, 0x18) // mov [esp+0x18], edx ; cur
	a.emit(0x43)                   // inc ebx
	a.emit(0x0F, 0xB6, 0x15)       // movzx edx, byte [cfgDecVA]
	a.imm32(cfgDecVA)
	a.emit(0x3B, 0xDA) // cmp ebx, edx
	a.jcc(0x83, "fracDone")
	a.jmp("fracLoop")

	// Отбрасываем хвостовые нули (111.10K → 111.1K).
	a.label("fracDone")
	a.label("trim")
	a.emit(0x85, 0xDB) // test ebx, ebx
	a.jcc(0x84, "trimDone")
	a.emit(0x80, 0x7C, 0x1C, 0x0D, 0x00) // cmp byte [esp+ebx+0xD], 0
	a.jcc(0x85, "trimDone")
	a.emit(0x4B) // dec ebx
	a.jmp("trim")
	a.label("trimDone")
	a.emit(0x88, 0x5C, 0x24, 0x0B) // mov [esp+0xB], bl

	// Пишем строку задом наперёд: суффикс, дробные цифры от младшей,
	// точка, целая часть.
	a.emit(0x4F)             // dec edi
	a.emit(0x8A, 0x44, 0x24, 0x0D) // mov al, [esp+0xD]  ; суффикс
	a.emit(0x88, 0x07)       // mov [edi], al
	a.label("fracEmit")
	a.emit(0x85, 0xDB) // test ebx, ebx
	a.jcc(0x84, "fracEmitDone")
	a.emit(0x8A, 0x44, 0x1C, 0x0D) // mov al, [esp+ebx+0xD]
	a.emit(0x04, 0x30)             // add al, '0'
	a.emit(0x4F)                   // dec edi
	a.emit(0x88, 0x07)             // mov [edi], al
	a.emit(0x4B)                   // dec ebx
	a.jmp("fracEmit")
	a.label("fracEmitDone")
	a.emit(0x80, 0x7C, 0x24, 0x0B, 0x00) // cmp byte [esp+0xB], 0
	a.jcc(0x84, "noDot")
	a.emit(0x4F)             // dec edi
	a.emit(0xC6, 0x07, 0x2E) // mov byte [edi], '.'
	a.label("noDot")
	a.emit(0x8B, 0x74, 0x24, 0x04) // mov esi, [esp+4]  ; целая часть
	a.emit(0x85, 0xF6)             // test esi, esi
	a.jcc(0x85, "intLoop")
	a.emit(0x4F)             // dec edi
	a.emit(0xC6, 0x07, 0x30) // mov byte [edi], '0'
	a.jmp("postnum")
	a.label("intLoop")
	a.emit(0x8B, 0xC6)       // mov eax, esi
	a.emit(0x33, 0xD2)       // xor edx, edx
	a.emit(0xB9, 0x0A, 0x00, 0x00, 0x00) // mov ecx, 10
	a.emit(0xF7, 0xF1)       // div ecx
	a.emit(0x8B, 0xF0)       // mov esi, eax
	a.emit(0x80, 0xC2, 0x30) // add dl, '0'
	a.emit(0x4F)             // dec edi
	a.emit(0x88, 0x17)       // mov [edi], dl
	a.emit(0x85, 0xF6)       // test esi, esi
	a.jcc(0x85, "intLoop")

	// '+' для критов/поглощения и копирование в buf.
	a.label("postnum")
	a.emit(0x80, 0x3C, 0x24, 0x00) // cmp byte [esp], 0
	a.jcc(0x84, "copy")
	a.emit(0x4F)             // dec edi
	a.emit(0xC6, 0x07, 0x2B) // mov byte [edi], '+'
	a.label("copy")
	a.emit(0x8B, 0x45, 0x08) // mov eax, [ebp+8]  ; buf
	a.label("copyLoop")
	a.emit(0x8A, 0x0F) // mov cl, [edi]
	a.emit(0x88, 0x08) // mov [eax], cl
	a.emit(0x47)       // inc edi
	a.emit(0x40)       // inc eax
	a.emit(0x84, 0xC9) // test cl, cl
	a.jccRel8(0x75, "copyLoop")
	a.emit(0x83, 0xC4, 0x20) // add esp, 0x20
	a.emit(0x5F, 0x5E, 0x5B, 0x5D) // pop edi; pop esi; pop ebx; pop ebp
	a.emit(0xC3)             // ret

	return a.resolve()
}

// Смещения внутри вшитой секции: байты конфигурации и две копии кода
// (урон и лечение с независимыми настройками).
const (
	offModeDmg  = 0x00 // byte: режим чисел урона
	offDecDmg   = 0x01 // byte: знаков после точки (урон)
	offModeHeal = 0x02 // byte: режим чисел лечения
	offDecHeal  = 0x03 // byte: знаков после точки (лечение)
	offMagic    = 0xF0 // "W3FM" — метка, что секция наша
	offCodeDmg  = 0x100
	offCodeHeal = 0x300
	codeSlot    = 0x200 // размер слота под одну копию кода
	caveSize    = 0x1000
)

var caveMagic = [4]byte{'W', '3', 'F', 'M'}

// buildCaveImage собирает содержимое всей вшитой секции.
func buildCaveImage(dmgMode, dmgDec, healMode, healDec byte, caveVA uint32) ([]byte, error) {
	img := make([]byte, caveSize)
	img[offModeDmg], img[offDecDmg] = dmgMode, dmgDec
	img[offModeHeal], img[offDecHeal] = healMode, healDec
	copy(img[offMagic:], caveMagic[:])

	dmg, err := genFormatter(caveVA+offModeDmg, caveVA+offDecDmg)
	if err != nil {
		return nil, err
	}
	heal, err := genFormatter(caveVA+offModeHeal, caveVA+offDecHeal)
	if err != nil {
		return nil, err
	}
	if len(dmg) > codeSlot || len(heal) > codeSlot {
		return nil, &asmError{"код форматтера не влезает в слот"}
	}
	copy(img[offCodeDmg:], dmg)
	copy(img[offCodeHeal:], heal)
	return img, nil
}

// relocateDword заменяет в коде imm32 fromVA на toVA (адреса конфига при
// переносе образа на другой базовый адрес — используется тестами).
func relocateDword(code []byte, fromVA, toVA uint32) {
	pat := []byte{byte(fromVA), byte(fromVA >> 8), byte(fromVA >> 16), byte(fromVA >> 24)}
	rep := []byte{byte(toVA), byte(toVA >> 8), byte(toVA >> 16), byte(toVA >> 24)}
	for i := 0; i+4 <= len(code); i++ {
		if code[i] == pat[0] && code[i+1] == pat[1] && code[i+2] == pat[2] && code[i+3] == pat[3] {
			copy(code[i:], rep)
		}
	}
}
