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

Патчер клиента WoW 3.3.5a: меняет вид цифр боя и раскрашивает их
по школе магии. Есть графический интерфейс и консольный режим из
старых версий.

### Возможности

- переформатирование цифр боя: `1234567` → `1.234.567`
  или `111.111M / 111.11M / 111.1M`; урон и лечение настраиваются
  отдельно, 1-3 знака после точки, хвостовые нули отбрасываются
  (`111.10K → 111.1K`);
- аддон `dmg_format_patcher` — переформатирует и красит цифры
  урона/лечения самого себя; поверх старой версии ставится без конфликтов;
- цвет цифр урона по школе магии — в 3D-тексте и в аддоне: свой цвет
  для каждой школы из таблицы клиента (физика белая, огонь красный,
  природа зелёный, лёд голубой, тень фиолетовый, тайная бирюзовый,
  все школы сразу — розово-фиолетовый и т.д.); автоатаки остаются белыми;
- восстановление ванильного клиента (цифры, цвета, удаление аддона);
- бэкап `Wow.exe.bak` создаётся один раз, повторные запуски просто
  меняют настройки.

### Что нового: графический интерфейс

Патчер получил полноценное окно.

- выбор папки клиента, кнопки «Патчить» и «Восстановить exe»;
- выпадающие списки форматов урона и лечения с примерами;
- галочки аддона: переформатирование цифр и окраска по школе;
- ползунки и поля RGB для цветов школ, рядом — квадратик с цветом;
- раскрытый список всех сочетаний школ: цвет каждой маски можно
  задать отдельно, при изменении базовых цветов сочетания обновляются;
- кнопка «Сбросить цвета» возвращает стандартную палитру;
- масштаб интерфейса подстраивается под монитор, кнопкой «Масштаб»
  можно переключить HD, FHD, 2K, 4K или авто;
- операции и ошибки — в небольшом окне уведомлений;
- форматы и цвета из пропатченного клиента читаются обратно.

### Консольный режим

```
dmg_format_patcher.exe                        — графический интерфейс
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
- `--restore` — ванильные цифры и цвета, удаление аддона;
- язык: `Язык / Choose language:` (по умолчанию русский) или `--lang=ru|en`;
  строки до выбора языка — на обоих языках;
- цвета консоли: урон — красный, лечение — зелёный, ошибки — красный;
  при выводе в файл/пайп отключаются автоматически.

---

<a name="english"></a>

## English

A WoW 3.3.5a client patcher: changes combat digits and colors them by
spell school. Includes a GUI and the console mode from older versions.

### Features

- number formats: `1234567` → `1.234.567` or `111.111M / 111.11M / 111.1M`;
  damage and healing configured separately, 1-3 decimals, trailing
  zeros trimmed (`111.10K → 111.1K`);
- the `dmg_format_patcher` addon — reformats and colors the floating
  digits of your own damage/healing; installs over the old version
  cleanly;
- spell-school colors for damage digits — in the 3D world text and in
  the addon: every client school mask gets its own color (physical
  white, fire red, nature green, frost light blue, shadow purple,
  arcane turquoise, all-schools chaos pink, etc.); auto-attacks stay white;
- restore of the vanilla client (digits, colors, addon removal);
- the `Wow.exe.bak` backup is created once; re-running just changes
  the settings.

### New: graphical interface

The patcher now has a full window.

- client folder selection with «Patch» and «Restore exe» buttons;
- dropdowns for damage and healing formats with examples;
- addon checkboxes: digit reformatting and school coloring;
- RGB sliders and edit fields for school colors with a color swatch;
- an expandable list of all school combinations: each mask color is
  editable and follows the base school colors;
- a «Reset colors» button restores the standard palette;
- the interface scales itself for the monitor, the «Scale» button
  switches HD, FHD, 2K, 4K or automatic;
- operations and errors are shown in a small notifications window;
- formats and colors are read back from a patched client.

### Console mode

```
dmg_format_patcher.exe                        — graphical interface
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
- `--restore` — vanilla numbers and colors, addon removal;
- language: `Язык / Choose language:` (Russian default) or `--lang=ru|en`;
  lines printed before the choice are bilingual;
- console colors: damage red, healing green (red — errors); auto-off
  when piped to a file.
