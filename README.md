# 🎧 Liquid Glass Radio
Demo - https://pl.darkdna.ru
<img width="1272" height="2772" alt="48841" src="https://github.com/user-attachments/assets/0b913b73-a7dc-4107-a1d5-721c0c2fc3e7" />
<img width="1270" height="2366" alt="48844" src="https://github.com/user-attachments/assets/57d3294b-bae4-4382-afcc-fb48942d8b6d" />


Красивый интернет-радио плеер на Go в стиле **Liquid Glass**.

Треки берутся из каталога `./music`. Обложки — из ID3-тегов.

## Быстрый старт

```bash
mkdir -p music
go run .
```

Откройте **http://localhost:8080**

## Возможности

### Настройки (кнопка ⚙)
- Выбор темы Liquid Glass: Aurora, Ocean, Sunset, Emerald, Amethyst
- Показать / скрыть анимированный эквалайзер
- Показать / скрыть обложку трека  
Настройки сохраняются в `localStorage`.

### Обложки и эквалайзер
- Обложки извлекаются из метаданных
- Эквалайзер в реальном времени (Web Audio API) — в такт музыке
- Миниатюры в плейлисте

### Управление
- Shuffle · Repeat · Sleep-таймер (15–60 мин)
- Горячие клавиши: Space, ←, →, Esc (закрыть настройки)

## Форматы
MP3 · OGG · WAV · FLAC · M4A · AAC · Opus · WebM

## Docker

### Сборка образа

```bash
docker build -t liquid-radio:latest .
```

### Запуск

```bash
# Положите треки в ./music, затем:
docker run -d \
  --name liquid-radio \
  -p 8080:8080 \
  -v "$(pwd)/music:/app/music:ro" \
  liquid-radio:latest
```

Или через Compose:

```bash
docker compose up -d --build
```

Откройте **http://localhost:8080**

### Параметры

| Параметр | Значение |
|----------|----------|
| Порт | `8080` |
| Том с музыкой | `/app/music` (mount host `./music`) |
| Пользователь в контейнере | `radio` (uid 1000) |
| Базовый образ runtime | Alpine 3.20 (~10–15 MB итого) |

Перезагрузка списка треков — перезапуск контейнера (сканирование каталога при старте).
