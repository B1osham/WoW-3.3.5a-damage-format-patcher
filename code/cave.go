package main

import "math"

// cave.go — машинный код, вшиваемый в секцию клиента, и мини-ассемблер
// для его генерации.
//
// До патча цвет спелл-урона выбирается одной командой mov eax/edi, 0xADA6C
// (адрес глобального жёлтого s_petSpellDamageColor), автоатаки идут мимо
// неё и остаются белыми. Вшитый стаб вместо этого читает SpellRec
// ([ebp+0x10], поле m_schoolMask на +0x284) и берёт цвет из своей таблицы
// через LUT по маске. Цвет упакован как 0xAARRGGBB (байты CImVector
// лежат в порядке b,g,r,a).

const (
	// адрес глобального жёлтого цвета — он же fallback при пустой маске
	yellowColorVA = 0x00ADA6C
)

// schoolType — маска школы → цвет (0xAARRGGBB), маски из DBC клиента.
type schoolType struct {
	mask  byte
	color uint32
}

// Цвета по маскам из таблицы школ 12340.
var schoolTypes = []schoolType{
	{0x01, argb(255, 255, 255)}, // SPELL_SCHOOL_NORMAL — белый
	{0x02, argb(255, 230, 120)}, // SPELL_SCHOOL_HOLY — золотистый «свет»
	{0x03, argb(255, 230, 120)}, // SPELL_SCHOOL_HOLYSTRIKE = holy
	{0x04, argb(255, 0, 0)},     // SPELL_SCHOOL_FIRE — красный
	{0x05, argb(255, 0, 0)},     // SPELL_SCHOOL_FLAMESTRIKE = fire
	{0x06, argb(255, 140, 60)},  // SPELL_SCHOOL_HOLYFIRE — огонь+свет: оранжевый
	{0x08, argb(0, 190, 0)},     // SPELL_SCHOOL_NATURE — тёмно-зелёный
	{0x09, argb(0, 190, 0)},     // SPELL_SCHOOL_STORMSTRIKE = nature
	{0x0A, argb(190, 255, 120)}, // SPELL_SCHOOL_HOLYSTORM — свет+природа
	{0x0C, argb(208, 255, 0)},   // SPELL_SCHOOL_FIRESTORM
	{0x10, argb(64, 130, 255)},  // SPELL_SCHOOL_FROST — мягкий голубой
	{0x11, argb(64, 130, 255)},  // SPELL_SCHOOL_FROSTSTRIKE = frost
	{0x14, argb(170, 0, 230)},   // SPELL_SCHOOL_FROSTFIRE — краски: красный+синий=фиолетовый
	{0x18, argb(0, 255, 128)},   // SPELL_SCHOOL_FROSTSTORM
	{0x1C, argb(170, 255, 160)}, // SPELL_SCHOOL_ELEMENTAL (три школы)
	{0x20, argb(185, 0, 171)},   // SPELL_SCHOOL_SHADOW — фиолетовый
	{0x21, argb(185, 0, 171)},   // SPELL_SCHOOL_SHADOWSTRIKE = shadow
	{0x22, argb(210, 140, 200)}, // SPELL_SCHOOL_TWILIGHT — тень+свет
	{0x24, argb(255, 0, 160)},   // SPELL_SCHOOL_SHADOWFLAME
	{0x28, argb(120, 180, 85)},  // SPELL_SCHOOL_PLAGUE
	{0x31, argb(92, 0, 213)},    // SPELL_SCHOOL_SHADOWFROST
	{0x40, argb(0, 255, 255)},   // SPELL_SCHOOL_ARCANE — бирюзовый
	{0x42, argb(255, 240, 170)}, // SPELL_SCHOOL_DIVINE — светлее holy
	{0x44, argb(255, 80, 190)},  // SPELL_SCHOOL_SPELLFIRE
	{0x48, argb(0, 255, 200)},   // SPELL_SCHOOL_SPELLSTORM
	{0x50, argb(0, 160, 255)},   // SPELL_SCHOOL_SPELLFROST
	{0x60, argb(92, 128, 213)},  // SPELL_SCHOOL_SPELLSHADOW
	{0x7E, argb(200, 160, 255)}, // SPELL_SCHOOL_MAGIC (все магии) — лавандовый
	{0x7F, argb(255, 0, 255)},   // SPELL_SCHOOL_CHAOS (все школы)
}

// argb — упаковка RGB в 0xAARRGGBB.
func argb(r, g, b uint32) uint32 {
	return 0xFF000000 | r<<16 | g<<8 | b
}

// lutMaskIndexes — таблица «маска → индекс цвета», 128 байт.
// Неизвестная маска даёт fallback.
func lutMaskIndexes(palette []schoolType) []byte {
	lut := make([]byte, 0x80)
	unknown := byte(len(palette)) // fallback
	for i := range lut {
		lut[i] = unknown
	}
	for i, st := range palette {
		lut[st.mask] = byte(i)
	}
	return lut
}

// Раскладка вшитой секции: конфиг и код форматтера, затем LUT и стабы цвета.
// Адреса в клиенте, нужные форматтеру: строки форматов и сам SStrPrintf.
const (
	fmtPlainVA   = 0x009E289C // "%d"
	fmtPlusVA    = 0x00A33A4C // "+%d"
	sstrPrintfVA = 0x0076F070
)

const (
	offModeDmg  = 0x000 // byte: режим чисел урона
	offDecDmg   = 0x001 // byte: знаков после точки (урон)
	offModeHeal = 0x002 // byte: режим чисел лечения
	offDecHeal  = 0x003 // byte: знаков после точки (лечение)
	offFmtMagic = 0x0F0 // "W3FM" — метка, что секция наша (форматтер)
	fmtCodeSlot = 0x200 // размер слота под одну копию форматтера
	offCodeDmg  = 0x100
	offCodeHeal = 0x300
	offSchMagic = 0x510 // "W3CS" — метка блока цвета школы
	colorsOff   = 0x520 // N dword цветов + fallback-цвет
	ptrTableOff = 0x5A0 // N+1 dword указателей на цвета (грузит стаб)
	lutOff      = 0x660 // 128 байт LUT «маска → индекс цвета»
	offStubA    = 0x6E0 // стаб для AddWorldDamageText (dest=eax)
	offStubB    = 0x740 // стаб для AddWorldText (dest=edi)
	caveSize    = 0x1000
)

// Метки для поиска своей секции при повторных запусках.
var (
	fmtMagic = [4]byte{'W', '3', 'F', 'M'}
	schMagic = [4]byte{'W', '3', 'C', 'S'}
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
	a.labels[name] = len(a.b)
}

// jmp — безусловный переход rel32.
func (a *asmBuf) jmp(label string) {
	a.emit(0xE9)
	a.fixups = append(a.fixups, asmFixup{at: len(a.b), label: label})
	a.emit(0, 0, 0, 0)
}

// jcc — условный переход rel32 (cc — байт после 0x0F: 0x84 je, 0x85 jne,
// 0x83 jae, 0x87 jnbe).
func (a *asmBuf) jcc(cc byte, label string) {
	a.emit(0x0F, cc)
	a.fixups = append(a.fixups, asmFixup{at: len(a.b), label: label})
	a.emit(0, 0, 0, 0)
}

// jccRel8 — короткий условный переход: cc — байт самой команды (74 = je,
// 77 = ja). Все переходы в этом вшитом коде короткие.
func (a *asmBuf) jccRel8(cc byte, label string) {
	a.emit(cc)
	a.fixups = append(a.fixups, asmFixup{at: len(a.b), label: label, rel8: true})
	a.emit(0)
}

func (a *asmBuf) resolve() ([]byte, error) {
	for _, f := range a.fixups {
		t, ok := a.labels[f.label]
		if !ok {
			return nil, asmError{"метка не найдена: " + f.label}
		}
		if f.rel8 {
			rel := t - (f.at + 1)
			if rel < -128 || rel > 127 {
				return nil, asmError{"переход дальше -128..127: " + f.label}
			}
			a.b[f.at] = byte(rel)
			continue
		}
		rel := int64(t) - int64(f.at+4)
		v := uint32(rel)
		a.b[f.at] = byte(v)
		a.b[f.at+1] = byte(v >> 8)
		a.b[f.at+2] = byte(v >> 16)
		a.b[f.at+3] = byte(v >> 24)
	}
	return a.b, nil
}

type asmError struct{ msg string }

func (e asmError) Error() string { return e.msg }

// genStub — стаб для одного сайта: dest — регистр цвета (0xB8 eax /
// 0xBF edi), backVA — адрес возврата, ptrTableVA/lutVA — таблицы в
// секции, stubVA — адрес стаба (для rel32).
func genStub(dest byte, backVA, ptrTableVA, lutVA, stubVA uint32) ([]byte, error) {
	// mov r32, [ebp+0x10] — spell. ModRM: reg=0 (eax) или 7 (edi).
	var dstIdx byte = 0
	if dest != 0xB8 {
		dstIdx = 7
	}
	a := newAsm()
	// mov dest, [ebp+0x10]
	a.emit(0x8B, 0x45|(dstIdx<<3), 0x10)
	// test dest, dest
	a.emit(0x85, 0xC0+(dstIdx<<3)+dstIdx)
	a.jccRel8(0x74, "def")
	// mov dest, [dest+0x284] — schoolMask
	a.emit(0x8B, 0x80|(dstIdx<<3)|dstIdx, 0x84, 0x02, 0x00, 0x00)
	// and dest, 0x7F
	a.emit(0x83, 0xE0|dstIdx, 0x7F)
	a.jccRel8(0x74, "def")
	// movzx eax, byte [dest + lutVA] — индекс цвета по маске (LUT)
	a.emit(0x0F, 0xB6, 0x80|dstIdx)
	a.imm32(lutVA)
	// mov dest, [eax*4 + ptrTableVA] — указатель на цвет, не значение
	a.emit(0x8B, 0x04|(dstIdx<<3), 0x85)
	a.imm32(ptrTableVA)
	a.jmp("back")
	a.label("def")
	// mov dest, 0xADA6C — original yellow fallback
	a.emit(dest, 0x6C, 0xAA, 0xAD, 0x00)
	a.label("back")
	// rel32 до backVA: здесь() даёт смещение внутри стаба, конец команды
	// = stubVA + here() + 5.
	at := hereAt(a, stubVA)
	a.emit(0xE9)
	a.imm32(uint32(int64(backVA) - int64(at+5)))
	return a.resolve()
}

// hereAt — абсолютный VA локального смещения в ленте стаба.
func hereAt(a *asmBuf, stubVA uint32) uint32 {
	return stubVA + uint32(a.here())
}

// buildCaveImage — образ секции: конфиг + две копии форматтера + блок
// школы (цвета, указатели на них, LUT) + два стаба.
func buildCaveImage(dmgMode, dmgDec, healMode, healDec byte, withSchool bool, caveVA uint32, palette []schoolType) ([]byte, error) {
	img := make([]byte, caveSize)
	img[offModeDmg], img[offDecDmg] = dmgMode, dmgDec
	img[offModeHeal], img[offDecHeal] = healMode, healDec
	copy(img[offFmtMagic:], fmtMagic[:])

	dmg, err := genFormatter(caveVA+offModeDmg, caveVA+offDecDmg)
	if err != nil {
		return nil, err
	}
	heal, err := genFormatter(caveVA+offModeHeal, caveVA+offDecHeal)
	if err != nil {
		return nil, err
	}
	if len(dmg) > fmtCodeSlot || len(heal) > fmtCodeSlot {
		return nil, asmError{"код форматтера не влезает в слот"}
	}
	copy(img[offCodeDmg:], dmg)
	copy(img[offCodeHeal:], heal)

	if withSchool {
		copy(img[offSchMagic:], schMagic[:])
		n := len(palette) + 1 // + fallback-указатель на жёлтый глобал
		if colorsOff+4*n > ptrTableOff || ptrTableOff+4*n > lutOff || lutOff+0x80 > offStubA {
			return nil, asmError{"блок школы не влезает в слоты секции"}
		}
		// значения цветов и указатели на них
		for i, st := range palette {
			imm32At(img, colorsOff+4*i, st.color)
			imm32At(img, ptrTableOff+4*i, caveVA+colorsOff+uint32(4*i))
		}
		// fallback: указа тельна глобальный жёлтый самого клиента.
		imm32At(img, colorsOff+4*n-4, 0xFFFFDE00)
		imm32At(img, ptrTableOff+4*n-4, yellowColorVA)
		// LUT: маска → индекс цвета (fallback = последний индекс).
		copy(img[lutOff:], lutMaskIndexes(palette))
		aCode, err := genStub(0xB8, backDamageVA, caveVA+ptrTableOff, caveVA+lutOff, caveVA+offStubA)
		if err != nil {
			return nil, err
		}
		bCode, err := genStub(0xBF, backWorldVA, caveVA+ptrTableOff, caveVA+lutOff, caveVA+offStubB)
		if err != nil {
			return nil, err
		}
		if len(aCode) > 0x60 || len(bCode) > 0x60 {
			return nil, asmError{"стаб школы не влезает в слот"}
		}
		copy(img[offStubA:], aCode)
		copy(img[offStubB:], bCode)
	}
	return img, nil
}

// findOurCave ищет вшитую секцию по меткам W3FM (форматтер) или W3CS
// (школа): любую из них считаем нашей.
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
		if string(data[off+offFmtMagic:off+offFmtMagic+4]) == string(fmtMagic[:]) ||
			string(data[off+offSchMagic:off+offSchMagic+4]) == string(schMagic[:]) {
			return off, imageBase + s.vaddr, true
		}
	}
	return 0, 0, false
}

// ---------- форматтер цифр (из dmg_format_patcher) ----------

func genFormatter(cfgModeVA, cfgDecVA uint32) ([]byte, error) {
	a := newAsm()

	// Пролог и сохранение аргументов.
	a.emit(0x55)             // push ebp
	a.emit(0x8B, 0xEC)       // mov ebp, esp
	a.emit(0x53, 0x56, 0x57) // push ebx; push esi; push edi
	a.emit(0x83, 0xEC, 0x20) // sub esp, 0x20
	a.emit(0x8B, 0x45, 0x10) // mov eax, [ebp+0x10]  ; fmt
	a.emit(0x8B, 0x75, 0x14) // mov esi, [ebp+0x14]  ; value

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
	a.emit(0x8B, 0xC6)                   // mov eax, esi
	a.emit(0x33, 0xD2)                   // xor edx, edx
	a.emit(0xB9, 0x0A, 0x00, 0x00, 0x00) // mov ecx, 10
	a.emit(0xF7, 0xF1)                   // div ecx
	a.emit(0x8B, 0xF0)                   // mov esi, eax
	a.emit(0x80, 0xC2, 0x30)             // add dl, '0'
	a.emit(0x4F)                         // dec edi
	a.emit(0x88, 0x17)                   // mov [edi], dl
	a.emit(0x85, 0xF6)                   // test esi, esi
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
	a.emit(0x8B, 0xC6)                   // mov eax, esi
	a.emit(0x33, 0xD2)                   // xor edx, edx
	a.emit(0xB9, 0x0A, 0x00, 0x00, 0x00) // mov ecx, 10
	a.emit(0xF7, 0xF1)                   // div ecx
	a.emit(0x8B, 0xF0)                   // mov esi, eax
	a.emit(0x80, 0xC2, 0x30)             // add dl, '0'
	a.emit(0x4F)                         // dec edi
	a.emit(0x88, 0x17)                   // mov [edi], dl
	a.emit(0x43)                         // inc ebx
	a.emit(0x85, 0xF6)                   // test esi, esi
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
	a.emit(0xC6, 0x44, 0x24, 0x0D, 'M')  // mov byte [esp+0xD], 'M'
	a.emit(0xB9, 0x40, 0x42, 0x0F, 0x00) // mov ecx, 1000000
	a.jmp("doLetters")
	a.label("doK")
	a.emit(0xC6, 0x44, 0x24, 0x0D, 'K')  // mov byte [esp+0xD], 'K'
	a.emit(0xB9, 0xE8, 0x03, 0x00, 0x00) // mov ecx, 1000
	a.label("doLetters")
	a.emit(0x8B, 0xC6)             // mov eax, esi
	a.emit(0x33, 0xD2)             // xor edx, edx
	a.emit(0xF7, 0xF1)             // div ecx  ; eax = целая часть, edx = остаток
	a.emit(0x89, 0x44, 0x24, 0x04) // mov [esp+4], eax
	a.emit(0x8B, 0xC2)             // mov eax, edx  ; остаток
	a.emit(0x33, 0xDB)             // xor ebx, ebx  ; j = 0

	// Дробные цифры: digit_j = (rem*10^j / D) % 10, считаются подряд.
	// Слоты: cur — [esp+0x18] (dword), он живёт только внутри цикла; строка
	// при выводе дорастает вниз максимум до [esp+0x15], уже после цикла.
	a.label("frac")
	a.emit(0x89, 0x44, 0x24, 0x18) // mov [esp+0x18], eax  ; cur
	a.label("fracLoop")
	a.emit(0xB8, 0x0A, 0x00, 0x00, 0x00) // mov eax, 10
	a.emit(0xF7, 0x64, 0x24, 0x18)       // mul dword [esp+0x18] ; edx:eax = cur*10
	a.emit(0xF7, 0xF1)                   // div ecx            ; eax = цифра, edx = остаток
	a.emit(0x88, 0x44, 0x1C, 0x0E)       // mov [esp+ebx+0xE], al
	a.emit(0x89, 0x54, 0x24, 0x18)       // mov [esp+0x18], edx ; cur
	a.emit(0x43)                         // inc ebx
	a.emit(0x0F, 0xB6, 0x15)             // movzx edx, byte [cfgDecVA]
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
	a.emit(0x4F)                   // dec edi
	a.emit(0x8A, 0x44, 0x24, 0x0D) // mov al, [esp+0xD]  ; суффикс
	a.emit(0x88, 0x07)             // mov [edi], al
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
	a.emit(0x8B, 0xC6)                   // mov eax, esi
	a.emit(0x33, 0xD2)                   // xor edx, edx
	a.emit(0xB9, 0x0A, 0x00, 0x00, 0x00) // mov ecx, 10
	a.emit(0xF7, 0xF1)                   // div ecx
	a.emit(0x8B, 0xF0)                   // mov esi, eax
	a.emit(0x80, 0xC2, 0x30)             // add dl, '0'
	a.emit(0x4F)                         // dec edi
	a.emit(0x88, 0x17)                   // mov [edi], dl
	a.emit(0x85, 0xF6)                   // test esi, esi
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
	a.emit(0x83, 0xC4, 0x20)       // add esp, 0x20
	a.emit(0x5F, 0x5E, 0x5B, 0x5D) // pop edi; pop esi; pop ebx; pop ebp
	a.emit(0xC3)                   // ret

	return a.resolve()
}

// imm32At пишет little-endian dword в img по смещению off.
func imm32At(img []byte, off int, v uint32) {
	img[off] = byte(v)
	img[off+1] = byte(v >> 8)
	img[off+2] = byte(v >> 16)
	img[off+3] = byte(v >> 24)
}

// knownMasks — все осмысленные маски школ (таблица SPELL_SCHOOL клиента).
var knownMasks = []byte{
	0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x08, 0x09, 0x0A, 0x0C, 0x10,
	0x11, 0x14, 0x18, 0x1C, 0x20, 0x21, 0x22, 0x24, 0x28, 0x31,
	0x40, 0x42, 0x44, 0x48, 0x50, 0x60, 0x7E, 0x7F,
}

// blendPalette собирает палитру из базовых цветов школ (ключи — маски
// 0x01..0x40 и 0x7F для chaos): одиночная маска — свой цвет, сочетание
// школ — смешивание «как красками» (paintMix) из цветов базовых школ,
// физика в цвете сочетания не участвует.
func blendPalette(base map[byte][3]uint8) []schoolType {
	// «все магии» — фиксированная лаванда: круговое смешивание шести
	// оттенков даёт грязный цвет
	curated := map[byte]uint32{}
	for _, st := range schoolTypes {
		curated[st.mask] = st.color
	}
	out := make([]schoolType, 0, len(knownMasks))
	for _, m := range knownMasks {
		var col [3]uint8
		switch {
		case m == 0x7F:
			c := base[0x7F]
			col = [3]uint8{c[0], c[1], c[2]}
		case m&0x7E == 0:
			c := base[0x01]
			col = [3]uint8{c[0], c[1], c[2]}
		default:
			magic := m & 0x7E
			var cols [][3]uint8
			for b := byte(0x02); b != 0; b <<= 1 {
				if magic&b != 0 {
					cols = append(cols, base[b])
				}
			}
			if len(cols) == 1 {
				col = cols[0]
			} else if m == 0x7E {
				cc := curated[m]
				col = [3]uint8{uint8(cc >> 16), uint8(cc >> 8), uint8(cc)}
			} else {
				col = paintMix(cols)
			}
		}
		out = append(out, schoolType{m, argb(uint32(col[0]), uint32(col[1]), uint32(col[2]))})
	}
	return out
}

// paintMix — смешение «как красками»: оттенки усредняются по кругу
// (жёлтый+синий = зелёный, красный+синий = фиолетовый, а не серое
// среднее). Насыщенность и яркость смеси — средние, взятые с весом
// насыщенности: серые и очень тёмные цвета не тянут оттенок на себя.
func paintMix(cols [][3]uint8) [3]uint8 {
	var sx, sy, wSum, sSum, vSum float64
	for _, c := range cols {
		h, s, v := rgbToHsv(c[0], c[1], c[2])
		w := s * v
		rad := h * math.Pi / 180
		sx += w * math.Cos(rad)
		sy += w * math.Sin(rad)
		wSum += w
		sSum += s * w
		vSum += v * w
	}
	if wSum == 0 {
		return [3]uint8{128, 128, 128}
	}
	h := math.Atan2(sy/wSum, sx/wSum) * 180 / math.Pi
	if h < 0 {
		h += 360
	}
	r, g, b := hsvToRgb(h, sSum/wSum, vSum/wSum)
	return [3]uint8{r, g, b}
}

// rgbToHsv — RGB (0..255) → оттенок в градусах, s/v в 0..1.
func rgbToHsv(r, g, b uint8) (h, s, v float64) {
	fr, fg, fb := float64(r)/255, float64(g)/255, float64(b)/255
	max := math.Max(fr, math.Max(fg, fb))
	min := math.Min(fr, math.Min(fg, fb))
	v = max
	if max == 0 {
		return 0, 0, 0
	}
	s = (max - min) / max
	if s == 0 {
		return 0, 0, v
	}
	d := max - min
	switch max {
	case fr:
		h = math.Mod((fg-fb)/d, 6)
	case fg:
		h = (fb-fr)/d + 2
	default:
		h = (fr-fg)/d + 4
	}
	h *= 60
	if h < 0 {
		h += 360
	}
	return h, s, v
}

// hsvToRgb — обратное преобразование rgbToHsv.
func hsvToRgb(h, s, v float64) (r, g, b uint8) {
	c := v * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := v - c
	var fr, fg, fb float64
	switch {
	case h < 60:
		fr, fg, fb = c, x, 0
	case h < 120:
		fr, fg, fb = x, c, 0
	case h < 180:
		fr, fg, fb = 0, c, x
	case h < 240:
		fr, fg, fb = 0, x, c
	case h < 300:
		fr, fg, fb = x, 0, c
	default:
		fr, fg, fb = c, 0, x
	}
	return uint8((fr + m) * 255), uint8((fg + m) * 255), uint8((fb + m) * 255)
}
