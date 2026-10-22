# syntax=docker/dockerfile:1.7
# Build reproducible de Rebirth Track Uploader para Windows (x64), igual en local y en CI:
#   docker build --build-arg VERSION=1.2.3 [--secret id=sign,src=sign.env] --target dist -o out .
# Deja en out/: el instalador, el .exe suelto y SHA256SUMS.txt.

FROM golang:1.27-trixie AS build
ARG VERSION=0.0.0-dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go vet ./... && go test ./... \
 && go install github.com/tc-hib/go-winres@v0.3.3 \
 && go-winres simply --arch amd64 --out cmd/rebirthtrack/rsrc --icon assets/icon-256.png --manifest gui \
      --product-name "Rebirth Track Uploader" --file-description "Rebirth Track Uploader" \
      --original-filename RebirthTrackUploader.exe --copyright "© Rebirth Knights" \
      --file-version "${VERSION%%-*}" --product-version "${VERSION}" \
 && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath \
      -ldflags "-H=windowsgui -s -w -X main.version=${VERSION}" \
      -o /out/RebirthTrackUploader.exe ./cmd/rebirthtrack

FROM debian:trixie-slim AS package
ARG VERSION=0.0.0-dev
ARG JSIGN_VERSION=7.6
ARG JSIGN_SHA256=6cf58aa080e7ff82656f320cff8d4a3bff30408df8766806dcba3740dd839fa5
RUN apt-get update && apt-get install -y --no-install-recommends nsis openjdk-21-jre-headless curl ca-certificates \
 && rm -rf /var/lib/apt/lists/* \
 && curl -fsSL -o /opt/jsign.jar "https://github.com/ebourg/jsign/releases/download/${JSIGN_VERSION}/jsign-${JSIGN_VERSION}.jar" \
 && echo "${JSIGN_SHA256}  /opt/jsign.jar" | sha256sum -c -
COPY build/sign.sh /usr/local/bin/sign
WORKDIR /src
COPY installer installer
COPY assets assets
COPY --from=build /out/RebirthTrackUploader.exe dist/RebirthTrackUploader.exe
ARG REQUIRE_SIGNING=0
RUN --mount=type=secret,id=sign,required=false \
    export REQUIRE_SIGNING=${REQUIRE_SIGNING} \
 && sign dist/RebirthTrackUploader.exe \
 && mkdir -p /out && cd installer \
 && makensis -V2 -DVERSION="${VERSION}" -DNUMVER="${VERSION%%-*}" -DSRC=../dist -DOUT="/out/RebirthTrackUploader-Setup-${VERSION}.exe" -DSIGN_CMD=/usr/local/bin/sign rebirthtrack.nsi \
 && cp ../dist/RebirthTrackUploader.exe "/out/RebirthTrackUploader-${VERSION}.exe" \
 && cd /out && sha256sum *.exe > SHA256SUMS.txt

FROM scratch AS dist
COPY --from=package /out /
