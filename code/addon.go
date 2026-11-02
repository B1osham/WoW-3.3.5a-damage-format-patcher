package main

// addon.go — генерация аддона dmg_format_patcher для «своего»
// всплывающего текста.
//
// Урон и лечение самого себя рисует не exe, а аддон Blizzard_CombatText
// (LoadOnDemand): в CombatText_OnEvent сообщение собирается конкатенацией
// ("-"..data / "+"..arg3, Blizzard_CombatText.lua), и в exe его не
// перехватить. Поэтому генерируется мини-аддон, который перехватывает
// итоговую выдачу:
//
//	CombatText_AddMessage(message, scrollFunction, r, g, b, displayType, isStaggered)
//
// и: (1) переформатирует ведущее число ("-N" — урон, "+N" — лечение),
// (2) при включённой школе меняет цвет урона: в COMBAT_TEXT_UPDATE нет
// информации о школе, поэтому перед хуком слушаем COMBAT_LOG_EVENT_UNFILTERED
// и запоминаем (сумма → школа) последних полученных игроком уронов;
// сообщение с подходящей суммой за последние ~0.7 с красится цветом школы
// (то же правило приоритета магии над физикой, что и в exe).

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

// addonLuaTmpl — шаблон хука. Плейсхолдеры заменяет strings.NewReplacer.
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

-- ЦветаSchools: по маске школы ( SPELLSCHOOL_...) — triple {r,g,b}.
-- Приоритет МАГИИ над физикой: у маски с прочими битами физика
-- отбрасывается (как в exe-стабе); чисто физика — белый.
-- MaskColor: маска школы → {r,g,b}, значения ровно из exe-таблицы
-- (LUT в секции .w3cs). Неизвестная маска — цвет Близзардов остаётся.
local MaskColor = {
	[1]   = {255, 255, 255}, -- физика
	[2]   = {255, 230, 120}, -- holy — золотистый «свет», [3] holystrike
	[3]   = {255, 230, 120},
	[4]   = {255, 0, 0},     -- fire, [5] flamestrike
	[5]   = {255, 0, 0},
	[6]   = {255, 140, 60},  -- holyfire — оранжевый (огонь+свет)
	[8]   = {0, 190, 0},     -- nature (тёмно-зелёный), [9] stormstrike
	[9]   = {0, 190, 0},
	[10]  = {190, 255, 120}, -- holystorm — свет+природа
	[12]  = {208, 255, 0},   -- firestorm
	[16]  = {64, 130, 255},  -- frost (мягкий голубой), [17] froststrike
	[17]  = {64, 130, 255},
	[20]  = {255, 0, 128},   -- frostfire
	[24]  = {0, 255, 128},   -- froststorm
	[28]  = {170, 255, 160}, -- elemental
	[32]  = {185, 0, 171},   -- shadow, [33] shadowstrike
	[33]  = {185, 0, 171},
	[34]  = {210, 140, 200}, -- twilight — тень+свет
	[36]  = {255, 0, 160},   -- shadowflame
	[40]  = {120, 180, 85},  -- plague
	[49]  = {92, 0, 213},    -- shadowfrost
	[64]  = {0, 255, 255},   -- arcane — бирюзовый
	[66]  = {255, 240, 170}, -- divine — светлее holy
	[68]  = {255, 80, 190},  -- spellfire
	[72]  = {0, 255, 200},   -- spellstorm
	[80]  = {0, 160, 255},   -- spellfrost
	[96]  = {92, 128, 213},  -- spellshadow
	[126] = {200, 160, 255}, -- magic (все магии) — лавандовый
	[127] = {255, 0, 255},   -- chaos (все школы)
}


local function SchoolColorRGB(mask)
	if not USE_SCHOOL or not mask or mask == 0 then
		return nil
	end
	local bit = bit or _G.bit
	mask = bit.band(mask, 0x7F)
	-- combat log иногда даёт школу индексом (1..7: физика..тайная):
	-- переводим в битовую маску, чтобы MaskColor нашёлся всегда.
	if mask <= 7 then
		mask = bit.lshift(1, mask - 1)
	end
	return MaskColor[mask]
end

-- Кэш последних уронов по ИГРОКУ из combat log: сумма-школа-время.
-- COMBAT_TEXT_UPDATE школу не несёт, поэтому суммой ищем совпадение.
local hist, hn = {}, 0
local LOG_LIFETIME = 0.7

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
		hist[(hn % 8) + 1] = { amount, school, GetTime() }
	elseif event == "SWING_DAMAGE" then
		local amount = select(1, ...)
		hn = hn + 1
		hist[(hn % 8) + 1] = { amount, 1, GetTime() }
	end
end)

-- MatchSchool(amount) — цвет школы недавнего лог-урона с такой суммой.
local function MatchSchool(amount)
	if not USE_SCHOOL or not amount or amount <= 0 then
		return nil
	end
	local now = GetTime()
	for i = 1, 8 do
		local e = hist[i]
		if e and e[1] == amount and (now - e[3]) < LOG_LIFETIME then
			return SchoolColorRGB(e[2])
		end
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
		end
	end
	return origAddMessage(message, scrollFunction, r, g, b, displayType, isStaggered)
end
`

// addonLua заполняет шаблон выбранными форматами.
func addonLua(dmg, heal formatCfg, school bool) string {
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
	).Replace(addonLuaTmpl)
}

// writeAddon создаёт аддон в dir\Interface\AddOns\school_colors_patcher.
func writeAddon(dir string, dmg, heal formatCfg, school bool) (string, error) {
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
	if err := os.WriteFile(lua, []byte(addonLua(dmg, heal, school)), 0o644); err != nil {
		return base, fmtErr(tr(
			"не удалось записать %s: %v",
			"failed to write %s: %v"), lua, err)
	}
	return base, nil
}

// removeAddon удаляет наш аддон (для --restore). Чужие файлы не трогаем.
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
