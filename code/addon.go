package main

// addon.go — генерация аддона для «своего» всплывающего текста.
//
// Урон/лечение самого себя рисует аддон Blizzard_CombatText, в exe это
// не перехватить. Генерируемый аддон хукает CombatText_AddMessage:
// переформатирует число ("-N" урон, "+N" лечение) и при включённой
// школе подкрашивает урон — школу берёт из кэша последних
// COMBAT_LOG_EVENT_UNFILTERED-уронов по игроку (совпадение по сумме:
// в COMBAT_TEXT_UPDATE школы нет).

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const addonFolder = "dmg_format_patcher"

const addonToc = `## Interface: 30300
## Title: dmg_format_patcher
## Notes: Self floating text — number formats and spell-school colors
## Author: dmg_format_patcher
## LoadOnDemand: 1
## LoadWith: Blizzard_CombatText
dmg_format.lua
`

// addonLuaTmpl — шаблон хука, {PLACES} заменяются при генерации.
const addonLuaTmpl = `-- Сгенерировано school_colors_patcher: формат цифр собственного
-- всплывающего текста и их цвет по школе заклинания. Обновляется
-- повторным запуском патчера.
local DMG_MODE, DMG_DEC = {DMG_MODE}, {DMG_DEC}
local HEAL_MODE, HEAL_DEC = {HEAL_MODE}, {HEAL_DEC}
local USE_SCHOOL = {USE_SCHOOL}

local function FormatNumber(n, mode, dec)
	if mode == 2 and n >= 10000 then
		local v, suffix
		if n >= 1000000 then
			v, suffix = n / 1000000, "M"
		else
			v, suffix = n / 1000, "K"
		end
		local s = string.format("%." .. dec .. "f", v)
		s = string.gsub(s, "0*$", "")
		s = string.gsub(s, "%.$", "")
		return s .. suffix
	elseif mode == 1 then
		-- точки-разделители: заякоренный повтор, жадный %d+ ломает группы
		local s, k = tostring(n), 0
		repeat
			s, k = string.gsub(s, "^(%d+)(%d%d%d)", "%1.%2")
		until k == 0
		return s
	end
	return tostring(n)
end


-- MaskColor: маска школы → {r, g, b}; при неизвестной маске остаётся
-- цвет Близзардов.
local MaskColor = {
{MASK_COLOR}
}


local function SchoolColorRGB(mask)
	if not USE_SCHOOL or not mask or mask == 0 then
		return nil
	end
	return MaskColor[bit.band(mask, 0x7F)]
end

-- Кэш последних уронов по ИГРОКУ из combat log: сумма-школа-время.
-- COMBAT_TEXT_UPDATE школу не несёт, поэтому суммой ищем совпадение.
local hist, hn = {}, 0
local LOG_LIFETIME = 1.2

local logFrame = CreateFrame("Frame")
logFrame:RegisterEvent("COMBAT_LOG_EVENT_UNFILTERED")
logFrame:SetScript("OnEvent", function(_, _, timestamp, event, srcGUID, srcName, srcFlags, dstGUID, dstName, dstFlags, ...)
	if dstGUID ~= UnitGUID("player") then
		return
	end
	if event == "SPELL_DAMAGE" or event == "SPELL_PERIODIC_DAMAGE" then
		-- 3.3.5: ... spellId, spellName, spellSchool, amount
		local school, amount = select(3, ...), select(4, ...)
		hn = hn + 1
		hist[(hn % 12) + 1] = { amount, school, GetTime() }
	elseif event == "SWING_DAMAGE" then
		local amount = select(1, ...)
		hn = hn + 1
		hist[(hn % 12) + 1] = { amount, 1, GetTime() }
	end
end)

-- MatchSchool(amount) — цвет школы недавнего лог-урона с такой суммой.
local function MatchSchool(amount)
	if not USE_SCHOOL or not amount or amount <= 0 then
		return nil
	end
	local now = GetTime()
	local recent
	for i = 1, 12 do
		local e = hist[i]
		if e and (now - e[3]) < LOG_LIFETIME then
			if e[1] == amount then
				return SchoolColorRGB(e[2])
			end
			if not recent then
				recent = e -- запасной вариант: самый свежий урон
			end
		end
	end
	if recent then
		return SchoolColorRGB(recent[2])
	end
	return nil
end

local origAddMessage = CombatText_AddMessage
function CombatText_AddMessage(message, scrollFunction, r, g, b, displayType, isStaggered)
	local sign, num, rest = string.match(message, "^([-+])(%d+)(.*)$")
	if num then
		local n = tonumber(num) or 0
		if sign == "-" then
			message = "-" .. FormatNumber(n, DMG_MODE, DMG_DEC) .. rest
			-- собственный полученный урон: красим по школе заклинания
			local rgb = MatchSchool(n)
			if rgb then
				r, g, b = rgb[1], rgb[2], rgb[3]
			end
		else
			message = "+" .. FormatNumber(n, HEAL_MODE, HEAL_DEC) .. rest
			-- лечение светлее, чтобы отличать от тёмно-зелёной природы
			r, g, b = 0.66, 1, 0.66
		end
	end
	return origAddMessage(message, scrollFunction, r, g, b, displayType, isStaggered)
end
`

// addonLua заполняет шаблон выбранными форматами.
func addonLua(dmg, heal formatCfg, school bool, palette []schoolType) string {
	yes := "true"
	if !school {
		yes = "false"
	}
	return strings.NewReplacer(
		"{DMG_MODE}", fmt.Sprint(int(dmg.mode)),
		"{DMG_DEC}", fmt.Sprint(int(clampDec(dmg.dec))),
		"{HEAL_MODE}", fmt.Sprint(int(heal.mode)),
		"{HEAL_DEC}", fmt.Sprint(int(clampDec(heal.dec))),
		"{USE_SCHOOL}", yes,
		"{MASK_COLOR}", maskColorLua(palette),
	).Replace(addonLuaTmpl)
}

// writeAddon создаёт аддон в dir\Interface\AddOns\dmg_format_patcher.
func writeAddon(dir string, dmg, heal formatCfg, school bool, palette []schoolType) (string, error) {
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
	if err := os.WriteFile(lua, []byte(addonLua(dmg, heal, school, palette)), 0o644); err != nil {
		return base, fmtErr(tr(
			"не удалось записать %s: %v",
			"failed to write %s: %v"), lua, err)
	}
	return base, nil
}

// removeAddon удаляет наш аддон, чужие файлы не трогает.
func removeAddon(dir string) bool {
	base := filepath.Join(dir, "Interface", "AddOns", addonFolder)
	toc := filepath.Join(base, addonFolder+".toc")
	data, err := os.ReadFile(toc)
	if err != nil {
		return false
	}
	if !strings.Contains(string(data), "Title: "+addonFolder) {
		return false
	}
	_ = os.Remove(toc)
	_ = os.Remove(filepath.Join(base, "dmg_format.lua"))
	if entries, err := os.ReadDir(base); err == nil && len(entries) == 0 {
		_ = os.Remove(base)
	}
	return true
}

// maskColorLua — Lua-таблица «маска = {r,g,b}» из палитры.
func maskColorLua(palette []schoolType) string {
	var sb strings.Builder
	for _, st := range palette {
		r := int(st.color>>16) & 255
		g := int(st.color>>8) & 255
		b := int(st.color) & 255
		fmt.Fprintf(&sb, "\t[%d] = {%d/255, %d/255, %d/255},\n", st.mask, r, g, b) // 0..1
	}
	return sb.String()
}
