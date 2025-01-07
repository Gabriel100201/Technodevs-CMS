# Usar una imagen base con Go
FROM golang:1.23 AS builder

# Crear el directorio de trabajo en el contenedor
WORKDIR /app

# Copiar los archivos necesarios al contenedor
COPY . .

# Entrar a la carpeta donde se encuentra el archivo main.go
WORKDIR /app/examples/base

# Compilar la aplicación con Go
RUN GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build

# Crear una etapa final para la imagen más ligera
FROM debian:bullseye-slim

# Crear el directorio para la aplicación
WORKDIR /app

# Copiar el binario desde la etapa de compilación
COPY --from=builder /app/examples/base/base.exe ./

# Exponer el puerto de la aplicación
EXPOSE 8090

# Comando para ejecutar la aplicación
CMD ["./base.exe", "serve"]
