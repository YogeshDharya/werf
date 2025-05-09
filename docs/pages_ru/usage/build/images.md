---
title: Образы и зависимости
permalink: usage/build/images.html
---

<!-- прим. для перевода: на основе https://werf.io/docs/v2/reference/werf_yaml.html#image-section -->

## Добавление образов

Для сборки c werf необходимо добавить описание образов в `werf.yaml` проекта. Каждый образ добавляется директивой `image` с указанием имени образа:

```yaml
project: example
configVersion: 1
---
image: frontend
# ...
---
image: backend
# ...
---
image: database
# ...
```

> Имя образа — это уникальный внутренний идентификатор образа, который позволяет ссылаться на него при конфигурации и при вызове команд werf.

Далее для каждого образа в `werf.yaml` необходимо определить сборочные инструкции [с помощью Dockerfile](#dockerfile) или [stapel](#stapel).

### Dockerfile

<!-- прим. для перевода: на основе https://werf.io/docs/v2/reference/werf_yaml.html#dockerfile-builder -->

#### Написание Dockerfile-инструкций

Для описания сборочных инструкций образа поддерживается стандартный Dockerfile. Следующие ресурсы помогут в его написании:

* [Dockerfile Reference](https://docs.docker.com/engine/reference/builder/).
* [Best practices for writing Dockerfiles](https://docs.docker.com/develop/develop-images/dockerfile_best-practices/).

#### Использование Dockerfile

Конфигурация сборки Dockerfile может выглядеть следующим образом:

```Dockerfile
# Dockerfile
FROM node
WORKDIR /app
COPY package*.json /app/
RUN npm ci
COPY . .
CMD ["node", "server.js"]
```

```yaml
# werf.yaml
project: example
configVersion: 1
---
image: backend
dockerfile: Dockerfile
```

#### Использование определённой Dockerfile-стадии

Также вы можете описывать несколько целевых образов из разных стадий одного и того же Dockerfile:

```Dockerfile
# Dockerfile
FROM node as backend
WORKDIR /app
COPY package*.json /app/
RUN npm ci
COPY . .
CMD ["node", "server.js"]

FROM python as frontend
WORKDIR /app
COPY requirements.txt /app/
RUN pip install -r requirements.txt
COPY . .
CMD ["gunicorn", "app:app", "-b", "0.0.0.0:80", "--log-file", "-"]
```

```yaml
# werf.yaml
project: example
configVersion: 1
---
image: backend
dockerfile: Dockerfile
target: backend
---
image: frontend
dockerfile: Dockerfile
target: frontend
```

И конечно вы можете описывать образы, основанные на разных Dockerfile:

```yaml
# werf.yaml
project: example
configVersion: 1
---
image: backend
dockerfile: dockerfiles/Dockerfile.backend
---
image: frontend
dockerfile: dockerfiles/Dockerfile.frontend
```

#### Выбор директории сборочного контекста

Чтобы указать сборочный контекст используется директива `context`. **Важно:** в этом случае путь до Dockerfile указывается относительно директории контекста:

```yaml
project: example
configVersion: 1
---
image: docs
context: docs
dockerfile: Dockerfile
---
image: service
context: service
dockerfile: Dockerfile
```

Для образа `docs` будет использоваться Dockerfile по пути `docs/Dockerfile`, а для `service` — `service/Dockerfile`.

#### Использование сборочных секретов

> **ЗАМЕЧАНИЕ:** Чтобы использовать секреты в сборках, их нужно явно разрешить в настройках гитерминизма. Подробнее ([здесь]({{ "/usage/project_configuration/giterminism.html#использование-сборочных-секретов" | true_relative_url }}))

Секрет сборки — это любая конфиденциальная информация, например пароль или токен API, используемая в процессе сборки вашего приложения.

Аргументы сборки и переменные окружения не подходят для передачи секретов в сборку, поскольку они сохраняются в конечном образе.  

Вы можете использовать секреты при сборке, описав их в `werf.yaml`.

```yaml
# werf.yaml
project: example
configVersion: 1
---
image: backend
dockerfile: Dockerfile
secrets:
  - env: AWS_ACCESS_KEY_ID
  - id: aws_secret_key
    env: AWS_SECRET_ACCESS_KEY
  - src: "~/.aws/credentials"
  - id: plainSecret
    value: plainSecretValue
```
```yaml
# werf-giterminism.yaml
giterminismConfigVersion: 1

config:
  secrets:
    allowEnvVariables:
      - "AWS_ACCESS_KEY_ID"
    allowFiles:
      - "~/.aws/credentials"
    allowValueIds:
      - plainSecret
```

Чтобы использовать секрет в сборке и сделать его доступным для инструкции `RUN`, используйте флаг `--mount=type=secret` в Dockerfile. 

При использовании секрета в Dockerfile, секрет монтируется в файл по умолчанию. Путь к файлу секрета по умолчанию внутри контейнера сборки — `/run/secrets/<id>`. Если `id` секрета явно не указан в `werf.yaml`, то в качестве `id` будет использовано значение по умолчанию:

- Для `env` — имя переменной окружения.
- Для `src` — имя конечного файла (например, для `/path/to/file` будет использован `id: file`).

> Для `value` — поле id является обязательным.

```Dockerfile
# Dockerfile
FROM alpine:3.18

# Пример использования секрета из переменной окружения
RUN --mount=type=secret,id=AWS_ACCESS_KEY_ID \
    export WERF_BUILD_SECRET="$(cat /run/secrets/AWS_ACCESS_KEY_ID)"

# Пример использования секрета из файла с секретами
RUN --mount=type=secret,id=credentials \
    AWS_SHARED_CREDENTIALS_FILE=/run/secrets/credentials \
    aws s3 cp ...

# Пример монтирования секрета как переменную окружения
RUN --mount=type=secret,id=AWS_ACCESS_KEY_ID,env=AWS_ACCESS_KEY_ID \
    --mount=type=secret,id=aws-secret-key,env=AWS_SECRET_ACCESS_KEY \
    aws s3 cp ...

# Пример монтирования секрета как файл с другим именем
RUN --mount=type=secret,id=credentials,target=/root/.aws/credentials \
    aws s3 cp ...

# Пример использования произвольного значения, которое не будет сохранено в конечном образе
RUN --mount=type=secret,id=plainSecret \
    export WERF_BUILD_SECRET="$(cat /run/secrets/plainSecret)"
```

#### Использование SSH-агента

Вы можете предоставить доступ к сокету SSH-агента или SSH-ключам во время сборки. Это удобно, если ваш Dockerfile содержит команды, требующие SSH-аутентификации, например, для клонирования приватных репозиториев.

Для этого используйте флаг `--mount=type=ssh` в командах RUN:

```Dockerfile
FROM alpine
RUN apk add --no-cache openssh-client
RUN mkdir -p -m 0700 ~/.ssh && ssh-keyscan gitlab.com >> ~/.ssh/known_hosts
RUN --mount=type=ssh ssh -q -T git@gitlab.com 2>&1 | tee /hello
```

Подробную информацию об использовании SSH-агента можно найти [здесь]({{ "/usage/build/process.html#использование-ssh-агента" | true_relative_url }}).

#### Добавление произвольных файлов в сборочный контекст

По умолчанию контекст сборки Dockerfile-образа включает только файлы из текущего коммита репозитория проекта. Файлы, не добавленные в Git, или некоммитнутые изменения не попадают в сборочный контекст. Такая логика действует в соответствии [с настройками гитерминизма]({{ "/usage/project_configuration/giterminism.html" | true_relative_url }}) по умолчанию.

Чтобы добавить в сборочный контекст файлы, которые не хранятся в Git, нужна директива `contextAddFiles` в `werf.yaml`, а также нужно разрешить использование директивы `contextAddFiles` в `werf-giterminism.yaml` (подробнее [про гитерминизм]({{ "/usage/project_configuration/giterminism.html#contextaddfiles" | true_relative_url }})):

```yaml
# werf.yaml
project: example
configVersion: 1
---
image: app
dockerfile: Dockerfile
context: app
contextAddFiles:
- file1
- dir1/
- dir2/file2.out
```

```yaml
# werf-giterminism.yaml
giterminismConfigVersion: 1
config:
  dockerfile:
    allowContextAddFiles:
    - app/file1
    - app/dir1/
    - app/dir2/file2.out
```

В данной конфигурации контекст сборки будет состоять из следующих файлов:

- `app/**/*` из текущего коммита репозитория проекта;
- файлы `app/file1`, `app/dir2/file2.out` и директория `dir1`, которые находятся в директории проекта.

#### Мультиплатформенная сборка

werf поддерживает мультиплатформенную и кроссплатформенную сборку, что позволяет создавать образы для различных архитектур и операционных систем (подробнее [в соответствующем разделе документации]({{ "/usage/build/process.html#мультиплатформенная-и-кроссплатформенная-сборка" | true_relative_url }})).

##### Кросс-компиляция

Если в вашем проекте требуется кросс-компиляция, вы можете использовать multi-stage сборки для создания артефактов для целевых платформ. Для этого доступны следующие аргументы сборки:

- `TARGETPLATFORM`: платформа для сборки результата (например, linux/amd64, linux/arm/v7, windows/amd64).
- `TARGETOS`: ОС целевой платформы.
- `TARGETARCH`: архитектура целевой платформы.
- `TARGETVARIANT`: вариант целевой платформы.
- `BUILDPLATFORM`: платформа узла, выполняющего сборку.
- `BUILDOS`: ОС платформы сборщика.
- `BUILDARCH`: архитектура платформы сборщика.
- `BUILDVARIANT`: вариант платформы сборщика.

Пример:

```Dockerfile
FROM --platform=$BUILDPLATFORM golang:alpine AS build
ARG TARGETPLATFORM
ARG BUILDPLATFORM
RUN echo "I am running on $BUILDPLATFORM, building for $TARGETPLATFORM" > /log

FROM alpine
COPY --from=build /log /log
```

В этом примере инструкция `FROM` закреплена за родной платформой сборщика с помощью опции `--platform=$BUILDPLATFORM`, чтобы предотвратить эмуляцию. Аргументы `$BUILDPLATFORM` и `$TARGETPLATFORM` затем используются в инструкции `RUN`.

### Stapel

В werf встроен альтернативный синтаксис описания сборочных инструкций, называемый stapel. Подробная документация по синтаксису stapel доступна [в соответствующей секции документации]({{ "/usage/build/stapel/overview.html" | true_relative_url }}).

Пример минимальной конфигурации stapel-образа в `werf.yaml`:

```yaml
project: example
configVersion: 1
---
image: app
from: ubuntu:22.04
```

Добавим исходники из Git в образ:

```yaml
project: example
configVersion: 1
---
image: app
from: ubuntu:22.04
git:
- add: /
  to: /app
```

Доступно 4 стадии для описания произвольных shell-инструкций, а также директива `git.stageDependencies` для настройки триггеров пересборки этих стадий при изменении соответствующих стадий ([см. подробнее]({{ "/usage/build/stapel/instructions.html#зависимость-от-изменений-в-git-репозитории" | true_relative_url }})):

```yaml
project: example
configVersion: 1
---
image: app
from: ubuntu:22.04
git:
- add: /
  to: /app
  stageDependencies:
    install:
    - package-lock.json
    - Gemfile.lock
    beforeSetup:
    - app/webpack/
    - app/assets/
    setup:
    - config/templates/
shell:
  beforeInstall:
  - apt update -q
  - apt install -y libmysqlclient-dev mysql-client g++
  install:
  - bundle install
  - npm install
  beforeSetup:
  - bundle exec rails assets:precompile
  setup:
  - rake generate:configs
```

Поддерживаются вспомогательные образы, из которых можно импортировать файлы в целевой образ (аналог `COPY --from=STAGE` в multi-stage Dockerfile), а также Golang-шаблонизация:

{% raw %}
```yaml
{{ $_ := set . "BaseImage" "ubuntu:22.04" }}

{{ define "package:build-tools" }}
  - apt update -q
  - apt install -y gcc g++ build-essential make
{{ end }}

project: example
configVersion: 1
---
image: builder
from: {{ .BaseImage }}
shell:
  beforeInstall:
{{ include "package:build-tools" }}
  install:
  - cd /app
  - make build
---
image: app
from: alpine:latest
import:
- image: builder
  add: /app/build/app
  to: /usr/local/bin/app
  after: install
```
{% endraw %}

Подробная документация по написанию доступна [в разделе stapel]({{ "usage/build/stapel/base.html" | true_relative_url }}).

## Изменение конфигурации образов

В OCI (Open Container Initiative) [image configuration spec](https://github.com/opencontainers/image-spec/blob/main/config.md) — это спецификация образа, которая описывает его структуру и метаданные. Директива `imageSpec` в `werf.yaml` предоставляет гибкие возможности управления и настройки различных аспектов образов:

- Гибкость в управлении полями спецификации.
- Удаление или сброс ненужных полей: меток, переменных окружения, томов, команд и истории сборки.
- Единый механизм конфигурации для всех поддерживаемых бэкендов и синтаксисов.
- Правила, которые применимы как ко всем образам в проекте, так и к отдельным образам.

### Глобальная конфигурация

Пример конфигурации, которая применяется ко всем образам в проекте:

```yaml
project: test
configVersion: 1
build:
  imageSpec:
    author: "Frontend Maintainer <frontend@example.com>"
    clearHistory: true
    config:
      removeLabels:
        - "unnecessary-label"
        - /org.opencontainers.image..*/
      labels:
        app: "my-app"
```

Эта настройка будет применяться ко всем образам в проекте: лейблы и автор будут установлены для всех образов, а ненужные лейблы будут удалены.

> **Примечание:** Глобальная конфигурация применяется только конечным (`final`) образам. Подробнее смотретите [здесь](#использование-промежуточных-и-конечных-образов)

### Конфигурация для конкретного образа

Пример настройки для отдельного образа:

```yaml
project: test
configVersion: 1
---
image: frontend_image
from: alpine
imageSpec:
  author: "Frontend Maintainer <frontend@example.com>"
  clearHistory: true
  config:
    user: "1001:1001"
    expose:
      - "8080/tcp"
    env:
      NODE_ENV: "production"
      API_URL: "https://api.example.com"
    entrypoint:
      - "/usr/local/bin/start.sh"
    volumes:
      - "/app/data"
    workingDir: "/app"
    labels:
      frontend-version: "1.2.3"
    stopSignal: "SIGTERM"
    removeLabels:
      - "old-frontend-label"
      - /old-regex-label.*/
    removeVolumes:
      - "/var/cache"
    removeEnv:
      - "DEBUG"
```

> **Примечание:** Конфигурация для конкретного образа имеет приоритет перед глобальной конфигурацией. Строковые значения будут переопределены, а для множественных значений данные будут объединяться в соответствии с приоритетом.

### Изменения в процессе сборки

Изменение конфигурации образа не влияет напрямую на процесс сборки образа, но позволяет настроить такие аспекты, как удаление ненужных томов или добавление переменных окружения для базового образа. Пример:

```yaml
image: base
from: postgres:12.22-bookworm
imageSpec:
  config:
    removeVolumes:
      - "/var/lib/postgresql/data"
---
image: app
fromImage: base
git:
  add: /postgresql/data
  to: /var/lib/postgresql/data
```

В этом примере базовый образ `postgres:12.22-bookworm` имеет удаленный ненужный том и теперь путь может быть использованы в образе `app`.

### Работа с CMD и ENTRYPOINT

Если в образе задан `CMD`, а в `imageSpec` указан `ENTRYPOINT`, то `CMD` будет сброшен. Таким образом, при изменении `ENTRYPOINT` необходимо явно задавать `CMD`, если он требуется.

Например, если в образе указана следующая конфигурация:

```json
"Cmd": ["node", "server.js"],
"Entrypoint": null
```

То для изменения `ENTRYPOINT`, `CMD` должен быть также явно указан. Конфигурация будет выглядеть так:

```yaml
project: test
configVersion: 1
---
image: frontend_image
from: alpine
imageSpec:
  config:
    entrypoint:
      - "/app/entrypoint.sh"
    cmd:
      - "node"
      - "server.js"
```

Такое поведение соответствует работе Docker с `CMD` и `ENTRYPOINT`, подробнее об этом можно узнать в [официальной документации](https://docs.docker.com/reference/dockerfile/#understand-how-cmd-and-entrypoint-interact)

### Работа с переменными окружения

При работе с переменными окружения вы можете ссылаться на существующие переменные в образе (`${ENV_NAME}`):

```yaml
project: test
configVersion: 1
---
image: backend
from: alpine:3.21
imageSpec:
  config:
    env:
      PATH: "${PATH}:/app/bin"
```

### Работа с лейблами

При добавлении лейблов можно использовать встроенные переменные:

 - `%image%`: имя образа.
 - `%project%`: имя проекта.

```yaml
project: test
configVersion: 1
---
image: backend
from: alpine:3.21
imageSpec:
  config:
    labels: 
      frontend-version: "1.2.3"
      project-%project%: "%image%-image"
```

## Взаимодействие между образами

### Наследование и импортирование файлов

При написании одного Dockerfile в нашем распоряжении имеется механизм multi-stage. Он позволяет объявить в Dockerfile отдельный образ-стадию и использовать её в качестве базового для другого образа, либо скопировать из неё отдельные файлы.

werf позволяет реализовать это не только в рамках одного Dockerfile, но и между произвольными образами, определяемыми в `werf.yaml`, в том числе собираемыми из разных Dockerfile'ов, либо собираемыми сборщиком stapel. Всю оркестрацию и выстраивание зависимостей werf возьмёт на себя и произведёт сборку за один шаг (вызов `werf build`).

Пример использования образа собранного из `base.Dockerfile` в качестве базового для образа из `Dockerfile`:

```Dockerfile
# base.Dockerfile
FROM ubuntu:22.04
RUN apt update -q && apt install -y gcc g++ build-essential make curl python3
```

```Dockerfile
# Dockerfile
ARG BASE_IMAGE
FROM ${BASE_IMAGE}
WORKDIR /app
COPY . .
CMD [ "/app/server", "start" ]
```

```yaml
# werf.yaml
project: example
configVersion: 1
---
image: base
dockerfile: base.Dockerfile
---
image: app
dockerfile: Dockerfile
dependencies:
- image: base
  imports:
  - type: ImageName
    targetBuildArg: BASE_IMAGE
```

В следующем примере рассмотрено импортирование файлов из образа stapel в образ Dockerfile:

```yaml
# werf.yaml
project: example
configVersion: 1
---
image: builder
from: golang:1.23rc1-alpine3.20
git:
- add: /
  to: /app
shell:
  install:
  - cd /app
  - go build -o /app/bin/server
---
image: app
dockerfile: Dockerfile
dependencies:
- image: builder
  imports:
  - type: ImageName
    targetBuildArg: BUILDER_IMAGE
```

```Dockerfile
# Dockerfile
ARG BUILDER_IMAGE
FROM ${BUILDER_IMAGE} AS builder

FROM alpine
COPY --from=builder /app/bin /app/bin
CMD [ "/app/bin/server", "server" ]
```

### Передача информации о собранном образе в другой образ

werf позволяет получить информацию о собранном образе при сборке другого образа. Например, если в сборочных инструкциях образа `app` требуются имена и digest'ы образов `auth` и `controlplane`, опубликованных в container registry, то конфигурация могла бы выглядеть так:

```Dockerfile
# modules/auth/Dockerfile
FROM alpine
WORKDIR /app
COPY . .
RUN ./build.sh
```

```Dockerfile
# modules/controlplane/Dockerfile
FROM alpine
WORKDIR /app
COPY . .
RUN ./build.sh
```

```Dockerfile
# Dockerfile
FROM alpine
WORKDIR /app
COPY . .

ARG AUTH_IMAGE_NAME
ARG AUTH_IMAGE_DIGEST
ARG CONTROLPLANE_IMAGE_NAME
ARG CONTROLPLANE_IMAGE_DIGEST

RUN echo AUTH_IMAGE_NAME=${AUTH_IMAGE_NAME}                     >> modules_images.env
RUN echo AUTH_IMAGE_DIGEST=${AUTH_IMAGE_DIGEST}                 >> modules_images.env   
RUN echo CONTROLPLANE_IMAGE_NAME=${CONTROLPLANE_IMAGE_NAME}     >> modules_images.env
RUN echo CONTROLPLANE_IMAGE_DIGEST=${CONTROLPLANE_IMAGE_DIGEST} >> modules_images.env
```

```yaml
# werf.yaml
project: example
configVersion: 1
---
image: auth
dockerfile: Dockerfile
context: modules/auth/
---
image: controlplane
dockerfile: Dockerfile
context: modules/controlplane/
---
image: app
dockerfile: Dockerfile
dependencies:
- image: auth
  imports:
  - type: ImageName
    targetBuildArg: AUTH_IMAGE_NAME
  - type: ImageDigest
    targetBuildArg: AUTH_IMAGE_DIGEST
- image: controlplane
  imports:
  - type: ImageName
    targetBuildArg: CONTROLPLANE_IMAGE_NAME
  - type: ImageDigest
    targetBuildArg: CONTROLPLANE_IMAGE_DIGEST
```

В процессе сборки werf автоматически подставит в указанные build-arguments соответствующие имена и идентификаторы. Всю оркестрацию и выстраивание зависимостей werf возьмёт на себя и произведёт сборку за один шаг (вызов `werf build`).

## Использование промежуточных и конечных образов

По умолчанию все образы являются конечными, что позволяет пользователю оперировать ими, используя их имена в качестве аргументов для большинства команд werf, а также в шаблонах Helm-чарта. С помощью директивы `final` можно регулировать это свойство образа.

Промежуточные образы (`final: false`) в отличие от конечных:
- не попадают [в служебные values для Helm-чарта]({{ "usage/deploy/values.html#информация-о-собранных-образах-только-в-werf" | true_relative_url }}).
- не тегируются произвольными тегами ([подробнее про --add-custom-tag]({{ "usage/build/process.html#добавление-произвольных-тегов" | true_relative_url }}));
- не публикуются в финальный репозиторий ([подробнее про --final-repo]({{ "usage/build/process.html#дополнительный-репозиторий-для-конечных-образов" | true_relative_url }}));
- не экспортируются ([подробнее про werf export]({{ "reference/cli/werf_export.html" | true_relative_url }})).

Пример использования директивы `final`:

```yaml
project: example
configVersion: 1
---
image: builder
final: false
dockerfile: Dockerfile.builder
---
image: app
dockerfile: Dockerfile.app
dependencies:
- image: builder
  imports:
  - type: ImageName
    targetBuildArg: BUILDER_IMAGE_NAME
```
