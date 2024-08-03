FROM golang:1.22.5

WORKDIR /app

COPY . .

RUN go build -o ./build/jobworker-server ./cmd/server

EXPOSE 50051

CMD ["./build/jobworker-server"]