### Stage: Builder ###

FROM golang:latest as builder

LABEL maintainer="Sindri Guðmundsson <sindrigudmundsson@gmail.com>"

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main .

### Stage: App ###

FROM alpine:latest

RUN apk --no-cache add ca-certificates

ARG LOG_DIR=/logs
ENV LOG_FILE_LOCATION=${LOG_DIR}/app.log
RUN mkdir -p ${LOG_DIR}

ENV JSONTVURL=https://fast-guide.smoothstreams.tv/

WORKDIR /root/

COPY --from=builder /app/main .

EXPOSE 8080
VOLUME [${LOG_DIR}]


CMD ["./main"]
