FROM golang:1.23 AS builder

# Establecer el directorio de trabajo dentro del contenedor
WORKDIR /app

# Copiar el contenido del repositorio al contenedor
COPY . .

# Cambiar al directorio "examples/base"
WORKDIR /app/examples/base

# Construir la aplicación con las configuraciones especificadas
RUN GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o base

# Crear una imagen mínima para ejecutar la aplicación
FROM debian:bullseye-slim

# Establecer el directorio de trabajo
WORKDIR /app

# Copiar el binario construido desde la etapa anterior
COPY --from=builder /app/examples/base/base .

# Exponer el puerto que utiliza el servidor (ajusta si es necesario)
EXPOSE 8080

# Comando para ejecutar la aplicación
CMD ["./base", "serve", "--http=0.0.0.0:8080"]
