# TwinBid

Frontend for the TwinBid advertising platform and advertiser dashboard.

## Размещение на сервере

Готовая сборка находится в `dist/`. Разместите всё содержимое этой папки, включая `assets/`, в корне фронтенда. Для адресов приложения настройте SPA fallback на `index.html`.

Единственный `.env` уже заполнен рабочими значениями и не исключён из Git. Готовый `dist/` также не исключён из Git. Дополнительные ENV-файлы не нужны. Если собираете из исходников: `npm ci`, затем `npm run build`. После изменения ENV обязательно пересоберите фронтенд. Уже собранный `dist/` использует API `https://twinbid.io`, моки отключены.

## Local development

Requirements: Node.js and npm.

```sh
npm install
npm run dev
```

The development server is available at `http://localhost:8080`.

## Production build

```sh
npm run build
npm run preview
```

## Technology

- Vite
- TypeScript
- React
- shadcn/ui
- Tailwind CSS

## Publisher cabinet

Полная интеграция с реальным API, ENV, команды сборки и ограничения тестирования: [README_PUBLISHERS.md](README_PUBLISHERS.md).
