# Установка WebKVM из локального GitLab

Целевым машинам не нужен Go/Node/исходники: GitLab CI (`.gitlab-ci.yml`)
собирает бинарь с встроенным фронтендом и публикует артефакты
(tar.gz, .deb, .rpm) в generic packages и Release проекта.

## Установка / обновление одной командой

На машине с доступом к GitLab:

```bash
sudo ./scripts/install-from-gitlab.sh
```

Скрипт скачивает последнюю версию, ставит бинарь в `/usr/local/bin/webkvm`
(прошлая копия сохраняется как `webkvm.previous`), обновляет unit и
logrotate, включает сервис и проверяет `/api/health` до 20 секунд; при
неудаче автоматически откатывается на предыдущий бинарь.

Переменные окружения:

| Переменная   | По умолчанию          | Описание                                      |
|--------------|-----------------------|-----------------------------------------------|
| `GITLAB_URL` | `http://localhost:8081` | Адрес локального GitLab                     |
| `PROJECT`    | `root/webkvm`         | `namespace/repo`                              |
| `VERSION`    | последний release     | Версия без `v` (например `1.2.0`)             |
| `TOKEN`      | —                     | PAT с правом `read_api` для приватного проекта |

Примеры:

```bash
# Конкретная версия на приватном GitLab
sudo GITLAB_URL=http://gitlab.local:8081 TOKEN=glpat-xxxx VERSION=1.2.0 ./scripts/install-from-gitlab.sh

# Удаление (данные /opt/webkvm остаются)
sudo ./scripts/install-from-gitlab.sh --uninstall
```

## Где лежат артефакты

- **Generic packages** (то, что качает скрипт):
  `${GITLAB_URL}/api/v4/projects/<id>/packages/generic/webkvm/<версия>/`
- **Releases**: `Deploy → Releases` проекта — там же ссылки на tar.gz.
- **Docker-образ**: `${GITLAB_URL}/<namespace>/webkvm:<тег>` (registry), только теги `v*`.

Публикация происходит при пуше тега `v*`; test+build — на каждый пуш в `main`.

## Ручная установка (без скрипта)

Скачайте `webkvm-linux-amd64.tar.gz`, внутри — `INSTALL.txt` с пятью
командами (бинарь, unit, logrotate, `systemctl enable --now`).
