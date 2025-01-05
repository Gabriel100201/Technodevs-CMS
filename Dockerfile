# Base más reciente compatible con Go 1.23
FROM golang:1.23 AS builder

# Instalar Node.js
RUN apt-get update && apt-get install -y curl && \
    curl -fsSL https://deb.nodesource.com/setup_18.x | bash - && \
    apt-get install -y nodejs && \
    apt-get clean

# Configurar el directorio de trabajo
WORKDIR /app

# Copiar todo el proyecto al contenedor
COPY . .

# 1. Construir la interfaz (UI)
WORKDIR /app/ui
RUN npm install && npm run build

# 2. Construir el ejecutable base.exe para Linux
WORKDIR /app/examples/base
ENV GOOS=linux
ENV GOARCH=amd64
RUN go build -o base.exe main.go

# 3. Crear una imagen más liviana con Alpine para la ejecución
FROM alpine:latest

# Instalar dependencias necesarias
RUN apk add --no-cache libc6-compat

# Copiar el ejecutable y los archivos necesarios desde la fase de construcción
WORKDIR /app
COPY --from=builder /app/examples/base/base.exe /app/base.exe
COPY --from=builder /app/examples/base/pb_data /app/pb_data

# Exponer el puerto del servidor
EXPOSE 3000

# Ejecutar el servidor
CMD ["./base.exe", "serve", "--http=0.0.0.0:8090"]
