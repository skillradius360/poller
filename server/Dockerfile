FROM golang:1.23-alpine AS build

WORKDIR /app

COPY go.mod go.sum* ./

RUN go mod download

COPY . .

RUN go mod tidy

RUN CGO_ENABLED=0 GOOS=linux go build -o server .

FROM alpine:3.21

WORKDIR /app

COPY --from=build /app/server .

EXPOSE 8080

CMD ["./server"]