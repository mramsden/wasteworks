FROM golang:1.21.0-alpine AS build

WORKDIR /app

COPY . .

RUN go build -o wasteworks ./cmd/server

FROM alpine:latest

ARG USERNAME=wasteworks
ARG USER_UID=1000
ENV HTTP_ADDR=0.0.0.0:8080

RUN adduser -u $USER_UID -D $USERNAME $USERNAME

WORKDIR /app

COPY --from=build /app/wasteworks .

EXPOSE 8080

USER $USERNAME

ENTRYPOINT ["/app/wasteworks"]
