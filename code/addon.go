package main

// addon.go — генерация аддона, форматирующего «свой» всплывающий текст.
//
// Урон и лечение самого себя рисует не exe, а аддон Blizzard_CombatText
// (LoadOnDemand): в CombatText_OnEvent сообщение собирается конкатенацией
// ("-"..data / "+"..arg3, Blizzard_CombatText.lua), и в exe его не перехватить.
// Поэтому генерируется мини-аддон, который перехватывает итоговую выдачу:
//
//	CombatText_AddMessage(message, scrollFunction, r, g, b, displayType, isStaggered)
//
// и переформатирует ведущее число сообщения: "-N" — формат урона,
// "+N" — формат лечения. Форматы запекаются в Lua константы при генерации
// (появление в мире и повторный запуск патчера обновляют их).
//
// Загрузка: ## LoadWith: Blizzard_CombatText — наш аддон подгружается
// сразу после подгрузки боевого текста, поэтому CombatText_AddMessage
// к моменту выполнения хука уже существует.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const addonFolder = "dmg_format_patcher"

// addonToc — toc-файл. LoadOnDemand+LoadWith: сам по себе аддон не грузится,
// ждёт подгрузки Blizzard_CombatText.
const addonToc = `## Interface: 30300
## Title: dmg_format_patcher
## Notes: Combat number formatting (K/M or dot separators) for self floating text
## Author: dmg_format_patcher
## LoadOnDemand: 1
## LoadWith: Blizzard_CombatText
dmg_format.lua
`

// addonLuaTmpl — шаблон хука (без fmt-экранирования: магию % в Lua-паттернах
// проще не пропускать через Sprintf). Плейсхолдеры заменяет strings.NewReplacer.
const addonLuaTmpl = `-- Сгенерировано dmg_format_patcher: формат цифр собственного
-- всплывающего текста. Обновляется повторным запуском патчера.
local DMG_MODE, DMG_DEC = {DMG_MODE}, {DMG_DEC}
local HEAL_MODE, HEAL_DEC = {HEAL_MODE}, {HEAL_DEC}

local function FormatNumber(n, mode, dec)
	if mode == 2 and n >= 10000 then
		local v, suffix
		if n >= 1000000 then
			v, suffix = n / 1000000, "M"
		else
			v, suffix = n / 1000, "K"
		end
		local s = string.format("%." .. dec .. "f", v)
		-- отбрасываем хвостовые нули: 111.10 -> 111.1, 111.00 -> 111
		s = string.gsub(s, "0*$", "")
		s = string.gsub(s, "%.$", "")
		return s .. suffix
	elseif mode == 1 then
		-- точки-разделители, справа налево: заякоренный повтор, иначе
		-- жадный %d+ ломает группы (1234567 -> 123.567)
		local s, k = tostring(n), 0
		repeat
			s, k = string.gsub(s, "^(%d+)(%d%d%d)", "%1.%2")
		until k == 0
		return s
	end
	return tostring(n)
end

local origAddMessage = CombatText_AddMessage
function CombatText_AddMessage(message, scrollFunction, r, g, b, displayType, isStaggered)
	local sign, num, rest = string.match(message, "^([-+])(%d+)(.*)$")
	if num then
		local n = tonumber(num) or 0
		if sign == "-" then
			message = "-" .. FormatNumber(n, DMG_MODE, DMG_DEC) .. rest
		else
			message = "+" .. FormatNumber(n, HEAL_MODE, HEAL_DEC) .. rest
		end
	end
	return origAddMessage(message, scrollFunction, r, g, b, displayType, isStaggered)
end
`

// addonLua заполняет шаблон выбранными форматами.
func addonLua(dmg, heal formatCfg) string {
	return strings.NewReplacer(
		"{DMG_MODE}", fmt.Sprint(int(dmg.mode)),
		"{DMG_DEC}", fmt.Sprint(int(clampDec(dmg.dec))),
		"{HEAL_MODE}", fmt.Sprint(int(heal.mode)),
		"{HEAL_DEC}", fmt.Sprint(int(clampDec(heal.dec))),
	).Replace(addonLuaTmpl)
}

// writeAddon создаёт аддон в dir\Interface\AddOns\dmg_format_patcher.
// Возвращает путь к папке аддона.
func writeAddon(dir string, dmg, heal formatCfg) (string, error) {
	base := filepath.Join(dir, "Interface", "AddOns", addonFolder)
	if err := os.MkdirAll(base, 0o755); err != nil {
		return "", fmtErr(tr(
			"не удалось создать папку аддона %s: %v",
			"failed to create addon folder %s: %v"), base, err)
	}
	toc := filepath.Join(base, addonFolder+".toc")
	lua := filepath.Join(base, "dmg_format.lua")
	if err := os.WriteFile(toc, []byte(addonToc), 0o644); err != nil {
		return base, fmtErr(tr(
			"не удалось записать %s: %v",
			"failed to write %s: %v"), toc, err)
	}
	if err := os.WriteFile(lua, []byte(addonLua(dmg, heal)), 0o644); err != nil {
		return base, fmtErr(tr(
			"не удалось записать %s: %v",
			"failed to write %s: %v"), lua, err)
	}
	return base, nil
}

// removeAddon удаляет наш аддон (для --restore). Чужие файлы в папке не трогаем:
// удаляем только знакомые два, папку — если после этого пусто.
func removeAddon(dir string) bool {
	base := filepath.Join(dir, "Interface", "AddOns", addonFolder)
	toc := filepath.Join(base, addonFolder+".toc")
	data, err := os.ReadFile(toc)
	if err != nil {
		return false
	}
	if !strings.Contains(string(data), "Title: "+addonFolder) {
		return false // не наш аддон — не трогаем
	}
	_ = os.Remove(toc)
	_ = os.Remove(filepath.Join(base, "dmg_format.lua"))
	if entries, err := os.ReadDir(base); err == nil && len(entries) == 0 {
		_ = os.Remove(base)
	}
	return true
}
