# План реализации: CLI/TUI Registry

## Файлы плана

| Файл | Содержание |
|---|---|
| [00-feedback.md](00-feedback.md) | Фидбек по todo.md: пробелы и уточнения |
| [step-01-cleanup.md](step-01-cleanup.md) | Удаление старой архитектуры (YAML, tldr, хоткеи) |
| [step-02-data-model.md](step-02-data-model.md) | Новая модель данных: ToolMeta как источник правды |
| [step-03-github-api.md](step-03-github-api.md) | Расширение GitHub API: Stars, Languages, About |
| [step-04-left-panel.md](step-04-left-panel.md) | Переделка левой панели: только Registry |
| [step-05-right-panel.md](step-05-right-panel.md) | Переделка правой панели: Card + Help Output |
| [step-06-help-output.md](step-06-help-output.md) | Вывод `--help` и `man` в правой половине |

## Порядок выполнения

Шаги 1 и 2 должны идти последовательно (2 зависит от 1).  
Шаги 3, 4 можно делать параллельно после шага 2.  
Шаги 5 и 6 идут после 3 и 4.

```
1 → 2 → 3 ──→ 5 → 6
          ↘ 4 ↗
```

## Итоговый результат

После всех шагов приложение:
- Показывает только tracked tools из `meta.yaml`
- Добавление через `keys track <tool> --github github.com/owner/repo`
- Правая панель: Tool Card (About, Stars, Release, Languages, Changelog) + вывод `--help`/`man`
- Никаких YAML-конфигов, никаких встроенных хоткеев
