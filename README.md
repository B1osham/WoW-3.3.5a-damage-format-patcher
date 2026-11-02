<table>
  <tr>
    <td width="50%">
      <a href="https://github.com/user-attachments/assets/edd3fd50-bfef-4ca7-8eb0-fd95e5304488">
        <img src="https://github.com/user-attachments/assets/edd3fd50-bfef-4ca7-8eb0-fd95e5304488" alt="First preview" width="100%" />
      </a>
    </td>
    <td width="50%">
      <a href="https://github.com/user-attachments/assets/02fc3823-91c6-4753-807a-2f84200f091a">
        <img src="https://github.com/user-attachments/assets/02fc3823-91c6-4753-807a-2f84200f091a" alt="Second preview" width="100%" />
      </a>
    </td>
  </tr>
</table>


*[Русский](#russian) | [English](#english)*

---


<a name="russian"></a>

## Русский

Меняет вид цифр боя: `1234567` → `1.234.567` или `111.111M / 111.11M / 111.1M`.
Урон и лечение настраиваются отдельно, 1-3 знака после точки,
хвостовые нули отбрасываются (`111.10K → 111.1K`).

**Новое в этой версии:** цифры урона можно раскрасить по школе магии —
в 3D-тексте и в аддоне своего текста. Свой цвет для каждой школы из
таблицы клиента (физика белая, огонь красный, природа зелёный, лёд
голубой, тень фиолетовый, тайная бирюзовый, все школы сразу —
розово-фиолетовый и т.д.). Автоатаки остаются белыми. Аддон
`dmg_format_patcher` ставится поверх старой версии без конфликтов.

### Запуск

```
dmg_format_patcher.exe                        — спросит язык, формат, аддон, цвета
dmg_format_patcher.exe Wow.exe --dmg=short:2 --heal=dots --addon=1 --school=1 --addon-school=1
dmg_format_patcher.exe Wow.exe --restore      — вернуть ванильные цифры и цвета
```

- формат: `plain` (0) | `dots` (1) | `short[:DEC]` (2), DEC = 1..3;
- интерактив по порядку: язык, урон, лечение, цвет цифр в мире (да/нет),
  аддон, цветной текст в аддоне (только если аддон нужен); можно всё флагами;
- `--school=1|0` — цвет цифр урона по школе магии в 3D-тексте;
- `--addon=1|0` — создать аддон `текущая_папка\Interface\AddOns\dmg_format_patcher\`
  (цифры урона/лечения самого себя); если патчер запущен не из корня игры —
  предупреждение перенести папку аддона;
- `--addon-school=1|0` — красить цифры в аддоне по школе тоже;
- повторный запуск просто меняет настройки, бэкап `Wow.exe.bak` создаётся один раз;
- `--restore` — ванильные цифры и цвета, удаление аддона;
- язык: `Язык / Choose language:` (по умолчанию русский) или `--lang=ru|en`;
  строки до выбора языка — на обоих языках;
- цвета консоли: урон — красный, лечение — зелёный, ошибки — красный;
  при выводе в файл/пайп отключаются автоматически.

---


<a name="english"></a>

## English

Changes combat numbers: `1234567` → `1.234.567` or `111.111M / 111.11M / 111.1M`.
Damage and healing are configured separately, 1-3 decimals, trailing
zeros are trimmed (`111.10K → 111.1K`).

**New in this version:** spell damage digits can be colored by magic
school — in the 3D world text and in the self floating text addon. Every
client school mask gets its own color (physical white, fire red, nature
green, frost light blue, shadow purple, arcane turquoise, all-schools
chaos pink, etc.). Auto-attacks stay white. The `dmg_format_patcher`
addon installs over the old version without conflicts.

### Run

```
dmg_format_patcher.exe                        — asks language, format, addon, colors
dmg_format_patcher.exe Wow.exe --dmg=short:2 --heal=dots --addon=1 --school=1 --addon-school=1
dmg_format_patcher.exe Wow.exe --restore      — restore vanilla numbers and colors
```

- format: `plain` (0) | `dots` (1) | `short[:DEC]` (2), DEC = 1..3;
- interactive order: language, damage, healing, world digit colors
  (yes/no), addon, colored addon text (only if the addon is wanted);
  everything flag-driven too;
- `--school=1|0` — spell-school colors for world damage digits;
- `--addon=1|0` — writes the addon to `current_folder\Interface\AddOns\dmg_format_patcher\`
  (self floating combat text); if launched outside the game root, warns to move the addon;
- `--addon-school=1|0` — tint the addon digits by school too;
- re-running just changes the settings; backup `Wow.exe.bak` is created once;
- `--restore` — vanilla numbers and colors, addon removal;
- language: `Язык / Choose language:` (Russian default) or `--lang=ru|en`;
  lines printed before the choice are bilingual;
- console colors: damage red, healing green (red — errors); auto-off
  when piped to a file.
