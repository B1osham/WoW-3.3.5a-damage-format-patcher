<img width="1076" height="774" alt="first" src="https://github.com/user-attachments/assets/edd3fd50-bfef-4ca7-8eb0-fd95e5304488" />
<img width="1077" height="830" alt="second" src="https://github.com/user-attachments/assets/02fc3823-91c6-4753-807a-2f84200f091a" />

*[Русский](#russian) | [English](#english)*

---

<a name="russian"></a>

## Русский

Меняет вид цифр боя: `1234567` → `1.234.567` или `111.11K / 111.11M`.
Урон и лечение настраиваются отдельно, 1..3 знака после точки,
хвостовые нули отбрасываются (`111.10K → 111.1K`).

### Запуск

```
dmg_format_patcher.exe                        — спросит язык, формат, аддон
dmg_format_patcher.exe Wow.exe --dmg=short:2 --heal=dots --addon=1
dmg_format_patcher.exe Wow.exe --restore      — вернуть ванильные цифры
```

- формат: `plain` (0) | `dots` (1) | `short[:DEC]` (2), DEC = 1..3;
- интерактив по порядку: язык, урон, лечение, аддон; можно всё флагами;
- `--addon=1|0` — создать аддон `текущая_папка\Interface\AddOns\dmg_format_patcher\`
  (цифры урона/лечения самого себя); если патчер запущен не из корня игры —
  предупреждение перенести папку аддона;
- повторный запуск просто меняет формат, бэкап `Wow.exe.bak` создаётся один раз;
- `--restore` — ванильные цифры и удаление аддона;
- язык: `Язык / Choose language:` (по умолчанию русский) или `--lang=ru|en`;
  строки до выбора языка — на обоих языках;
- цвета: урон — красный, лечение — зелёный, ошибки — красный;
  при выводе в файл/пайп отключаются автоматически.

---

<a name="english"></a>

## English

Changes combat numbers: `1234567` → `1.234.567` or `111.11K / 111.11M`.
Damage and healing are configured separately, 1..3 decimals, trailing
zeros are trimmed (`111.10K → 111.1K`).

### Run

```
dmg_format_patcher.exe                        — asks language, format, addon
dmg_format_patcher.exe Wow.exe --dmg=short:2 --heal=dots --addon=1
dmg_format_patcher.exe Wow.exe --restore      — restore vanilla numbers
```

- format: `plain` (0) | `dots` (1) | `short[:DEC]` (2), DEC = 1..3;
- interactive order: language, damage, healing, addon; everything flag-driven too;
- `--addon=1|0` — writes the addon to `current_folder\Interface\AddOns\dmg_format_patcher\`
  (self floating combat text); if launched outside the game root, warns to move the addon;
- re-running just changes the format; backup `Wow.exe.bak` is created once;
- `--restore` — vanilla numbers and addon removal;
- language: `Язык / Choose language:` (Russian default) or `--lang=ru|en`;
  lines printed before the choice are bilingual;
- colors: damage red, healing green (red — errors); auto-off when piped to a file.
