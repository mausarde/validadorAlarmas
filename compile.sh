#!/bin/bash

# Obtener la versión desde Git
VERSION=$(git describe --tags --always --dirty)
COMMIT=$(git rev-parse --short HEAD)
DATE=$(date "+%Y-%m-%dT%H:%M:%S")
BUILT_BY=$(whoami)

cd src
go clean
# Compilar el binario con la versión obtenida
# CGO_ENABLED=0 Para compilar todas las dependencias y evitar algún error de librería en server
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ../validadorAlarmas -ldflags "-X main.version=$VERSION -X main.commit=$COMMIT -X main.date=$DATE -X main.builtBy=$BUILT_BY" 

echo "Build completed with version: $VERSION"

