FROM node:22-alpine AS fe
WORKDIR /fe
COPY frontend/package*.json ./
RUN npm install
COPY frontend/ .
RUN npm run build

FROM golang:1.23-alpine AS be
WORKDIR /be
COPY backend/ .
RUN go mod tidy && CGO_ENABLED=0 go build -o /server .

FROM alpine:3.20
WORKDIR /app
COPY --from=be /server /app/server
COPY --from=fe /fe/dist /app/static
ENV STATIC_DIR=/app/static
EXPOSE 8080
CMD ["/app/server"]
